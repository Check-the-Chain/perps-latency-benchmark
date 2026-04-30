package edgex

import (
	"encoding/json"
	"fmt"

	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/venues/spec"
)

const DefaultBaseURL = "https://pro.edgex.exchange"

func Definition() spec.Definition {
	return spec.Definition{
		Name: "edgex",
		Aliases: []string{
			"edgeX",
			"edge-x",
			"edge_x",
			"edgex_exchange",
		},
		DefaultBaseURL:  DefaultBaseURL,
		DefaultHTTPPath: "/api/v1/private/order/createOrder",
		Capabilities: spec.Capabilities{
			HTTPSingle: true,
		},
		BuilderParams: spec.BuilderParams{
			Required: []string{"contract_id", "price", "size", "metadata"},
			Defaults: map[string]any{
				"contract_id":   "10000001",
				"price":         "75000",
				"size":          "0.001",
				"side":          "BUY",
				"type":          "LIMIT",
				"time_in_force": "POST_ONLY",
			},
		},
		Classifier: classify,
		Docs: []string{
			"https://edgex-1.gitbook.io/edgeX-documentation/api/authentication",
			"https://edgex-1.gitbook.io/edgeX-documentation/api/sign",
			"https://edgexhelp.zendesk.com/hc/en-001/articles/14562599237391-Order-API",
			"https://edgex-1.gitbook.io/edgeX-documentation/api/websocket-api",
			"https://github.com/edgex-Tech/edgex-golang-sdk",
			"https://github.com/edgex-Tech/edgex-python-sdk",
		},
		Notes: []string{
			"Private REST host is verified from the official authentication docs and SDK examples.",
			"HTTP order submission uses POST /api/v1/private/order/createOrder with a JSON body containing order fields directly; there is no outer action/envelope wrapper.",
			"Private REST authentication uses X-edgeX-Api-Timestamp and X-edgeX-Api-Signature headers over timestamp, method, path, and sorted body/query content.",
			"Order payloads include precomputed L2 fields such as l2Nonce, l2Value, l2Size, l2LimitFee, l2ExpireTime, and l2Signature; build signing material outside the timed send path.",
			"The command builder uses the official edgeX Python SDK StarkEx signing and request signing helpers with caller-provided metadata, avoiding network fetches and hand-rolled L2 crypto.",
			"No official batch order creation endpoint was found in the current edgeX docs or official SDK repos, so DefaultBatchPath is intentionally empty.",
			"Official WebSocket docs and SDKs describe market-data and private account/order-update streams, not order submission over WebSocket, so DefaultWSURL and DefaultWSBatchURL are intentionally empty.",
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
	if success, ok := decoded["success"].(bool); ok && !success {
		return lifecycle.Classification{Status: lifecycle.StatusRejected, Reason: reason(decoded)}
	}
	return generic
}

func reason(value map[string]any) string {
	for _, key := range []string{"error", "message", "reason", "code"} {
		if raw, ok := value[key]; ok {
			return fmt.Sprint(raw)
		}
	}
	return ""
}
