package hyperliquid

import (
	"testing"
	"time"

	"perps-latency-benchmark/internal/netlatency"
)

func TestMatchHyperliquidConfirmationAcceptsFilledMarket(t *testing.T) {
	matched, err := matchHyperliquidConfirmation(map[string]any{
		"channel": "orderUpdates",
		"data": []any{map[string]any{
			"status": "filled",
			"order":  map[string]any{"cloid": "0xabc"},
		}},
	}, map[string]struct{}{"0xabc": {}}, "market")
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Fatal("expected filled market confirmation")
	}
}

func TestMatchHyperliquidConfirmationRejectsTerminalFailure(t *testing.T) {
	matched, err := matchHyperliquidConfirmation(map[string]any{
		"channel": "orderUpdates",
		"data": []any{map[string]any{
			"status": "rejected",
			"order":  map[string]any{"cloid": "0xabc"},
		}},
	}, map[string]struct{}{"0xabc": {}}, "market")
	if err == nil {
		t.Fatal("expected rejected order error")
	}
	if matched {
		t.Fatal("did not expect terminal failure to match as success")
	}
}

func TestMatchHyperliquidConfirmationAcceptsOpenPostOnly(t *testing.T) {
	matched, err := matchHyperliquidConfirmation(map[string]any{
		"channel": "orderUpdates",
		"data": []any{map[string]any{
			"status": "open",
			"order":  map[string]any{"cloid": "0xabc"},
		}},
	}, map[string]struct{}{"0xabc": {}}, "post_only")
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Fatal("expected open post-only confirmation")
	}
}

func TestHLConfirmStartUsesCompletedWriteTime(t *testing.T) {
	start := time.Unix(10, 0).UTC()
	got := hlConfirmStart(netlatency.Trace{StartedAt: start, WroteRequestAtNS: int64(2 * time.Millisecond)})
	if want := start.Add(2 * time.Millisecond); !got.Equal(want) {
		t.Fatalf("hlConfirmStart = %s, want %s", got, want)
	}
}
