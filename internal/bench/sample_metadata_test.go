package bench

import (
	"testing"
	"time"
)

func TestOrderRefsFromMetadataNormalizesVenueRefs(t *testing.T) {
	metadata := map[string]any{
		"cleanup_orders": []any{
			map[string]any{"venue": "aster", "symbol": "BTCUSDT", "clientOrderId": "aster-1"},
			map[string]any{"venue": "lighter", "market_index": float64(1), "orderIndex": float64(1649732491)},
			map[string]any{"venue": "extended", "market": "BTC-USD", "externalOrderId": "ext-1"},
			map[string]any{"venue": "hyperliquid", "asset": float64(0), "cloid": "0xabc"},
		},
	}

	refs := OrderRefsFromMetadata(metadata, "cleanup_orders")
	if len(refs) != 4 {
		t.Fatalf("len(refs) = %d, want 4: %#v", len(refs), refs)
	}
	if refs[0].ClientOrderID != "aster-1" {
		t.Fatalf("aster ref = %#v", refs[0])
	}
	if refs[1].MarketIndex != 1 || refs[1].ClientOrderIndex != "1649732491" || refs[1].OrderIndex != "1649732491" {
		t.Fatalf("lighter ref = %#v", refs[1])
	}
	if refs[2].ExternalID != "ext-1" {
		t.Fatalf("extended ref = %#v", refs[2])
	}
	if refs[3].Asset != 0 || refs[3].Cloid != "0xabc" {
		t.Fatalf("hyperliquid ref = %#v", refs[3])
	}
}

func TestOrderRefContractPrefersTypedSampleRefs(t *testing.T) {
	contract := CleanupOrderRefContract("")
	sample := Sample{
		OrderRefs: []OrderRef{{Venue: "typed", ClientOrderID: "typed-1"}},
		Metadata: map[string]any{
			"cleanup_orders": []any{map[string]any{"venue": "metadata", "client_order_id": "metadata-1"}},
		},
	}

	refs := contract.FromSample(sample)
	if len(refs) != 1 || refs[0].Venue != "typed" || refs[0].ClientOrderID != "typed-1" {
		t.Fatalf("refs = %#v", refs)
	}
}

func TestOrderRefContractWritesConfiguredMetadataField(t *testing.T) {
	contract := CleanupOrderRefContract("custom_refs")
	metadata := contract.PutMetadata(nil, []OrderRef{{Venue: "venue-a", ClientOrderID: "order-1"}})

	refs := OrderRefsFromMetadata(metadata, "custom_refs")
	if len(refs) != 1 || refs[0].ClientOrderID != "order-1" {
		t.Fatalf("metadata = %#v refs = %#v", metadata, refs)
	}
}

func TestDebugMetadataStripsRequiredBenchmarkSemantics(t *testing.T) {
	metadata := map[string]any{
		"builder":             "test",
		"cleanup_orders":      []any{map[string]any{"client_order_id": "entry-1"}},
		"cleanup_metadata":    map[string]any{"cleanup_orders": []any{map[string]any{"client_order_id": "exit-1"}}},
		"expected_entry_fill": map[string]any{"expected_price": 100.0},
		"expected_exit_fill":  map[string]any{"expected_price": 99.0},
		"speed_bump_ns":       float64(1_000_000),
		"speed_bump_source":   "venue docs",
		"order_type":          "market",
	}

	got := DebugMetadata(metadata)
	if got["builder"] != "test" {
		t.Fatalf("builder missing: %#v", got)
	}
	for _, key := range []string{"cleanup_orders", "cleanup_metadata", "expected_entry_fill", "expected_exit_fill", "speed_bump_ns", "speed_bump_source", "order_type"} {
		if _, ok := got[key]; ok {
			t.Fatalf("%s leaked into debug metadata: %#v", key, got)
		}
	}
}

func TestExpectedFillFromMetadata(t *testing.T) {
	receivedAt := time.Now().UTC().Truncate(time.Nanosecond)
	exchangeAt := receivedAt.Add(-time.Millisecond)
	fill := ExpectedFillFromMetadata(map[string]any{
		"expected_entry_fill": map[string]any{
			"phase":            "entry",
			"side":             "buy",
			"size":             "0.001",
			"expected_price":   "100000.50",
			"top_sufficient":   true,
			"book_age_ns":      "1000",
			"book_received_at": receivedAt.Format(time.RFC3339Nano),
			"book_exchange_at": exchangeAt.Format(time.RFC3339Nano),
		},
	}, "expected_entry_fill")

	if fill == nil || fill.Phase != "entry" || fill.Side != "buy" || fill.Size != 0.001 || fill.ExpectedPrice != 100000.50 || !fill.TopSufficient || fill.BookAgeNS != 1000 {
		t.Fatalf("fill = %#v", fill)
	}
	if !fill.BookReceivedAt.Equal(receivedAt) || fill.BookExchangeAt == nil || !fill.BookExchangeAt.Equal(exchangeAt) {
		t.Fatalf("fill timestamps = %#v", fill)
	}
}
