package aster

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"perps-latency-benchmark/internal/booktop"
	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/venues/spec"
)

const DefaultBaseURL = "https://fapi.asterdex.com"
const DefaultWSURL = "wss://fstream.asterdex.com"
const DefaultHTTPPath = "/fapi/v3/order"
const DefaultBatchPath = "/fapi/v3/batchOrders"

func Definition() spec.Definition {
	return spec.Definition{
		Name:             "aster",
		Aliases:          []string{"asterdex", "aster_perpetuals", "aster-perpetuals", "aster_perp", "aster-perp"},
		DefaultBaseURL:   DefaultBaseURL,
		DefaultHTTPPath:  DefaultHTTPPath,
		DefaultBatchPath: DefaultBatchPath,
		DefaultWSURL:     DefaultWSURL,
		Capabilities: spec.Capabilities{
			HTTPSingle:     true,
			HTTPBatch:      true,
			Cleanup:        true,
			Neutralization: true,
		},
		BuilderParams: spec.BuilderParams{
			Required: []string{"symbol", "side", "type", "quantity"},
			Defaults: map[string]any{
				"symbol":              "BTCUSDT",
				"side":                "BUY",
				"type":                "LIMIT",
				"time_in_force":       "GTX",
				"quantity":            "0.001",
				"price":               "75000",
				"new_order_resp_type": "ACK",
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
				filepath.FromSlash("internal/venues/aster/cancel_payload.py"),
			},
			Description:    "cancel Aster benchmark orders by client order id",
			OrderRefsField: "cleanup_orders",
			SkipNoRefs:     true,
		},
		CostCommand: spec.CostCommand{
			Type: "persistent_command",
			Command: []string{
				"uv",
				"run",
				"--python",
				"3.13",
				"--with",
				"eth-account",
				"python",
				filepath.FromSlash("internal/venues/aster/cost_payload.py"),
			},
			Timeout: 60 * time.Second,
		},
		BookTop: spec.BookTop{
			Build: func(runtime spec.RuntimeConfig) (booktop.Config, bool) {
				symbol := strings.ToLower(spec.TextParam(runtime.Params, "symbol", "BTCUSDT"))
				url := spec.CoalesceURL(runtime.WSURL, DefaultWSURL)
				if url == "" || symbol == "" {
					return booktop.Config{}, false
				}
				return booktop.Config{
					URL:    url + "/ws/" + symbol + "@depth20@100ms",
					Symbol: symbol,
					Parser: booktop.NewAsterParser(),
				}, true
			},
		},
		ExpectedFill: spec.ExpectedFill{
			Build: func(runtime spec.RuntimeConfig) (spec.ExpectedFillOrder, bool) {
				return spec.ExpectedFillOrder{
					Side: strings.ToLower(spec.TextParam(runtime.Params, "side", "BUY")),
					Size: spec.FloatParam(runtime.Params, "quantity"),
				}, true
			},
		},
		Classifier:         Classify,
		Confirmation:       ConfirmWebSocket,
		CancelConfirmation: ConfirmCancelWebSocket,
		Docs: []string{
			"https://asterdex.github.io/aster-api-website/futures-v3/general-info/",
			"https://asterdex.github.io/aster-api-website/futures-v3/account%26trades/",
			"https://asterdex.github.io/aster-api-website/asterCode/authentication/",
			"https://docs.asterdex.com/product/aster-perpetuals/api/api-documentation",
		},
		Notes: []string{
			"Futures V3 order submission uses an API Wallet signer: user is the main login wallet, signer is the API wallet, nonce is microseconds, and signature is EIP-712 Message.msg over the final encoded form/query string.",
			"HTTPS single order submission uses POST /fapi/v3/order with application/x-www-form-urlencoded parameters.",
			"HTTPS batch order submission uses POST /fapi/v3/batchOrders with a stringified batchOrders list and one top-level signature.",
			"Post-only orders use timeInForce=GTX.",
			"WebSocket confirmation uses the signed V3 user data stream listenKey at wss://fstream.asterdex.com/ws/<listenKey> and matches ORDER_TRADE_UPDATE events by client order ID.",
			"Cleanup cancels by origClientOrderId and can neutralize filled inventory with reduce-only market orders outside the measured latency window.",
			"Official WebSocket documentation verifies market and user data streams at wss://fstream.asterdex.com, but no order-submission WebSocket endpoint was verified.",
			"Tokyo fairness is not verified: official docs do not state the order-entry backend, sequencer, matching-engine, or validator region. REST hits CloudFront NRT from the Tokyo benchmark host, and fstream resolves to low-latency AWS Tokyo-looking hosts, but that proves edge/host placement only.",
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
	if finding := classifyAsterValue(decoded); finding != nil {
		return *finding
	}
	return generic
}

func classifyAsterValue(value any) *lifecycle.Classification {
	switch typed := value.(type) {
	case []any:
		for _, child := range typed {
			if finding := classifyAsterValue(child); finding != nil && finding.Status != lifecycle.StatusAccepted {
				return finding
			}
		}
		return &lifecycle.Classification{Status: lifecycle.StatusAccepted}
	case map[string]any:
		if rawCode, ok := typed["code"]; ok {
			code, codeOK := asterCode(rawCode)
			if codeOK && code < 0 {
				status := lifecycle.StatusRejected
				if code == -1021 {
					status = lifecycle.StatusNonceError
				}
				if code == -2014 || code == -2015 || code == -1022 {
					status = lifecycle.StatusAuthError
				}
				return &lifecycle.Classification{Status: status, Reason: reason(typed)}
			}
		}
	}
	return nil
}

func asterCode(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case int:
		return typed, true
	case string:
		code, err := strconv.Atoi(typed)
		return code, err == nil
	default:
		return 0, false
	}
}

func reason(value map[string]any) string {
	for _, key := range []string{"msg", "message", "error", "reason", "code"} {
		if raw, ok := value[key]; ok {
			return fmt.Sprint(raw)
		}
	}
	return ""
}
