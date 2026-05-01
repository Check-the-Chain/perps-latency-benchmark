package lighter

import (
	"testing"
	"time"

	"perps-latency-benchmark/internal/netlatency"
)

func TestMatchLighterConfirmationByTradeClientID(t *testing.T) {
	matched, err := matchLighterConfirmation(map[string]any{
		"channel": "account_all_trades:9",
		"trades": map[string]any{
			"1": []any{map[string]any{"bid_client_id": float64(1234)}},
		},
	}, "1", map[string]struct{}{"1234": {}}, "market")
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Fatal("expected trade client id match")
	}
}

func TestMatchLighterConfirmationRejectsMarketCancel(t *testing.T) {
	matched, err := matchLighterConfirmation(map[string]any{
		"channel": "account_orders:1",
		"orders": map[string]any{
			"1": []any{map[string]any{"client_order_index": float64(1234), "status": "canceled-too-much-slippage"}},
		},
	}, "1", map[string]struct{}{"1234": {}}, "market")
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if matched {
		t.Fatal("did not expect cancellation to match as success")
	}
}

func TestMatchLighterConfirmationAcceptsPostOnlyOpen(t *testing.T) {
	matched, err := matchLighterConfirmation(map[string]any{
		"channel": "account_orders:1",
		"orders": map[string]any{
			"1": []any{map[string]any{"client_order_id": "1234", "status": "open"}},
		},
	}, "1", map[string]struct{}{"1234": {}}, "post_only")
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Fatal("expected open post-only order confirmation")
	}
}

func TestConfirmStartUsesCompletedWriteTime(t *testing.T) {
	start := time.Unix(10, 0).UTC()
	got := confirmStart(netlatency.Trace{StartedAt: start, WroteRequestAtNS: int64(2 * time.Millisecond)})
	if want := start.Add(2 * time.Millisecond); !got.Equal(want) {
		t.Fatalf("confirmStart = %s, want %s", got, want)
	}
}
