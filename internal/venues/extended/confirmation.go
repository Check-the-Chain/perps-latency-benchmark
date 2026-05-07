package extended

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"perps-latency-benchmark/internal/accountfeed"
	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/confirmws"
	"perps-latency-benchmark/internal/payload"
	"perps-latency-benchmark/internal/venues/confirmutil"
)

func ConfirmWebSocket(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	plan, ok, err := accountfeed.DecodePlan(built, accountfeed.PlanOptions{
		Key:      "confirmation",
		Venue:    "extended",
		IDField:  "external_ids",
		Required: []string{"ws_url", "api_key"},
	})
	if !ok || err != nil {
		return nil, err
	}
	if plan.WSURL == "" {
		return nil, nil
	}
	headers := http.Header{}
	headers.Set("X-Api-Key", plan.Text("api_key"))
	headers.Set("User-Agent", "perps-latency-benchmark")
	client, err := confirmws.Dial(ctx, plan.WSURL, headers, false)
	if err != nil {
		return nil, err
	}
	return accountfeed.NewConfirmation(client, func(msg map[string]any) (bool, error) {
		return matchExtendedConfirmation(msg, plan.IDs, plan.Order)
	}), nil
}

func ConfirmCancelWebSocket(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	plan, ok, err := accountfeed.DecodePlan(built, accountfeed.PlanOptions{
		Key:      "cancel_confirmation",
		Venue:    "extended",
		IDField:  "external_ids",
		Required: []string{"ws_url", "api_key"},
	})
	if !ok || err != nil {
		return nil, err
	}
	headers := http.Header{}
	headers.Set("X-Api-Key", plan.Text("api_key"))
	headers.Set("User-Agent", "perps-latency-benchmark")
	client, err := confirmws.Dial(ctx, plan.WSURL, headers, false)
	if err != nil {
		return nil, err
	}
	return accountfeed.NewCancelConfirmation(client, plan.IDs, matchExtendedCancelConfirmation), nil
}

func matchExtendedConfirmation(msg map[string]any, externalIDs map[string]struct{}, orderType string) (bool, error) {
	data := confirmutil.Object(msg["data"])
	for _, trade := range confirmutil.ObjectList(data["trades"]) {
		if confirmutil.HasID(externalIDs, trade["externalOrderId"], trade["external_order_id"], trade["externalId"], trade["external_id"]) {
			return true, nil
		}
	}
	for _, order := range confirmutil.ObjectList(data["orders"]) {
		if !confirmutil.HasID(externalIDs, order["externalId"], order["external_id"]) {
			continue
		}
		status := strings.ToLower(confirmutil.Text(order["status"]))
		if orderType == "market" || orderType == "ioc" || orderType == "fok" {
			if status == "filled" || status == "partially_filled" || status == "partially-filled" {
				return true, nil
			}
			if isExtendedTerminalFailure(status) {
				return false, fmt.Errorf("extended order %s", status)
			}
			continue
		}
		if isExtendedTerminalFailure(status) {
			return false, fmt.Errorf("extended order %s", status)
		}
		return true, nil
	}
	return false, nil
}

func matchExtendedCancelConfirmation(msg map[string]any, remaining map[string]struct{}) bool {
	data := confirmutil.Object(msg["data"])
	for _, order := range confirmutil.ObjectList(data["orders"]) {
		id := confirmutil.FirstMatchingID(remaining, order["externalId"], order["external_id"])
		if id == "" {
			continue
		}
		status := strings.ToLower(confirmutil.Text(order["status"]))
		if status == "cancelled" || status == "canceled" {
			delete(remaining, id)
		}
	}
	return len(remaining) == 0
}

func isExtendedTerminalFailure(status string) bool {
	switch status {
	case "rejected", "cancelled", "canceled", "expired", "failed":
		return true
	default:
		return false
	}
}
