package edgex

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/venues/spec"
)

const DefaultBaseURL = "https://pro.edgex.exchange"
const DefaultHTTPPath = "/api/v1/private/order/createOrder"

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
		DefaultHTTPPath: DefaultHTTPPath,
		Capabilities: spec.Capabilities{
			HTTPSingle:     true,
			Cleanup:        true,
			Neutralization: true,
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
		CleanupCommand: spec.CleanupCommand{
			Type: "persistent_command",
			Command: []string{
				"uv",
				"run",
				"--python",
				"3.13",
				"--with",
				"edgex-python-sdk",
				"--with",
				"requests",
				"python",
				filepath.FromSlash("internal/venues/edgex/cancel_payload.py"),
			},
			Description:    "cancel edgeX benchmark orders by client order id",
			OrderRefsField: "cleanup_orders",
			SkipNoRefs:     true,
		},
		Classifier:   classify,
		Confirmation: ConfirmWebSocket,
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
			"WebSocket confirmation uses the private account stream at /api/v1/private/ws and matches benchmark orders by clientOrderId.",
			"Cleanup cancels by clientOrderId and can neutralize filled inventory with reduce-only IOC orders outside the measured latency window.",
			"No official batch order creation endpoint was found in the current edgeX docs or official SDK repos, so DefaultBatchPath is intentionally empty.",
			"Official WebSocket docs and SDKs describe market-data and private account/order-update streams, not order submission over WebSocket, so DefaultWSURL and DefaultWSBatchURL remain intentionally empty.",
			"Tokyo fairness is not verified: official docs do not state the order-entry backend, sequencer, matching-engine, or operator region, and pro.edgex.exchange is fronted by Cloudflare.",
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
	if code, ok := decoded["code"].(string); ok {
		if code == "SUCCESS" {
			return lifecycle.Classification{Status: lifecycle.StatusAccepted}
		}
		return lifecycle.Classification{Status: lifecycle.StatusRejected, Reason: reason(decoded)}
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
