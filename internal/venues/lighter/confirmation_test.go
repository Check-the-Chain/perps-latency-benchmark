package lighter

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
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

func TestMatchLighterCancelConfirmationWaitsForAllOrders(t *testing.T) {
	remaining := map[string]struct{}{"1234": {}, "1235": {}}
	first := matchLighterCancelConfirmation(map[string]any{
		"channel": "account_orders:1",
		"orders": map[string]any{
			"1": []any{map[string]any{"client_order_index": float64(1234), "status": "canceled-by-user"}},
		},
	}, "1", remaining)
	if first {
		t.Fatal("expected first cancel update to leave one order outstanding")
	}
	second := matchLighterCancelConfirmation(map[string]any{
		"channel": "account_orders:1",
		"orders": map[string]any{
			"1": []any{map[string]any{"client_order_id": "1235", "status": "cancelled"}},
		},
	}, "1", remaining)
	if !second {
		t.Fatalf("expected all cancels confirmed, remaining = %#v", remaining)
	}
}

func TestLighterAuthExpirationParsesJWTExp(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour).Truncate(time.Second).UTC()
	token := testJWT(map[string]any{"exp": expiresAt.Unix()})

	got := lighterAuthExpiration(token)
	if !got.Equal(expiresAt) {
		t.Fatalf("expiration = %s, want %s", got, expiresAt)
	}
}

func TestLighterAuthExpirationIgnoresOpaqueToken(t *testing.T) {
	if got := lighterAuthExpiration("opaque-token"); !got.IsZero() {
		t.Fatalf("expiration = %s, want zero", got)
	}
}

func testJWT(claims map[string]any) string {
	header, _ := json.Marshal(map[string]any{"alg": "none"})
	payload, _ := json.Marshal(claims)
	return base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload) + "."
}
