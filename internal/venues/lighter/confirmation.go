package lighter

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"perps-latency-benchmark/internal/accountfeed"
	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/confirmws"
	"perps-latency-benchmark/internal/payload"
	"perps-latency-benchmark/internal/venues/confirmutil"
)

func ConfirmWebSocket(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	plan, ok, err := accountfeed.DecodePlan(built, accountfeed.PlanOptions{
		Key:      "confirmation",
		Venue:    "lighter",
		IDField:  "order_indices",
		Required: []string{"ws_url", "auth_token", "account_index", "market_index"},
	})
	if !ok || err != nil {
		return nil, err
	}
	marketIndex := plan.Text("market_index")
	client, err := dialAccountFeed(ctx, plan.WSURL, plan.Text("auth_token"), plan.Text("account_index"), marketIndex, true)
	if err != nil {
		return nil, err
	}
	confirmation := accountfeed.NewConfirmation(client, func(msg map[string]any) (bool, error) {
		return matchLighterConfirmation(msg, marketIndex, plan.IDs, plan.Order)
	})
	return confirmation, nil
}

func ConfirmCancelWebSocket(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	plan, ok, err := accountfeed.DecodePlan(built, accountfeed.PlanOptions{
		Key:      "cancel_confirmation",
		Venue:    "lighter",
		IDField:  "order_indices",
		Required: []string{"ws_url", "auth_token", "account_index", "market_index"},
	})
	if !ok || err != nil {
		return nil, err
	}
	marketIndex := plan.Text("market_index")
	client, err := dialAccountFeed(ctx, plan.WSURL, plan.Text("auth_token"), plan.Text("account_index"), marketIndex, false)
	if err != nil {
		return nil, err
	}
	return accountfeed.NewCancelConfirmation(client, plan.IDs, func(msg map[string]any, remaining map[string]struct{}) bool {
		return matchLighterCancelConfirmation(msg, marketIndex, remaining)
	}), nil
}

func dialAccountFeed(ctx context.Context, wsURL string, auth string, accountIndex string, marketIndex string, includeTrades bool) (*confirmws.Client, error) {
	client, err := confirmws.Dial(ctx, wsURL, http.Header{}, true)
	if err != nil {
		return nil, err
	}
	subscribeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.WriteJSON(subscribeCtx, map[string]any{
		"type":    "subscribe",
		"channel": fmt.Sprintf("account_orders/%s/%s", marketIndex, accountIndex),
		"auth":    auth,
	}); err != nil {
		_ = client.Close()
		return nil, err
	}
	if err := client.DrainUntil(subscribeCtx, func(msg map[string]any) bool {
		return strings.HasPrefix(confirmutil.Text(msg["channel"]), "account_orders:")
	}); err != nil {
		_ = client.Close()
		return nil, err
	}
	if !includeTrades {
		return client, nil
	}
	if err := client.WriteJSON(subscribeCtx, map[string]any{
		"type":    "subscribe",
		"channel": fmt.Sprintf("account_all_trades/%s", accountIndex),
		"auth":    auth,
	}); err != nil {
		_ = client.Close()
		return nil, err
	}
	if err := client.DrainUntil(subscribeCtx, func(msg map[string]any) bool {
		return strings.HasPrefix(confirmutil.Text(msg["channel"]), "account_all_trades:")
	}); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func matchLighterConfirmation(msg map[string]any, marketIndex string, orderIDs map[string]struct{}, orderType string) (bool, error) {
	for _, trade := range marketTrades(msg, marketIndex) {
		if confirmutil.HasID(orderIDs, trade["ask_client_id"], trade["ask_client_id_str"], trade["bid_client_id"], trade["bid_client_id_str"]) {
			return true, nil
		}
	}
	for _, order := range marketOrders(msg, marketIndex) {
		if !confirmutil.HasID(orderIDs, order["client_order_index"], order["client_order_id"], order["order_index"], order["order_id"]) {
			continue
		}
		status := strings.ToLower(confirmutil.Text(order["status"]))
		if orderType == "market" || orderType == "ioc" {
			if status == "filled" {
				return true, nil
			}
			if strings.HasPrefix(status, "canceled") {
				return false, fmt.Errorf("lighter order %s", status)
			}
			continue
		}
		if strings.HasPrefix(status, "canceled") && status != "canceled-post-only" {
			return false, fmt.Errorf("lighter order %s", status)
		}
		return true, nil
	}
	return false, nil
}

func matchLighterCancelConfirmation(msg map[string]any, marketIndex string, remaining map[string]struct{}) bool {
	for _, order := range marketOrders(msg, marketIndex) {
		id := confirmutil.FirstMatchingID(remaining, order["client_order_index"], order["client_order_id"], order["order_index"], order["order_id"])
		if id == "" {
			continue
		}
		status := strings.ToLower(confirmutil.Text(order["status"]))
		if strings.HasPrefix(status, "canceled") || strings.HasPrefix(status, "cancelled") {
			delete(remaining, id)
		}
	}
	return len(remaining) == 0
}

func marketOrders(msg map[string]any, marketIndex string) []map[string]any {
	rawOrders, ok := msg["orders"].(map[string]any)
	if !ok {
		return nil
	}
	return confirmutil.ObjectList(rawOrders[marketIndex])
}

func marketTrades(msg map[string]any, marketIndex string) []map[string]any {
	rawTrades, ok := msg["trades"].(map[string]any)
	if ok {
		return confirmutil.ObjectList(rawTrades[marketIndex])
	}
	return confirmutil.ObjectList(msg["trades"])
}
