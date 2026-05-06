package aster

import (
	"context"
	"slices"
	"testing"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/lifecycle"
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
	if definition.DefaultWSURL != "wss://fstream.asterdex.com" {
		t.Fatalf("DefaultWSURL = %q", definition.DefaultWSURL)
	}
	if !definition.Capabilities.Cleanup {
		t.Fatal("expected cleanup capability")
	}
	if definition.Confirmation == nil {
		t.Fatal("expected confirmation factory")
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
			Body:      "symbol=BTCUSDT&type=LIMIT&side=BUY&timeInForce=GTX&quantity=0.001&price=75000&nonce=1&signature=0x0",
			BatchBody: "batchOrders=%5B%7B%22symbol%22%3A%22BTCUSDT%22%7D%5D&nonce=1&signature=0x0",
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

func TestClassifyAsterCodeFailure(t *testing.T) {
	got := Classify(lifecycle.ResponseInput{
		StatusCode: 200,
		Body:       []byte(`{"code":-2010,"msg":"NEW_ORDER_REJECTED"}`),
	})
	if got.Status != lifecycle.StatusRejected {
		t.Fatalf("status = %s, want rejected", got.Status)
	}
}

func TestMatchAsterConfirmationByClientOrderID(t *testing.T) {
	ok, err := matchAsterConfirmation(map[string]any{
		"e": "ORDER_TRADE_UPDATE",
		"o": map[string]any{"c": "pb_123", "X": "NEW", "x": "NEW"},
	}, map[string]struct{}{"pb_123": {}}, "post_only")
	if err != nil {
		t.Fatalf("matchAsterConfirmation error = %v", err)
	}
	if !ok {
		t.Fatal("expected confirmation match")
	}
}

func TestMatchAsterConfirmationRejectsTerminalFailure(t *testing.T) {
	ok, err := matchAsterConfirmation(map[string]any{
		"e": "ORDER_TRADE_UPDATE",
		"o": map[string]any{"c": "pb_123", "X": "REJECTED", "x": "REJECTED"},
	}, map[string]struct{}{"pb_123": {}}, "post_only")
	if err == nil {
		t.Fatal("expected terminal failure error")
	}
	if ok {
		t.Fatal("expected no successful match")
	}
}
