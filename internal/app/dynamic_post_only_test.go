package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"perps-latency-benchmark/internal/payload"
	"perps-latency-benchmark/internal/venues/hyperliquid"
	"perps-latency-benchmark/internal/venues/spec"
)

func TestDynamicPostOnlyHTTPPriceHookCachesHyperliquidL2Book(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/info" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["type"] != "l2Book" || body["coin"] != "BTC" {
			t.Fatalf("request body = %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"coin":"BTC","time":1777966248747,"levels":[[{"px":"100","sz":"5"}],[{"px":"101","sz":"3"}]]}`))
	}))
	defer server.Close()

	params := map[string]any{
		"post_only_price_source":     "hyperliquid_l2book_http",
		"post_only_price_offset_bps": 500,
		"post_only_price_refresh_ms": 300000,
	}
	runtime := spec.RuntimeConfig{
		BaseURL: server.URL,
		Params:  map[string]any{"symbol": "BTC"},
	}
	hook := dynamicPostOnlyHTTPPriceHook("hyperliquid", params, hyperliquid.Definition(), runtime)
	req := payload.Request{Params: map[string]any{"side": "buy"}}

	effective, metadata, err := hook(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if effective["price"] != "95" {
		t.Fatalf("price = %v", effective["price"])
	}
	if metadata["post_only_price_source"] != "hyperliquid_l2book_http" {
		t.Fatalf("metadata source = %v", metadata["post_only_price_source"])
	}
	if metadata["post_only_price_cached"] != false {
		t.Fatalf("first cached = %v", metadata["post_only_price_cached"])
	}
	if requests != 1 {
		t.Fatalf("requests after first hook = %d", requests)
	}

	_, metadata, err = hook(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if metadata["post_only_price_cached"] != true {
		t.Fatalf("second cached = %v", metadata["post_only_price_cached"])
	}
	if requests != 1 {
		t.Fatalf("requests after cached hook = %d", requests)
	}

	nextChunkHook := dynamicPostOnlyHTTPPriceHook("hyperliquid", params, hyperliquid.Definition(), runtime)
	_, metadata, err = nextChunkHook(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if metadata["post_only_price_cached"] != true {
		t.Fatalf("next chunk cached = %v", metadata["post_only_price_cached"])
	}
	if requests != 1 {
		t.Fatalf("requests after next chunk hook = %d", requests)
	}
}
