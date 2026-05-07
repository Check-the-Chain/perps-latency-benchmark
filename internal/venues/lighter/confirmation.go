package lighter

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/confirmws"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/payload"
	"perps-latency-benchmark/internal/venues/confirmutil"
)

func ConfirmWebSocket(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	raw, ok := built.Metadata["confirmation"].(map[string]any)
	if !ok || raw["venue"] != "lighter" {
		return nil, nil
	}
	wsURL := confirmutil.Text(raw["ws_url"])
	auth := confirmutil.Text(raw["auth_token"])
	accountIndex := confirmutil.Text(raw["account_index"])
	marketIndex := confirmutil.Text(raw["market_index"])
	if wsURL == "" || auth == "" || accountIndex == "" || marketIndex == "" {
		return nil, fmt.Errorf("lighter confirmation metadata missing ws_url, auth_token, account_index, or market_index")
	}
	orderIDs := confirmutil.IDSet(raw["order_indices"])
	if len(orderIDs) == 0 {
		return nil, fmt.Errorf("lighter confirmation metadata missing order_indices")
	}
	client, err := confirmws.Dial(ctx, wsURL, http.Header{}, true)
	if err != nil {
		return nil, err
	}
	cleanup := func() error { return client.Close() }
	subscribeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.WriteJSON(subscribeCtx, map[string]any{
		"type":    "subscribe",
		"channel": fmt.Sprintf("account_orders/%s/%s", marketIndex, accountIndex),
		"auth":    auth,
	}); err != nil {
		_ = cleanup()
		return nil, err
	}
	if err := client.DrainUntil(subscribeCtx, func(msg map[string]any) bool {
		return strings.HasPrefix(confirmutil.Text(msg["channel"]), "account_orders:")
	}); err != nil {
		_ = cleanup()
		return nil, err
	}
	if err := client.WriteJSON(subscribeCtx, map[string]any{
		"type":    "subscribe",
		"channel": fmt.Sprintf("account_all_trades/%s", accountIndex),
		"auth":    auth,
	}); err != nil {
		_ = cleanup()
		return nil, err
	}
	if err := client.DrainUntil(subscribeCtx, func(msg map[string]any) bool {
		return strings.HasPrefix(confirmutil.Text(msg["channel"]), "account_all_trades:")
	}); err != nil {
		_ = cleanup()
		return nil, err
	}
	orderType := confirmutil.Text(raw["order_type"])
	return &bench.Confirmation{
		Wait: func(ctx context.Context, submission netlatency.Result) (netlatency.Result, error) {
			return client.Wait(ctx, confirmutil.Start(submission.Trace), func(msg map[string]any) (bool, error) {
				return matchLighterConfirmation(msg, marketIndex, orderIDs, orderType)
			})
		},
		Close: cleanup,
	}, nil
}

func ConfirmCancelWebSocket(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	raw, ok := built.Metadata["cancel_confirmation"].(map[string]any)
	if !ok || raw["venue"] != "lighter" {
		return nil, nil
	}
	wsURL := confirmutil.Text(raw["ws_url"])
	auth := confirmutil.Text(raw["auth_token"])
	accountIndex := confirmutil.Text(raw["account_index"])
	marketIndex := confirmutil.Text(raw["market_index"])
	if wsURL == "" || auth == "" || accountIndex == "" || marketIndex == "" {
		return nil, fmt.Errorf("lighter cancel confirmation metadata missing ws_url, auth_token, account_index, or market_index")
	}
	orderIDs := confirmutil.IDSet(raw["order_indices"])
	if len(orderIDs) == 0 {
		return nil, fmt.Errorf("lighter cancel confirmation metadata missing order_indices")
	}
	client, err := confirmws.Dial(ctx, wsURL, http.Header{}, true)
	if err != nil {
		return nil, err
	}
	cleanup := func() error { return client.Close() }
	subscribeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.WriteJSON(subscribeCtx, map[string]any{
		"type":    "subscribe",
		"channel": fmt.Sprintf("account_orders/%s/%s", marketIndex, accountIndex),
		"auth":    auth,
	}); err != nil {
		_ = cleanup()
		return nil, err
	}
	if err := client.DrainUntil(subscribeCtx, func(msg map[string]any) bool {
		return strings.HasPrefix(confirmutil.Text(msg["channel"]), "account_orders:")
	}); err != nil {
		_ = cleanup()
		return nil, err
	}
	return &bench.Confirmation{
		Wait: func(ctx context.Context, submission netlatency.Result) (netlatency.Result, error) {
			remaining := copyIDSet(orderIDs)
			return client.Wait(ctx, confirmutil.Start(submission.Trace), func(msg map[string]any) (bool, error) {
				return matchLighterCancelConfirmation(msg, marketIndex, remaining), nil
			})
		},
		Close: cleanup,
	}, nil
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
		id := firstMatchingID(remaining, order["client_order_index"], order["client_order_id"], order["order_index"], order["order_id"])
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

func copyIDSet(ids map[string]struct{}) map[string]struct{} {
	copied := make(map[string]struct{}, len(ids))
	for id := range ids {
		copied[id] = struct{}{}
	}
	return copied
}

func firstMatchingID(ids map[string]struct{}, values ...any) string {
	for _, value := range values {
		id := confirmutil.Text(value)
		if _, ok := ids[id]; ok {
			return id
		}
	}
	return ""
}

func marketTrades(msg map[string]any, marketIndex string) []map[string]any {
	rawTrades, ok := msg["trades"].(map[string]any)
	if ok {
		return confirmutil.ObjectList(rawTrades[marketIndex])
	}
	return confirmutil.ObjectList(msg["trades"])
}
