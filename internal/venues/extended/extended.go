package extended

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

const DefaultBaseURL = "https://api.starknet.extended.exchange"
const DefaultHTTPPath = "/api/v1/user/order"

func Definition() spec.Definition {
	return spec.Definition{
		Name:            "extended",
		Aliases:         []string{"extended_exchange"},
		DefaultBaseURL:  DefaultBaseURL,
		DefaultHTTPPath: DefaultHTTPPath,
		Capabilities: spec.Capabilities{
			HTTPSingle:     true,
			HTTPBatch:      true,
			Cleanup:        true,
			Neutralization: true,
		},
		BuilderParams: spec.BuilderParams{
			Required: []string{"size", "price"},
			Defaults: map[string]any{
				"env":           "mainnet",
				"market":        "BTC-USD",
				"side":          "buy",
				"size":          "0.001",
				"price":         "75000",
				"time_in_force": "GTT",
				"post_only":     true,
				"fee":           "0.0002",
				"l2_config": map[string]any{
					"type":                  "STARKX",
					"synthetic_id":          "0x4254432d3600000000000000000000",
					"synthetic_resolution":  1000000,
					"collateral_id":         "0x1",
					"collateral_resolution": 1000000,
				},
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
				"x10-python-trading-starknet",
				"python",
				filepath.FromSlash("internal/venues/extended/cancel_payload.py"),
			},
			Description:    "cancel extended benchmark orders by external id",
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
				"python",
				filepath.FromSlash("internal/venues/extended/cost_payload.py"),
			},
			Timeout: 60 * time.Second,
		},
		BookTop: spec.BookTop{
			Build: func(runtime spec.RuntimeConfig) (booktop.Config, bool) {
				market := spec.TextParam(runtime.Params, "market", "BTC-USD")
				baseURL := spec.CoalesceURL(runtime.WSURL, strings.Replace(DefaultBaseURL, "https://", "wss://", 1))
				if baseURL == "" || market == "" {
					return booktop.Config{}, false
				}
				return booktop.Config{
					URL:    baseURL + "/stream.extended.exchange/v1/orderbooks/" + market,
					Symbol: market,
					Parser: booktop.NewExtendedParser(),
				}, true
			},
		},
		ExpectedFill: spec.ExpectedFill{
			Build: func(runtime spec.RuntimeConfig) (spec.ExpectedFillOrder, bool) {
				return spec.ExpectedFillOrder{
					Side: spec.TextParam(runtime.Params, "side", "buy"),
					Size: spec.FloatParam(runtime.Params, "size"),
				}, true
			},
		},
		Classifier:         classify,
		Confirmation:       ConfirmWebSocket,
		CancelConfirmation: ConfirmCancelWebSocket,
		Docs: []string{
			"https://api.docs.extended.exchange/",
		},
		Notes: []string{
			"REST order creation uses POST /api/v1/user/order.",
			"Batch benchmarking uses concurrent POST /api/v1/user/order requests because the official docs do not verify a native batch order endpoint.",
			"The official x10-python-trading-starknet SDK can create signed order objects without submitting them; submission is a separate place_order call.",
			"Official WebSocket docs describe public/private streams and account updates, not order submission; DefaultWSURL is intentionally unset for order benchmarking.",
			"Order management requires Stark key signatures prepared outside the timed network path.",
			"WebSocket confirmation uses the private account stream at /stream.extended.exchange/v1/account and matches benchmark orders by externalId.",
			"Cleanup cancels by externalId and can neutralize filled inventory with reduce-only IOC orders outside the measured latency window.",
			"Official API docs state the exchange servers are hosted in AWS Tokyo region ap-northeast-1, AZ ap-northeast-1a.",
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
	if status, ok := decoded["status"].(string); ok {
		switch status {
		case "OK", "ok":
			return lifecycle.Classification{Status: lifecycle.StatusAccepted}
		case "ERROR", "error":
			return lifecycle.Classification{Status: lifecycle.StatusRejected, Reason: reason(decoded)}
		}
	}
	if raw, ok := decoded["error"]; ok {
		return lifecycle.Classification{Status: lifecycle.StatusRejected, Reason: fmt.Sprint(raw)}
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
