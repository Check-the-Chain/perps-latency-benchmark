package aster

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
		Venue:    "aster",
		IDField:  "client_order_ids",
		Required: []string{"ws_url"},
	})
	if !ok || err != nil {
		return nil, err
	}
	if plan.WSURL == "" {
		return nil, nil
	}
	client, err := confirmws.Dial(ctx, plan.WSURL, http.Header{}, false)
	if err != nil {
		return nil, err
	}
	return accountfeed.NewConfirmation(client, func(msg map[string]any) (bool, error) {
		return matchAsterConfirmation(msg, plan.IDs, plan.Order)
	}), nil
}

func ConfirmCancelWebSocket(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	plan, ok, err := accountfeed.DecodePlan(built, accountfeed.PlanOptions{
		Key:      "cancel_confirmation",
		Venue:    "aster",
		IDField:  "client_order_ids",
		Required: []string{"ws_url"},
	})
	if !ok || err != nil {
		return nil, err
	}
	client, err := confirmws.Dial(ctx, plan.WSURL, http.Header{}, false)
	if err != nil {
		return nil, err
	}
	return accountfeed.NewCancelConfirmation(client, plan.IDs, matchAsterCancelConfirmation), nil
}

func matchAsterConfirmation(msg map[string]any, clientIDs map[string]struct{}, orderType string) (bool, error) {
	if confirmutil.Text(msg["e"]) != "ORDER_TRADE_UPDATE" {
		return false, nil
	}
	order, ok := msg["o"].(map[string]any)
	if !ok || !confirmutil.HasID(clientIDs, order["c"], order["C"]) {
		return false, nil
	}
	status := strings.ToUpper(confirmutil.Text(order["X"]))
	execution := strings.ToUpper(confirmutil.Text(order["x"]))
	if orderType == "market" || orderType == "ioc" || orderType == "fok" {
		if status == "FILLED" || execution == "TRADE" {
			return true, nil
		}
		if isAsterTerminalFailure(status) {
			return false, fmt.Errorf("aster order %s", status)
		}
		return false, nil
	}
	if isAsterTerminalFailure(status) {
		return false, fmt.Errorf("aster order %s", status)
	}
	return true, nil
}

func matchAsterCancelConfirmation(msg map[string]any, remaining map[string]struct{}) bool {
	if confirmutil.Text(msg["e"]) != "ORDER_TRADE_UPDATE" {
		return false
	}
	order, ok := msg["o"].(map[string]any)
	if !ok {
		return false
	}
	id := confirmutil.FirstMatchingID(remaining, order["c"], order["C"])
	if id == "" {
		return false
	}
	status := strings.ToUpper(confirmutil.Text(order["X"]))
	execution := strings.ToUpper(confirmutil.Text(order["x"]))
	if status == "CANCELED" || status == "CANCELLED" || execution == "CANCELED" || execution == "CANCELLED" {
		delete(remaining, id)
	}
	return len(remaining) == 0
}

func isAsterTerminalFailure(status string) bool {
	switch status {
	case "CANCELED", "CANCELLED", "REJECTED", "EXPIRED":
		return true
	default:
		return strings.Contains(status, "REJECT") || strings.Contains(status, "EXPIRE")
	}
}
