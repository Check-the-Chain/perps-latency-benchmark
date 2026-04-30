package grvt

import (
	"encoding/json"
	"fmt"

	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/venues/spec"
)

const DefaultBaseURL = "https://trades.grvt.io"
const DefaultWSURL = "wss://trades.grvt.io/ws/full"

func Definition() spec.Definition {
	return spec.Definition{
		Name:             "grvt",
		Aliases:          []string{"gravity", "gravitymarkets", "gravity-markets"},
		DefaultBaseURL:   DefaultBaseURL,
		DefaultHTTPPath:  "/full/v1/create_order",
		DefaultBatchPath: "/full/v2/bulk_orders",
		DefaultWSURL:     DefaultWSURL,
		Capabilities: spec.Capabilities{
			HTTPSingle:      true,
			HTTPBatch:       true,
			WebSocketSingle: true,
			WebSocketBatch:  true,
		},
		BuilderParams: spec.BuilderParams{Required: []string{"instrument", "size", "price", "instruments"}},
		Classifier:    classify,
		Docs: []string{
			"https://api-docs.grvt.io/",
			"https://api-docs.grvt.io/trading_api/",
			"https://api-docs.grvt.io/trading_streams/",
			"https://github.com/gravity-technologies/grvt-pysdk",
		},
		Notes: []string{
			"HTTP order submission uses POST /full/v1/create_order with a signed order wrapped as {\"order\": ...}.",
			"HTTP batch order submission uses POST /full/v2/bulk_orders with signed orders wrapped under the orders array.",
			"WebSocket order submission uses JSON-RPC over /ws/full with method \"v1/create_order\" or \"v2/bulk_orders\" and the same signed payload in params.",
			"GRVT order payloads carry EIP-712 signatures; signing and payload generation must be performed before benchmark timing.",
			"Authenticated requests require a GRVT session cookie and X-Grvt-Account-Id header.",
		},
	}
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
	if raw, ok := decoded["error"]; ok {
		return lifecycle.Classification{Status: lifecycle.StatusRejected, Reason: fmt.Sprint(raw)}
	}
	return generic
}
