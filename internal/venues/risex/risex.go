package risex

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/venues/spec"
)

const DefaultBaseURL = "https://api.rise.trade"
const DefaultHTTPPath = "/v1/orders/place"
const DefaultCancelPath = "/v1/orders/cancel"
const DefaultWSURL = "wss://ws.rise.trade/ws"
const DefaultChainID = 4153

func Definition() spec.Definition {
	return spec.Definition{
		Name:            "risex",
		Aliases:         []string{"rise_x", "rise-x", "rise_trade"},
		DefaultBaseURL:  DefaultBaseURL,
		DefaultHTTPPath: DefaultHTTPPath,
		Capabilities: spec.Capabilities{
			HTTPSingle:      true,
			HTTPBatch:       false,
			WebSocketSingle: false,
			Cleanup:         true,
			Neutralization:  false,
		},
		BuilderParams: spec.BuilderParams{
			Required: []string{"market_id", "price", "amount"},
			Defaults: map[string]any{
				"market_id":     1,
				"symbol":        "BTC/USDC",
				"side":          "buy",
				"amount":        "0.001",
				"price":         "63000",
				"order_type":    "limit",
				"time_in_force": "gtc",
				"post_only":     true,
				"reduce_only":   false,
				"stp_mode":      0,
				"deadline_secs": 3600,
				"chain_id":      DefaultChainID,
				"confirmation":  false,
			},
		},
		CleanupCommand: spec.CleanupCommand{
			Type: "persistent_command",
			Command: []string{
				"uv",
				"run",
				"--python",
				"3.13",
				"--with",
				"eth-account",
				"--with",
				"eth-utils",
				"python",
				filepath.FromSlash("internal/venues/risex/cancel_payload.py"),
			},
			Description:    "cancel RISEx benchmark orders by resting_order_id",
			OrderRefsField: "cleanup_orders",
			SkipNoRefs:     true,
		},
		ExpectedFill: spec.ExpectedFill{
			Build: func(runtime spec.RuntimeConfig) (spec.ExpectedFillOrder, bool) {
				return spec.ExpectedFillOrder{
					Side: spec.TextParam(runtime.Params, "side", "buy"),
					Size: spec.FloatParam(runtime.Params, "amount"),
				}, true
			},
		},
		Classifier:         Classify,
		Confirmation:       ConfirmWebSocket,
		CancelConfirmation: ConfirmCancelWebSocket,
		Docs: []string{
			"https://docs.risechain.com/docs/risex",
			"https://developer.rise.trade/reference/general-information",
			"https://developer.rise.trade/reference/integration",
			"https://developer.rise.trade/reference/orderservice_placeorder",
			"https://developer.rise.trade/reference/orderservice_cancelorder",
			"https://developer.rise.trade/reference/authservice_geteip712domain",
			"https://developer.rise.trade/reference/authservice_registersigner",
			"https://developer.rise.trade/reference/apiservice_getsystemconfig",
			"https://developer.rise.trade/reference/marketservice_getmarkets",
			"https://developer.rise.trade/reference/ws-connection",
			"https://developer.rise.trade/reference/orders-channel",
			"https://developer.rise.trade/reference/authentication-3",
		},
		Notes: []string{
			"RISEx docs are inconsistent about mainnet hostnames; DefaultBaseURL and DefaultWSURL are the verified-working values -- see README.md's mainnet host caveat.",
			"Order submission is REST-only; RISEx has no documented WebSocket order-entry endpoint, so WebSocketSingle is false.",
			"Batch scenario is refused outright (HTTPBatch: false), not faked -- RISEx's strict sequential nonce_anchor makes concurrent single-order fanout unreliable. See README.md's batch section.",
			"Auth model is EIP-712 permit-based session-key delegation via a registered session signer, not a raw wallet signature per order. See README.md's auth model section.",
			"nonce_anchor must be exactly (current anchor) + 1 and is fetched fresh at the start of every Build() call, never cached across calls. See README.md's nonce scheme section.",
			"order_data bit-packing and header_flags are verified live for post-only/GTC/limit orders; other order types raise rather than guess. See README.md.",
			"Cancellation needs resting_order_id (from GET /v1/orders/open), not the order_id returned by place; POST /v1/orders/cancel's order_id must be 24-byte 0x-prefixed hex, not decimal.",
			"Rate limits: 500 REST requests/10s/IP, 10 WebSocket requests/s/IP per RISEx docs.",
		},
	}
}

func Classify(in lifecycle.ResponseInput) lifecycle.Classification {
	generic := lifecycle.ClassifyResponse(in)
	if in.Err != nil || len(in.Body) == 0 {
		return generic
	}
	var decoded any
	if err := json.Unmarshal(in.Body, &decoded); err != nil {
		return generic
	}
	if isSuccess(decoded) {
		return lifecycle.Classification{Status: lifecycle.StatusAccepted}
	}
	if code, message, ok := findError(decoded); ok {
		classification := lifecycle.Classification{Status: lifecycle.StatusRejected, Reason: message}
		lower := strings.ToLower(message)
		switch {
		case strings.Contains(lower, "signature") || strings.Contains(lower, "auth") || strings.Contains(lower, "unauthor") || strings.Contains(lower, "not registered"):
			classification.Status = lifecycle.StatusAuthError
		case strings.Contains(lower, "nonce"):
			classification.Status = lifecycle.StatusNonceError
		case strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many"):
			classification.Status = lifecycle.StatusRateLimited
		}
		if classification.Reason == "" {
			classification.Reason = fmt.Sprintf("code %v", code)
		}
		return classification
	}
	return generic
}

func isSuccess(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if _, ok := typed["order_id"]; ok {
			return true
		}
		if _, ok := typed["tx_hash"]; ok {
			if success, ok := typed["success"].(bool); ok {
				return success
			}
			return true
		}
		if data, ok := typed["data"]; ok {
			return isSuccess(data)
		}
	}
	return false
}

func findError(value any) (any, string, bool) {
	switch typed := value.(type) {
	case map[string]any:
		if code, hasCode := typed["code"]; hasCode {
			message, _ := typed["message"].(string)
			return code, message, true
		}
		// Map iteration order is randomized in Go; sort keys so a response
		// body with more than one nested object carrying a "code" field
		// classifies the same way on every run instead of picking whichever
		// child the runtime happened to visit first.
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if code, message, ok := findError(typed[key]); ok {
				return code, message, true
			}
		}
	case []any:
		for _, child := range typed {
			if code, message, ok := findError(child); ok {
				return code, message, true
			}
		}
	}
	return nil, "", false
}
