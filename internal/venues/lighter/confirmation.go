package lighter

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/confirmws"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/payload"
)

func ConfirmWebSocket(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	raw, ok := built.Metadata["confirmation"].(map[string]any)
	if !ok || raw["venue"] != "lighter" {
		return nil, nil
	}
	wsURL := text(raw["ws_url"])
	auth := text(raw["auth_token"])
	accountIndex := text(raw["account_index"])
	marketIndex := text(raw["market_index"])
	if wsURL == "" || auth == "" || accountIndex == "" || marketIndex == "" {
		return nil, fmt.Errorf("lighter confirmation metadata missing ws_url, auth_token, account_index, or market_index")
	}
	orderIDs := idSet(raw["order_indices"])
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
		return strings.HasPrefix(text(msg["channel"]), "account_orders:")
	}); err != nil {
		_ = cleanup()
		return nil, err
	}
	_ = client.WriteJSON(ctx, map[string]any{
		"type":    "subscribe",
		"channel": fmt.Sprintf("account_all_trades/%s", accountIndex),
		"auth":    auth,
	})
	orderType := text(raw["order_type"])
	return &bench.Confirmation{
		Wait: func(ctx context.Context, submission netlatency.Result) (netlatency.Result, error) {
			return client.Wait(ctx, submission.Trace.StartedAt, func(msg map[string]any) (bool, error) {
				return matchLighterConfirmation(msg, marketIndex, orderIDs, orderType)
			})
		},
		Close: cleanup,
	}, nil
}

func matchLighterConfirmation(msg map[string]any, marketIndex string, orderIDs map[string]struct{}, orderType string) (bool, error) {
	for _, trade := range marketTrades(msg, marketIndex) {
		if hasID(orderIDs, trade["ask_client_id"], trade["ask_client_id_str"], trade["bid_client_id"], trade["bid_client_id_str"]) {
			return true, nil
		}
	}
	for _, order := range marketOrders(msg, marketIndex) {
		if !hasID(orderIDs, order["client_order_index"], order["client_order_id"], order["order_index"], order["order_id"]) {
			continue
		}
		status := strings.ToLower(text(order["status"]))
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

func marketOrders(msg map[string]any, marketIndex string) []map[string]any {
	rawOrders, ok := msg["orders"].(map[string]any)
	if !ok {
		return nil
	}
	return objectList(rawOrders[marketIndex])
}

func marketTrades(msg map[string]any, marketIndex string) []map[string]any {
	rawTrades, ok := msg["trades"].(map[string]any)
	if ok {
		return objectList(rawTrades[marketIndex])
	}
	return objectList(msg["trades"])
}

func objectList(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if obj, ok := item.(map[string]any); ok {
			out = append(out, obj)
		}
	}
	return out
}

func hasID(ids map[string]struct{}, values ...any) bool {
	for _, value := range values {
		if _, ok := ids[text(value)]; ok {
			return true
		}
	}
	return false
}

func idSet(value any) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, item := range anySlice(value) {
		id := text(item)
		if id != "" {
			ids[id] = struct{}{}
		}
	}
	return ids
}

func anySlice(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []int:
		out := make([]any, len(typed))
		for i, value := range typed {
			out[i] = value
		}
		return out
	default:
		return nil
	}
}

func text(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		if value == nil {
			return ""
		}
		return fmt.Sprint(value)
	}
}
