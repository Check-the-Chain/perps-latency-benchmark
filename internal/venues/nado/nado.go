package nado

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/venues/spec"
)

const DefaultBaseURL = "https://gateway.prod.nado.xyz/v1"
const DefaultHTTPPath = "/execute"
const DefaultWSURL = "wss://gateway.prod.nado.xyz/v1/ws"
const DefaultSubscriptionWSURL = "wss://gateway.prod.nado.xyz/v1/subscribe"
const DefaultChainID = 57073

func Definition() spec.Definition {
	return spec.Definition{
		Name:            "nado",
		Aliases:         []string{"nado_xyz", "nado-perps", "nado_perps"},
		DefaultBaseURL:  DefaultBaseURL,
		DefaultHTTPPath: DefaultHTTPPath,
		DefaultWSURL:    DefaultWSURL,
		WSHeartbeat: spec.WebSocketHeartbeat{
			ControlFrame: "ping",
			IdleAfter:    25 * time.Second,
			Timeout:      5 * time.Second,
		},
		Capabilities: spec.Capabilities{
			HTTPSingle:      true,
			HTTPBatch:       true,
			WebSocketSingle: true,
			Cleanup:         true,
			Neutralization:  false,
		},
		BuilderParams: spec.BuilderParams{
			Required: []string{"product_id", "price", "amount"},
			Defaults: map[string]any{
				"product_id":      2,
				"symbol":          "BTC-PERP",
				"side":            "buy",
				"amount":          "0.0014",
				"price":           "77000",
				"order_type":      "post_only",
				"subaccount":      "default",
				"expiration":      "4294967295",
				"recv_window_ms":  5000,
				"chain_id":        DefaultChainID,
				"subscription_ws": DefaultSubscriptionWSURL,
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
				"python",
				filepath.FromSlash("internal/venues/nado/cancel_payload.py"),
			},
			Description:    "cancel Nado benchmark orders by digest",
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
			"https://docs.nado.xyz/developer-resources/api/endpoints",
			"https://docs.nado.xyz/developer-resources/api/gateway",
			"https://docs.nado.xyz/developer-resources/api/gateway/executes/place-order",
			"https://docs.nado.xyz/developer-resources/api/gateway/executes/cancel-orders",
			"https://docs.nado.xyz/developer-resources/api/gateway/signing",
			"https://docs.nado.xyz/developer-resources/api/subscriptions",
			"https://docs.nado.xyz/developer-resources/api/subscriptions/streams",
			"https://docs.nado.xyz/developer-resources/api/subscriptions/events",
			"https://docs.inkonchain.com/general/network-information",
		},
		Notes: []string{
			"Fastest verified order path is WebSocket execute at wss://gateway.prod.nado.xyz/v1/ws; REST fallback is POST https://gateway.prod.nado.xyz/v1/execute.",
			"Batch benchmarking uses concurrent POST /v1/execute place_order requests because the official docs do not verify a native multi-place-order endpoint.",
			"HTTP execute requests must include an Accept-Encoding value containing gzip, br, or deflate; WebSocket avoids that HTTP requirement.",
			"Place-order signing uses EIP-712 domain name=Nado, version=0.0.1, chainId=57073 on Ink mainnet, and verifyingContract=20-byte product_id address.",
			"Order nonce packs a recv/discard timestamp into the high 44 bits and random collision bits into the low 20 bits; set recv_window_ms low only when prepare-to-send delay is tightly controlled.",
			"Post-only appendix is 1537: version 1 plus order type POST_ONLY in bits 9-10.",
			"Private order confirmation uses the authenticated subscription WebSocket at wss://gateway.prod.nado.xyz/v1/subscribe and order_update events matched by order digest.",
			"Gateway and subscription sockets require WebSocket ping frames every 30 seconds; this venue sends protocol ping frames after 25 seconds of idle time.",
			"Nado documents off-chain sequencer latency around 5-15 ms, with on-chain settlement batched later on Ink L2.",
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
	status, reason := findStatus(decoded)
	switch strings.ToLower(status) {
	case "success", "ok":
		return lifecycle.Classification{Status: lifecycle.StatusAccepted}
	case "failure", "failed", "error", "rejected":
		classification := lifecycle.Classification{Status: lifecycle.StatusRejected, Reason: reason}
		lower := strings.ToLower(reason)
		if strings.Contains(lower, "signature") || strings.Contains(lower, "auth") || strings.Contains(lower, "unauthor") {
			classification.Status = lifecycle.StatusAuthError
		}
		if strings.Contains(lower, "nonce") || strings.Contains(lower, "recv") {
			classification.Status = lifecycle.StatusNonceError
		}
		if strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many") {
			classification.Status = lifecycle.StatusRateLimited
		}
		return classification
	default:
		return generic
	}
}

func findStatus(value any) (string, string) {
	switch typed := value.(type) {
	case map[string]any:
		if raw, ok := typed["status"].(string); ok {
			return raw, reason(typed)
		}
		if raw, ok := typed["result"].(string); ok {
			return raw, reason(typed)
		}
		for _, child := range typed {
			if status, reason := findStatus(child); status != "" {
				return status, reason
			}
		}
	case []any:
		for _, child := range typed {
			if status, reason := findStatus(child); status != "" {
				return status, reason
			}
		}
	}
	return "", ""
}

func reason(value map[string]any) string {
	for _, key := range []string{"error", "message", "reason", "error_code", "code"} {
		if raw, ok := value[key]; ok {
			return fmt.Sprint(raw)
		}
	}
	return ""
}
