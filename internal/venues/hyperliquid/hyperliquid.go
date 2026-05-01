package hyperliquid

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/venues/spec"
)

const DefaultBaseURL = "https://api.hyperliquid.xyz"
const DefaultHTTPPath = "/exchange"
const DefaultWSURL = "wss://api.hyperliquid.xyz/ws"
const WebSocketHeartbeatMessage = `{"method":"ping"}`

const ExchangeEndpointDocsURL = "https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/exchange-endpoint"
const WebSocketDocsURL = "https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/websocket"
const WebSocketPostRequestsDocsURL = "https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/websocket/post-requests"

func Definition() spec.Definition {
	return spec.Definition{
		Name:            "hyperliquid",
		Aliases:         []string{"hl", "hyper-liquid", "hyper_liquid"},
		DefaultBaseURL:  DefaultBaseURL,
		DefaultHTTPPath: DefaultHTTPPath,
		DefaultWSURL:    DefaultWSURL,
		WSHeartbeat: spec.WebSocketHeartbeat{
			Message:   WebSocketHeartbeatMessage,
			IdleAfter: 15 * time.Second,
			Timeout:   5 * time.Second,
		},
		Capabilities: spec.Capabilities{
			HTTPSingle:      true,
			HTTPBatch:       true,
			WebSocketSingle: true,
			WebSocketBatch:  true,
		},
		BuilderParams: spec.BuilderParams{
			Required: []string{"asset", "size", "price"},
			Defaults: map[string]any{
				"symbol": "BTC",
				"asset":  0,
				"side":   "buy",
				"size":   "0.001",
				"price":  "75000",
				"tif":    "Alo",
			},
		},
		Classifier:   classify,
		Confirmation: ConfirmWebSocket,
		Docs: []string{
			ExchangeEndpointDocsURL,
			WebSocketDocsURL,
			WebSocketPostRequestsDocsURL,
		},
		Notes: []string{
			"HTTP submits the pre-signed exchange payload directly to POST /exchange.",
			"WebSocket post requests connect to /ws and must send a wrapper with method=post, id, request.type=action, and request.payload containing the signed exchange payload.",
			"Hyperliquid closes idle WebSocket connections after 60 seconds, so the venue sends an untimed ping before measured sends when the connection has been idle.",
			"Signing and any HTTP-to-WebSocket payload wrapping must be done before benchmarking so request preparation only reuses the prebuilt body.",
		},
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
	status, reason := findStatus(decoded)
	switch strings.ToLower(status) {
	case "ok":
		return lifecycle.Classification{Status: lifecycle.StatusAccepted}
	case "err", "error", "rejected":
		return lifecycle.Classification{Status: lifecycle.StatusRejected, Reason: reason}
	default:
		return generic
	}
}

func findStatus(value any) (string, string) {
	switch typed := value.(type) {
	case map[string]any:
		if raw, ok := typed["status"].(string); ok {
			return raw, textReason(typed)
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

func textReason(value map[string]any) string {
	for _, key := range []string{"response", "error", "message"} {
		if raw, ok := value[key]; ok {
			return fmt.Sprint(raw)
		}
	}
	return ""
}
