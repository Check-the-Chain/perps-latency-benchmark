package variational_omni

import (
	"encoding/json"
	"fmt"

	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/venues/spec"
)

const (
	ReadOnlyBaseURL   = "https://omni-client-api.prod.ap-northeast-1.variational.io"
	ReadOnlyStatsPath = "/metadata/stats"
	DefaultBaseURL    = "https://api.variational.io/v1"
	DefaultHTTPPath   = "/status"
)

func Definition() spec.Definition {
	return spec.Definition{
		Name: "variational_omni",
		Aliases: []string{
			"variational-omni",
			"variational",
			"omni",
		},
		Docs: []string{
			"https://docs.variational.io/technical-documentation/api",
			"https://docs.variational.io/for-developers/api/endpoints",
			"https://docs.variational.io/for-developers/api/authentication",
			"https://docs.variational.io/official-links",
			"https://github.com/variational-research/variational-sdk-python",
		},
		DefaultBaseURL:  DefaultBaseURL,
		DefaultHTTPPath: DefaultHTTPPath,
		Capabilities: spec.Capabilities{
			HTTPSingle: true,
		},
		BuilderParams: spec.BuilderParams{
			Defaults: map[string]any{
				"action":                "status",
				"instrument_type":       "perpetual_future",
				"underlying":            "BTC",
				"settlement_asset":      "USDC",
				"side":                  "buy",
				"qty":                   "0.001",
				"expires_after_seconds": 30,
			},
		},
		Classifier: classify,
		Notes: []string{
			"Current official Omni docs verify a read-only stats API at " + ReadOnlyBaseURL + ReadOnlyStatsPath + ".",
			"Current official Omni docs state the Omni trading API is still in development and not available to users; live trading access must be confirmed with Variational before benchmark results are comparable.",
			"The public Variational Python SDK exposes HMAC-signed RFQ/portfolio endpoints under /v1; this venue builder signs the same headers without doing network work inside the timed path.",
			"Default action is an authenticated /status smoke check. For RFQ submission, set params.action=create_rfq and provide target_companies from the live API account.",
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
	if raw, ok := decoded["error"]; ok && raw != nil {
		return lifecycle.Classification{Status: lifecycle.StatusRejected, Reason: fmt.Sprint(raw)}
	}
	return generic
}
