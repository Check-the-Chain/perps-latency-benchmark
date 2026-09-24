package accountfeed

import (
	"context"
	"testing"

	"perps-latency-benchmark/internal/payload"
)

func TestNewConfirmationIgnoresOtherVenue(t *testing.T) {
	called := false
	confirmation, err := NewConfirmation(context.Background(), payload.Built{Metadata: map[string]any{
		"confirmation": map[string]any{"venue": "venue-b"},
	}}, PlanOptions{Key: "confirmation", Venue: "venue-a"}, func(Plan) (ConfirmationBinding, error) {
		called = true
		return ConfirmationBinding{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if confirmation != nil {
		t.Fatal("unexpected confirmation")
	}
	if called {
		t.Fatal("builder was called for another venue")
	}
}
