package variational_omni

import "perps-latency-benchmark/internal/venues/spec"

const (
	ReadOnlyBaseURL   = "https://omni-client-api.prod.ap-northeast-1.variational.io"
	ReadOnlyStatsPath = "/metadata/stats"
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
			"https://docs.variational.io/official-links",
			"https://github.com/variational-research/variational-sdk-python",
		},
		Notes: []string{
			"Current official Omni API docs only verify a read-only stats API at " + ReadOnlyBaseURL + ReadOnlyStatsPath + ".",
			"Current official docs state the Omni trading API is still in development and not available to users, so HTTP order/tx, batch, and WebSocket submission defaults are intentionally empty.",
			"The public variational Python SDK targets legacy RFQ/portfolio endpoints and signs direct JSON requests with X-Request-Timestamp-Ms, X-Variational-Key, and X-Variational-Signature headers; it does not provide a verified Omni order payload builder.",
		},
	}
}
