package hyperliquid

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestMatchHyperliquidConfirmationRejectsPerpMarginRejected(t *testing.T) {
	matched, err := matchHyperliquidConfirmation(map[string]any{
		"channel": "orderUpdates",
		"data": []any{map[string]any{
			"status": "perpMarginRejected",
			"order":  map[string]any{"cloid": "0xabc"},
		}},
	}, map[string]struct{}{"0xabc": {}}, "post_only")
	if err == nil {
		t.Fatal("expected perp margin rejected order error")
	}
	if matched {
		t.Fatal("did not expect terminal failure to match as success")
	}
}

func TestMatchHyperliquidConfirmationRejectsMixedCaseCanceledStatus(t *testing.T) {
	matched, err := matchHyperliquidConfirmation(map[string]any{
		"channel": "orderUpdates",
		"data": []any{map[string]any{
			"status": "selfTradeCanceled",
			"order":  map[string]any{"cloid": "0xabc"},
		}},
	}, map[string]struct{}{"0xabc": {}}, "post_only")
	if err == nil {
		t.Fatal("expected canceled order error")
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

func TestMatchHyperliquidCancelConfirmationWaitsForAllOrders(t *testing.T) {
	remaining := map[string]struct{}{"0xabc": {}, "0xdef": {}}
	first := matchHyperliquidCancelConfirmation(map[string]any{
		"channel": "orderUpdates",
		"data": []any{map[string]any{
			"status": "canceled",
			"order":  map[string]any{"cloid": "0xabc"},
		}},
	}, remaining)
	if first {
		t.Fatal("expected first cancel update to leave one order outstanding")
	}
	second := matchHyperliquidCancelConfirmation(map[string]any{
		"channel": "orderUpdates",
		"data": []any{map[string]any{
			"status": "selfTradeCanceled",
			"order":  map[string]any{"cloid": "0xdef"},
		}},
	}, remaining)
	if !second {
		t.Fatalf("expected all cancels confirmed, remaining = %#v", remaining)
	}
}

func TestHyperliquidAPIBaseURLFromWebSocketURL(t *testing.T) {
	if got := hyperliquidAPIBaseURL("wss://api.hyperliquid.xyz/ws"); got != "https://api.hyperliquid.xyz" {
		t.Fatalf("hyperliquidAPIBaseURL() = %q", got)
	}
	if got := hyperliquidAPIBaseURL("ws://127.0.0.1:8080/ws"); got != "http://127.0.0.1:8080" {
		t.Fatalf("hyperliquidAPIBaseURL() = %q", got)
	}
}

func TestVerifyHyperliquidCancelByOpenOrdersAcceptsMissingCloids(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/info" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["type"] != "openOrders" || body["user"] != "0xabc" {
			t.Fatalf("unexpected body %#v", body)
		}
		_, _ = w.Write([]byte(`[{"cloid":"0xopen"}]`))
	}))
	defer server.Close()

	result, ok, err := verifyHyperliquidCancelByOpenOrders(context.Background(), "ws"+server.URL[len("http"):]+"/ws", "0xabc", map[string]struct{}{"0xcanceled": {}})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("verification ok = false, want true")
	}
	if result.Trace.Transport != "http_state_poll" || result.BytesRead == 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestVerifyHyperliquidCancelByOpenOrdersRejectsRemainingCloids(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"cloid":"0xopen"}]`))
	}))
	defer server.Close()

	_, ok, err := verifyHyperliquidCancelByOpenOrders(context.Background(), "ws"+server.URL[len("http"):]+"/ws", "0xabc", map[string]struct{}{"0xopen": {}})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("verification ok = true, want false")
	}
}
