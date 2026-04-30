package aster

import "perps-latency-benchmark/internal/venues/spec"

const DefaultBaseURL = "https://fapi.asterdex.com"

func Definition() spec.Definition {
	return spec.Definition{
		Name:             "aster",
		Aliases:          []string{"asterdex", "aster_perpetuals", "aster-perpetuals", "aster_perp", "aster-perp"},
		DefaultBaseURL:   DefaultBaseURL,
		DefaultHTTPPath:  "/fapi/v3/order",
		DefaultBatchPath: "/fapi/v3/batchOrders",
		Capabilities: spec.Capabilities{
			HTTPSingle: true,
			HTTPBatch:  true,
		},
		Docs: []string{
			"https://github.com/asterdex/api-docs/blob/master/README.md",
			"https://github.com/asterdex/api-docs/blob/master/Aster%20API%20Overview.md",
			"https://github.com/asterdex/api-docs/blob/master/V3(Recommended)/EN/aster-finance-futures-api-v3.md",
			"https://docs.asterdex.com/product/aster-perpetuals/api/api-documentation",
			"https://github.com/asterdex/aster-connector-python",
		},
		Notes: []string{
			"Official api-docs README marks V3 as recommended; V1 new API key creation is no longer supported starting March 25, 2026.",
			"Futures V3 order submission uses POST /fapi/v3/order on the documented base endpoint.",
			"Futures V3 batch order submission uses POST /fapi/v3/batchOrders; the batchOrders form field wraps a stringified list of order objects.",
			"V3 signed requests use API Wallet/Agent parameters such as signer, nonce, and signature with application/x-www-form-urlencoded payloads; generate signing material before benchmarking so it stays outside the timed path.",
			"No SDK-backed command builder is included for Aster yet: the official aster-connector-python package is a V1 HMAC REST connector that submits requests, while the recommended V3 order flow requires API Wallet signing and the official docs currently provide signing examples rather than a reusable SDK payload builder.",
			"Official WebSocket documentation verifies market and user data streams at wss://fstream.asterdex.com, but no order-submission WebSocket endpoint was verified; DefaultWSURL is intentionally empty.",
			"The V3 futures example uses host https://fapi3.asterdex.com, but the same official page states the base endpoint is https://fapi.asterdex.com; fapi.asterdex.com/fapi/v3/time returned HTTP 200 during verification, while fapi3.asterdex.com/fapi/v3/time returned HTTP 403.",
		},
	}
}
