package pacifica

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"perps-latency-benchmark/internal/booktop"
	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/venues/spec"
)

const DefaultBaseURL = "https://api.pacifica.fi/api/v1"
const DefaultHTTPPath = "/orders/create"
const DefaultBatchPath = "/orders/batch"
const DefaultWSURL = "wss://ws.pacifica.fi/ws"
const WebSocketHeartbeatMessage = `{"method":"ping"}`
const WebSocketDocsURL = "https://docs.pacifica.fi/api-documentation/api/websocket"
const WebSocketLimitOrderDocsURL = "https://docs.pacifica.fi/api-documentation/api/websocket/trading-operations/create-limit-order"
const WebSocketMarketOrderDocsURL = "https://docs.pacifica.fi/api-documentation/api/websocket/trading-operations/create-order"
const WebSocketBatchOrderDocsURL = "https://docs.pacifica.fi/api-documentation/api/websocket/trading-operations/batch-order"
const WebSocketCancelOrderDocsURL = "https://docs.pacifica.fi/api-documentation/api/websocket/trading-operations/cancel-order"
const SigningDocsURL = "https://docs.pacifica.fi/api-documentation/api/signing"
const RateLimitsDocsURL = "https://docs.pacifica.fi/api-documentation/api/rate-limits"
const AgentKeysDocsURL = "https://docs.pacifica.fi/api-documentation/api/signing/api-agent-keys"

func Definition() spec.Definition {
	return spec.Definition{
		Name:             "pacifica",
		Aliases:          []string{"pacifica-fi", "pacifica_fi"},
		DefaultBaseURL:   DefaultBaseURL,
		DefaultHTTPPath:  DefaultHTTPPath,
		DefaultBatchPath: DefaultBatchPath,
		DefaultWSURL:     DefaultWSURL,
		WSHeartbeat: spec.WebSocketHeartbeat{
			Message:   WebSocketHeartbeatMessage,
			IdleAfter: 45 * time.Second,
			Timeout:   5 * time.Second,
		},
		Capabilities: spec.Capabilities{
			WebSocketSingle: true,
			WebSocketBatch:  true,
			Cleanup:         true,
		},
		BuilderParams: spec.BuilderParams{
			Required: []string{"symbol", "amount", "side", "price"},
			Defaults: map[string]any{
				"symbol":        "BTC",
				"amount":        "0.001",
				"side":          "bid",
				"price":         "75000",
				"tif":           "ALO",
				"expiry_window": 5000,
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
				"solders",
				"--with",
				"base58",
				"python",
				filepath.FromSlash("internal/venues/pacifica/cancel_payload.py"),
			},
			Description:    "cancel Pacifica benchmark orders by client order id",
			OrderRefsField: "cleanup_orders",
			SkipNoRefs:     true,
		},
		BookTop: spec.BookTop{
			Build: func(runtime spec.RuntimeConfig) (booktop.Config, bool) {
				url := spec.CoalesceURL(runtime.WSURL, DefaultWSURL)
				symbol := spec.TextParam(runtime.Params, "symbol", "BTC")
				if url == "" || symbol == "" {
					return booktop.Config{}, false
				}
				return booktop.Config{
					URL:    url,
					Symbol: symbol,
					Parser: booktop.NewPacificaParser(),
				}, true
			},
		},
		ExpectedFill: spec.ExpectedFill{
			Build: func(runtime spec.RuntimeConfig) (spec.ExpectedFillOrder, bool) {
				return spec.ExpectedFillOrder{
					Side: sideForExpectedFill(spec.TextParam(runtime.Params, "side", "bid")),
					Size: spec.FloatParam(runtime.Params, "amount"),
				}, true
			},
		},
		Classifier:         Classify,
		Confirmation:       ConfirmWebSocket,
		CancelConfirmation: ConfirmCancelWebSocket,
		Docs: []string{
			"https://docs.pacifica.fi/api-documentation/api",
			WebSocketDocsURL,
			WebSocketLimitOrderDocsURL,
			WebSocketMarketOrderDocsURL,
			WebSocketBatchOrderDocsURL,
			WebSocketCancelOrderDocsURL,
			SigningDocsURL,
			RateLimitsDocsURL,
			AgentKeysDocsURL,
			"https://github.com/pacifica-fi/python-sdk",
		},
		Notes: []string{
			"WebSocket order submission at wss://ws.pacifica.fi/ws is the fastest verified order-entry path.",
			"Limit-order messages use params.create_order; native batch messages use params.batch_orders.actions with up to 10 individually signed actions.",
			"Market-order messages use params.create_market_order and are subject to Pacifica's documented roughly 200 ms latency-protection delay.",
			"Pacifica signing uses deterministic compact JSON and Ed25519 signatures over a header containing timestamp, expiry_window, and operation type plus the order data.",
			"Pacifica closes WebSocket connections if no message is sent for 60 seconds, so this venue sends an untimed ping before measured sends when the connection has been idle.",
			"Default params use TIF=ALO because Pacifica documents a roughly 200 ms delay for GTC/IOC limit orders; ALO and TOB avoid that delay.",
			"Batch orders have a conditional randomized 50-100 ms delay when the batch contains market orders or GTC/IOC limit orders; all-ALO/all-TOB batches avoid it.",
			"API Agent Keys can sign order requests for the main account when PACIFICA_ACCOUNT and PACIFICA_AGENT_WALLET are configured.",
		},
	}
}

