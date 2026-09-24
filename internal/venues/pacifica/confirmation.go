package pacifica

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
	return accountfeed.NewConfirmation(ctx, built, accountfeed.PlanOptions{
		Key:      "confirmation",
		Venue:    "pacifica",
		IDField:  "client_order_ids",
		Required: []string{"ws_url", "account"},
	}, func(plan accountfeed.Plan) (accountfeed.ConfirmationBinding, error) {
		account := plan.Text("account")
		return accountfeed.ConfirmationBinding{
			FeedKey: accountfeed.FeedKey("pacifica", plan.WSURL, account),
			Options: accountfeed.FeedOptions{
				Dial: func(ctx context.Context) (*confirmws.Client, error) {
					return dialAccountOrderUpdates(ctx, plan.WSURL, account)
				},
			},
			Match: func(msg map[string]any) (bool, error) {
				return matchPacificaConfirmation(msg, plan.IDs, plan.Order)
			},
		}, nil
	})
}

func ConfirmCancelWebSocket(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	return accountfeed.NewCancelConfirmation(ctx, built, accountfeed.PlanOptions{
		Key:      "cancel_confirmation",
		Venue:    "pacifica",
		IDField:  "client_order_ids",
		Required: []string{"ws_url", "account"},
	}, func(plan accountfeed.Plan) (accountfeed.CancelConfirmationBinding, error) {
		account := plan.Text("account")
		return accountfeed.CancelConfirmationBinding{
			FeedKey: accountfeed.FeedKey("pacifica", plan.WSURL, account),
			Options: accountfeed.FeedOptions{
				Dial: func(ctx context.Context) (*confirmws.Client, error) {
					return dialAccountOrderUpdates(ctx, plan.WSURL, account)
				},
			},
			Match: matchPacificaCancelConfirmation,
		}, nil
	})
}

func dialAccountOrderUpdates(ctx context.Context, wsURL string, account string) (*confirmws.Client, error) {
	client, err := confirmws.Dial(ctx, wsURL, http.Header{}, false)
	if err != nil {
		return nil, err
	}
	if err := client.WriteJSON(ctx, map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"source":  "account_order_updates",
			"account": account,
		},
	}); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func matchPacificaConfirmation(msg map[string]any, clientIDs map[string]struct{}, orderType string) (bool, error) {
	if channel := confirmutil.Text(msg["channel"]); channel != "" && channel != "account_order_updates" {
		return false, nil
	}
	for _, order := range pacificaOrders(msg) {
		if !confirmutil.HasID(clientIDs, order["I"], order["client_order_id"], order["clientOrderId"]) {
			continue
		}
		status := strings.ToLower(confirmutil.Text(order["os"]))
		event := strings.ToLower(confirmutil.Text(order["oe"]))
		if orderType == "market" || orderType == "ioc" {
			if status == "filled" || status == "partially_filled" || event == "fulfill_market" || event == "fulfill_limit" {
				return true, nil
			}
			if isPacificaTerminalFailure(status, event) {
				return false, fmt.Errorf("pacifica order %s", firstNonEmptyText(status, event))
			}
			continue
		}
		if isPacificaTerminalFailure(status, event) {
			return false, fmt.Errorf("pacifica order %s", firstNonEmptyText(status, event))
		}
		return true, nil
	}
	return false, nil
}

func matchPacificaCancelConfirmation(msg map[string]any, remaining map[string]struct{}) bool {
	if channel := confirmutil.Text(msg["channel"]); channel != "" && channel != "account_order_updates" {
		return false
	}
	for _, order := range pacificaOrders(msg) {
		id := confirmutil.FirstMatchingID(remaining, order["I"], order["client_order_id"], order["clientOrderId"])
		if id == "" {
			continue
		}
		status := strings.ToLower(confirmutil.Text(order["os"]))
		event := strings.ToLower(confirmutil.Text(order["oe"]))
		if status == "cancelled" || status == "canceled" || event == "cancel" || event == "force_cancel" {
			delete(remaining, id)
		}
	}
	return len(remaining) == 0
}

func pacificaOrders(msg map[string]any) []map[string]any {
	if orders := confirmutil.ObjectList(msg["data"]); len(orders) > 0 {
		return orders
	}
	data := confirmutil.Object(msg["data"])
	if len(data) == 0 {
		return nil
	}
	return []map[string]any{data}
}

func isPacificaTerminalFailure(status string, event string) bool {
	switch status {
	case "cancelled", "canceled", "rejected", "expired":
		return true
	}
	switch event {
	case "post_only_rejected", "self_trade_prevented", "force_cancel", "expired":
		return true
	default:
		return false
	}
}

func firstNonEmptyText(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "terminal"
}
