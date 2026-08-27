package bullet

import (
	"encoding/json"
	"testing"
)

func decodeFrame(t *testing.T, raw string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	return out
}

func TestMatchConfirmationOnNewOrder(t *testing.T) {
	msg := decodeFrame(t, `{"e":"orderTradeUpdate","E":1706745600000000,"o":{"s":"BTC-USD","i":100001,"co":12345,"X":"NEW","x":"NEW","T":1706745600000000,"th":"0xabc","S":"BUY","o":"LIMIT","f":"GTC","p":"50000.00","q":"0.0001"}}`)
	ids := map[string]struct{}{"12345": {}}
	ok, err := matchBulletConfirmation(msg, ids, "post_only")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("NEW order update must confirm")
	}
}

func TestMatchConfirmationIgnoresOtherClientOrderIDs(t *testing.T) {
	msg := decodeFrame(t, `{"e":"orderTradeUpdate","o":{"s":"BTC-USD","i":100001,"co":999,"X":"NEW","x":"NEW"}}`)
	ok, err := matchBulletConfirmation(msg, map[string]struct{}{"12345": {}}, "post_only")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("unrelated client order id must not confirm")
	}
}

func TestMatchConfirmationFailsOnRejection(t *testing.T) {
	msg := decodeFrame(t, `{"e":"orderTradeUpdate","o":{"s":"BTC-USD","i":100001,"co":12345,"X":"REJECTED","x":"REJECTED"}}`)
	_, err := matchBulletConfirmation(msg, map[string]struct{}{"12345": {}}, "post_only")
	if err == nil {
		t.Fatal("REJECTED must surface as a confirmation error")
	}
}

func TestMatchConfirmationRequiresFillForTakerOrders(t *testing.T) {
	newMsg := decodeFrame(t, `{"e":"orderTradeUpdate","o":{"s":"BTC-USD","i":1,"co":12345,"X":"NEW","x":"NEW"}}`)
	ok, err := matchBulletConfirmation(newMsg, map[string]struct{}{"12345": {}}, "immediate_or_cancel")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("taker confirmation must wait for a fill, not a NEW event")
	}

	fillMsg := decodeFrame(t, `{"e":"orderTradeUpdate","o":{"s":"BTC-USD","i":1,"co":12345,"X":"FILLED","x":"TRADE","l":"0.0001","L":"50000.00"}}`)
	ok, err = matchBulletConfirmation(fillMsg, map[string]struct{}{"12345": {}}, "immediate_or_cancel")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("FILLED must confirm a taker order")
	}
}

func TestMatchConfirmationPostOnlyVariantsConfirmOnNew(t *testing.T) {
	for _, orderType := range []string{"post_only", "post_only_slide", "post_only_front"} {
		msg := decodeFrame(t, `{"e":"orderTradeUpdate","o":{"s":"BTC-USD","i":1,"co":12345,"X":"NEW","x":"NEW"}}`)
		ok, err := matchBulletConfirmation(msg, map[string]struct{}{"12345": {}}, orderType)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", orderType, err)
		}
		if !ok {
			t.Fatalf("%s: NEW order update must confirm a maker order", orderType)
		}
	}
}

func TestMatchConfirmationFillOrKillRequiresFill(t *testing.T) {
	newMsg := decodeFrame(t, `{"e":"orderTradeUpdate","o":{"s":"BTC-USD","i":1,"co":12345,"X":"NEW","x":"NEW"}}`)
	ok, err := matchBulletConfirmation(newMsg, map[string]struct{}{"12345": {}}, "fill_or_kill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("fill_or_kill must not confirm on a bare NEW event")
	}

	fillMsg := decodeFrame(t, `{"e":"orderTradeUpdate","o":{"s":"BTC-USD","i":1,"co":12345,"X":"FILLED","x":"TRADE"}}`)
	ok, err = matchBulletConfirmation(fillMsg, map[string]struct{}{"12345": {}}, "fill_or_kill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("fill_or_kill must confirm on a fill")
	}
}

func TestMatchConfirmationLimitConfirmsOnEitherEvent(t *testing.T) {
	newMsg := decodeFrame(t, `{"e":"orderTradeUpdate","o":{"s":"BTC-USD","i":1,"co":12345,"X":"NEW","x":"NEW"}}`)
	ok, err := matchBulletConfirmation(newMsg, map[string]struct{}{"12345": {}}, "limit")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("limit order must confirm on NEW when it rests")
	}

	fillMsg := decodeFrame(t, `{"e":"orderTradeUpdate","o":{"s":"BTC-USD","i":2,"co":67890,"X":"FILLED","x":"TRADE"}}`)
	ok, err = matchBulletConfirmation(fillMsg, map[string]struct{}{"67890": {}}, "limit")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("limit order must also confirm on a fill when it crosses without a prior NEW")
	}
}

func TestMatchConfirmationEmptyOrderTypeStaysLenient(t *testing.T) {
	newMsg := decodeFrame(t, `{"e":"orderTradeUpdate","o":{"s":"BTC-USD","i":1,"co":12345,"X":"NEW","x":"NEW"}}`)
	ok, err := matchBulletConfirmation(newMsg, map[string]struct{}{"12345": {}}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("empty order_type (unspecified) must confirm on NEW like the lenient bucket")
	}

	fillMsg := decodeFrame(t, `{"e":"orderTradeUpdate","o":{"s":"BTC-USD","i":2,"co":67890,"X":"FILLED","x":"TRADE"}}`)
	ok, err = matchBulletConfirmation(fillMsg, map[string]struct{}{"67890": {}}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("empty order_type (unspecified) must also confirm on a fill")
	}
}

func TestMatchConfirmationUnrecognizedOrderTypeFailsLoudly(t *testing.T) {
	newMsg := decodeFrame(t, `{"e":"orderTradeUpdate","o":{"s":"BTC-USD","i":1,"co":12345,"X":"NEW","x":"NEW"}}`)
	ok, err := matchBulletConfirmation(newMsg, map[string]struct{}{"12345": {}}, "post_olny")
	if err == nil {
		t.Fatal("a typo'd, non-empty order_type must surface an error rather than silently confirming")
	}
	if ok {
		t.Fatal("a rejected classification must not also report confirmed")
	}
}

func TestMatchCancelConfirmationDrainsIDs(t *testing.T) {
	remaining := map[string]struct{}{"12345": {}, "67890": {}}
	first := decodeFrame(t, `{"e":"orderTradeUpdate","o":{"s":"BTC-USD","i":1,"co":12345,"X":"CANCELED","x":"CANCELED"}}`)
	if matchBulletCancelConfirmation(first, remaining) {
		t.Fatal("must not complete while ids remain")
	}
	second := decodeFrame(t, `{"e":"orderTradeUpdate","o":{"s":"BTC-USD","i":2,"co":67890,"X":"CANCELED","x":"CANCELED"}}`)
	if !matchBulletCancelConfirmation(second, remaining) {
		t.Fatal("must complete once all ids are cancelled")
	}
}
