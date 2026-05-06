package extended

import (
	"testing"

	"perps-latency-benchmark/internal/lifecycle"
)

func TestDefinition(t *testing.T) {
	definition := Definition()

	if definition.Name != "extended" {
		t.Fatalf("name = %s", definition.Name)
	}
	if definition.DefaultBaseURL != DefaultBaseURL {
		t.Fatalf("base url = %s", definition.DefaultBaseURL)
	}
	if definition.DefaultHTTPPath != "/api/v1/user/order" {
		t.Fatalf("http path = %s", definition.DefaultHTTPPath)
	}
	if !definition.Capabilities.Cleanup {
		t.Fatal("expected cleanup capability")
	}
	if definition.Confirmation == nil {
		t.Fatal("expected websocket confirmation factory")
	}
	if definition.DefaultWSURL != "" {
		t.Fatalf("expected no websocket order submission default, got %s", definition.DefaultWSURL)
	}
	if len(definition.Docs) == 0 {
		t.Fatal("expected docs")
	}
}

func TestClassifyExtendedStatusError(t *testing.T) {
	got := Classify(lifecycle.ResponseInput{
		StatusCode: 200,
		Body:       []byte(`{"status":"ERROR","error":{"code":"InvalidOrder","message":"bad order"}}`),
	})
	if got.Status != lifecycle.StatusRejected {
		t.Fatalf("status = %s, want rejected", got.Status)
	}
}

func TestMatchExtendedConfirmationByOrderExternalID(t *testing.T) {
	ok, err := matchExtendedConfirmation(map[string]any{
		"type": "ORDER",
		"data": map[string]any{
			"orders": []any{
				map[string]any{"externalId": "pb-1", "status": "NEW"},
			},
		},
	}, map[string]struct{}{"pb-1": {}}, "post_only")
	if err != nil {
		t.Fatalf("matchExtendedConfirmation error = %v", err)
	}
	if !ok {
		t.Fatal("expected confirmation match")
	}
}

func TestMatchExtendedConfirmationRejectsTerminalFailure(t *testing.T) {
	ok, err := matchExtendedConfirmation(map[string]any{
		"type": "ORDER",
		"data": map[string]any{
			"orders": []any{
				map[string]any{"externalId": "pb-1", "status": "REJECTED"},
			},
		},
	}, map[string]struct{}{"pb-1": {}}, "post_only")
	if err == nil {
		t.Fatal("expected terminal failure error")
	}
	if ok {
		t.Fatal("expected no successful match")
	}
}

func TestMatchExtendedConfirmationByTradeExternalID(t *testing.T) {
	ok, err := matchExtendedConfirmation(map[string]any{
		"type": "TRADE",
		"data": map[string]any{
			"trades": []any{
				map[string]any{"externalOrderId": "pb-1"},
			},
		},
	}, map[string]struct{}{"pb-1": {}}, "market")
	if err != nil {
		t.Fatalf("matchExtendedConfirmation error = %v", err)
	}
	if !ok {
		t.Fatal("expected trade confirmation match")
	}
}