func BuildCommand() []string {
	return []string{
		"uv",
		"run",
		"--python",
		"3.13",
		"--with",
		"solders",
		"--with",
		"base58",
		"python",
		filepath.FromSlash("internal/venues/pacifica/build_payload.py"),
	}
}

func Classify(in lifecycle.ResponseInput) lifecycle.Classification {
	return classify(in)
}

func classify(in lifecycle.ResponseInput) lifecycle.Classification {
	generic := lifecycle.ClassifyResponse(in)
	if in.Err != nil || len(in.Body) == 0 || !generic.OK() {
		return generic
	}
	var decoded any
	if err := json.Unmarshal(in.Body, &decoded); err != nil {
		return generic
	}
	if finding := classifyValue(decoded); finding != nil {
		return *finding
	}
	return generic
}

func classifyValue(value any) *lifecycle.Classification {
	switch typed := value.(type) {
	case map[string]any:
		if code, ok := numericCode(typed["code"]); ok {
			if code >= 200 && code < 300 {
				return &lifecycle.Classification{Status: lifecycle.StatusAccepted}
			}
			return &lifecycle.Classification{Status: lifecycle.StatusRejected, Reason: reason(typed)}
		}
		if errText := firstText(typed, "error", "message"); errText != "" {
			return &lifecycle.Classification{Status: lifecycle.StatusRejected, Reason: errText}
		}
		for _, child := range typed {
			if finding := classifyValue(child); finding != nil {
				return finding
			}
		}
	case []any:
		for _, child := range typed {
			if finding := classifyValue(child); finding != nil && finding.Status != lifecycle.StatusAccepted {
				return finding
			}
		}
		return &lifecycle.Classification{Status: lifecycle.StatusAccepted}
	}
	return nil
}

func numericCode(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case int:
		return typed, true
	default:
		return 0, false
	}
}

func reason(value map[string]any) string {
	if text := firstText(value, "error", "message", "reason"); text != "" {
		return text
	}
	if raw, ok := value["code"]; ok {
		return fmt.Sprint(raw)
	}
	return ""
}

func firstText(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, ok := value[key]; ok {
			text := strings.TrimSpace(fmt.Sprint(raw))
			if text != "" {
				return text
			}
		}
	}
	return ""
}

func sideForExpectedFill(side string) string {
	switch strings.ToLower(strings.TrimSpace(side)) {
	case "bid", "buy":
		return "buy"
	case "ask", "sell":
		return "sell"
	default:
		return strings.ToLower(strings.TrimSpace(side))
	}
}
