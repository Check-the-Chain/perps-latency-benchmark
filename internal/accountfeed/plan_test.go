package accountfeed

import (
	"testing"

	"perps-latency-benchmark/internal/payload"
)

func TestDecodePlan(t *testing.T) {
	built := payload.Built{Metadata: map[string]any{
		"confirmation": map[string]any{
			"venue":      "venue-a",
			"ws_url":     "wss://example.test",
			"account":    "acct",
			"order_type": "post_only",
			"ids":        []any{"a", "b"},
		},
	}}

	plan, ok, err := DecodePlan(built, PlanOptions{
		Key:      "confirmation",
		Venue:    "venue-a",
		IDField:  "ids",
		Required: []string{"ws_url", "account"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("plan not found")
	}
	if plan.WSURL != "wss://example.test" || plan.Text("account") != "acct" || plan.Order != "post_only" {
		t.Fatalf("plan = %#v", plan)
	}
	if _, ok := plan.IDs["a"]; !ok {
		t.Fatalf("ids = %#v", plan.IDs)
	}
}

func TestDecodePlanIgnoresOtherVenue(t *testing.T) {
	built := payload.Built{Metadata: map[string]any{
		"confirmation": map[string]any{"venue": "venue-b"},
	}}

	_, ok, err := DecodePlan(built, PlanOptions{Key: "confirmation", Venue: "venue-a"})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("unexpected plan")
	}
}

func TestDecodePlanRejectsMissingRequiredField(t *testing.T) {
	built := payload.Built{Metadata: map[string]any{
		"confirmation": map[string]any{"venue": "venue-a"},
	}}

	_, ok, err := DecodePlan(built, PlanOptions{
		Key:      "confirmation",
		Venue:    "venue-a",
		Required: []string{"ws_url"},
	})
	if !ok {
		t.Fatal("plan not found")
	}
	if err == nil {
		t.Fatal("expected error")
	}
}
