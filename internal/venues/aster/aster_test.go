package aster

import (
	"context"
	"slices"
	"testing"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/venues/prebuilt"
	"perps-latency-benchmark/internal/venues/spec"
)

func TestDefinitionDocumentsVerifiedAsterDefaults(t *testing.T) {
	definition := Definition()

	if definition.Name != "aster" {
		t.Fatalf("Name = %q", definition.Name)
	}
	if definition.DefaultBaseURL != "https://fapi.asterdex.com" {
		t.Fatalf("DefaultBaseURL = %q", definition.DefaultBaseURL)
	}
	if definition.DefaultHTTPPath != "/fapi/v3/order" {
		t.Fatalf("DefaultHTTPPath = %q", definition.DefaultHTTPPath)
	}
	if definition.DefaultBatchPath != "/fapi/v3/batchOrders" {
		t.Fatalf("DefaultBatchPath = %q", definition.DefaultBatchPath)
	}
	if definition.DefaultWSURL != "" {
		t.Fatalf("DefaultWSURL = %q, want empty until order submission WS is verified", definition.DefaultWSURL)
	}
	if len(definition.Docs) == 0 {
		t.Fatal("Docs is empty")
	}
	if len(definition.Notes) == 0 {
		t.Fatal("Notes is empty")
	}
}

func TestDefinitionAliases(t *testing.T) {
	names := Definition().Names()
	for _, name := range []string{"aster", "asterdex", "aster_perpetuals", "aster_perp"} {
		if !slices.Contains(names, name) {
			t.Fatalf("Names() = %v, missing %q", names, name)
		}
	}
}

func TestDefinitionBuildsDefaultOrderURLs(t *testing.T) {
	venue, err := Definition().Build(spec.Config{
		Request: prebuilt.Config{
			Body:      "symbol=ASTERUSDT&type=LIMIT&side=BUY&timeInForce=GTC&quantity=20&price=0.5&signer=0x0&nonce=1&signature=0x0",
			BatchBody: "batchOrders=%5B%7B%27symbol%27%3A%27ASTERUSDT%27%7D%5D&signer=0x0&nonce=1&signature=0x0",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := venue.Prepare(context.Background(), bench.ScenarioSingle, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Request.URL != "https://fapi.asterdex.com/fapi/v3/order" {
		t.Fatalf("single URL = %q", prepared.Request.URL)
	}

	prepared, err = venue.Prepare(context.Background(), bench.ScenarioBatch, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Request.URL != "https://fapi.asterdex.com/fapi/v3/batchOrders" {
		t.Fatalf("batch URL = %q", prepared.Request.URL)
	}
}
