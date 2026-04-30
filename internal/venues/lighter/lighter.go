package lighter

import (
	"encoding/json"
	"fmt"

	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/venues/spec"
)

const DefaultBaseURL = "https://mainnet.zklighter.elliot.ai"
const DefaultHTTPPath = "/api/v1/sendTx"
const DefaultBatchPath = "/api/v1/sendTxBatch"
const DefaultWSURL = "wss://mainnet.zklighter.elliot.ai/stream"

const WebSocketSendTxType = "jsonapi/sendtx"
const WebSocketSendTxBatchType = "jsonapi/sendtxbatch"

func Definition() spec.Definition {
	return spec.Definition{
		Name:             "lighter",
		Aliases:          []string{"zkLighter", "zk-lighter", "zklighter"},
		DefaultBaseURL:   DefaultBaseURL,
		DefaultHTTPPath:  DefaultHTTPPath,
		DefaultBatchPath: DefaultBatchPath,
		DefaultWSURL:     DefaultWSURL,
		Capabilities: spec.Capabilities{
			HTTPSingle:      true,
			HTTPBatch:       true,
			WebSocketSingle: true,
			WebSocketBatch:  true,
		},
		BuilderParams: spec.BuilderParams{
			Required: []string{"market_index", "base_amount", "price"},
			Defaults: map[string]any{
				"symbol":       "BTC",
				"market_index": 1,
				"base_amount":  100,
				"price":        750000,
				"side":         "buy",
				"post_only":    true,
			},
		},
		Classifier: classify,
		Docs: []string{
			"https://apidocs.lighter.xyz/docs/get-started",
			"https://apidocs.lighter.xyz/reference/sendtx",
			"https://apidocs.lighter.xyz/reference/sendtxbatch",
			"https://apidocs.lighter.xyz/docs/websocket-reference",
			"https://github.com/elliottech/lighter-python",
		},
		Notes: []string{
			"HTTP transaction submission is POST /api/v1/sendTx with tx_type and signed tx_info form fields.",
			"HTTP batch transaction submission is POST /api/v1/sendTxBatch with signed tx_types and tx_infos form fields.",
			"WebSocket transaction submission uses type=jsonapi/sendtx with data.tx_type and signed data.tx_info.",
			"WebSocket batch transaction submission uses type=jsonapi/sendtxbatch with signed data.tx_types and signed data.tx_infos.",
			"Signing and payload generation are expected to happen before benchmarking; this venue reuses prebuilt request bodies.",
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
	var decoded map[string]any
	if err := json.Unmarshal(in.Body, &decoded); err != nil {
		return generic
	}
	if code, ok := numericCode(decoded["code"]); ok && code >= 400 {
		return lifecycle.Classification{Status: lifecycle.StatusRejected, Reason: reason(decoded)}
	}
	if success, ok := decoded["success"].(bool); ok && !success {
		return lifecycle.Classification{Status: lifecycle.StatusRejected, Reason: reason(decoded)}
	}
	return generic
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
	for _, key := range []string{"message", "error", "reason"} {
		if raw, ok := value[key]; ok {
			return fmt.Sprint(raw)
		}
	}
	return ""
}
