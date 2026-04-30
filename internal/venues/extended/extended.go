package extended

import (
	"encoding/json"
	"fmt"

	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/venues/spec"
)

const DefaultBaseURL = "https://api.starknet.extended.exchange"

func Definition() spec.Definition {
	return spec.Definition{
		Name:            "extended",
		Aliases:         []string{"extended_exchange"},
		DefaultBaseURL:  DefaultBaseURL,
		DefaultHTTPPath: "/api/v1/user/order",
		Capabilities: spec.Capabilities{
			HTTPSingle: true,
		},
		BuilderParams: spec.BuilderParams{Required: []string{"size", "price"}},
		Classifier:    classify,
		Docs: []string{
			"https://api.docs.extended.exchange/",
		},
		Notes: []string{
			"REST order creation uses POST /api/v1/user/order.",
			"The official x10-python-trading-starknet SDK can create signed order objects without submitting them; submission is a separate place_order call.",
			"Official WebSocket docs describe public/private streams and account updates, not order submission; DefaultWSURL is intentionally unset for order benchmarking.",
			"Order management requires Stark key signatures prepared outside the timed network path.",
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
