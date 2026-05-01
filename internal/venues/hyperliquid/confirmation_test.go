package hyperliquid

import "testing"

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
