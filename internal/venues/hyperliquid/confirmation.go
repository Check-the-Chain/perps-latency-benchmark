package hyperliquid

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
	if !ok || raw["venue"] != "hyperliquid" {
		return nil, nil
	}
	wsURL := hlText(raw["ws_url"])
	user := hlText(raw["user"])
	if wsURL == "" || user == "" {
		return nil, fmt.Errorf("hyperliquid confirmation metadata missing ws_url or user")
	}
	cloids := hlIDSet(raw["cloids"])
	if len(cloids) == 0 {
		return nil, fmt.Errorf("hyperliquid confirmation metadata missing cloids")
	}
	client, err := confirmws.Dial(ctx, wsURL, http.Header{}, false)
	if err != nil {
		return nil, err
	}
	cleanup := func() error { return client.Close() }
	subscribeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.WriteJSON(subscribeCtx, map[string]any{
		"method":       "subscribe",
		"subscription": map[string]any{"type": "orderUpdates", "user": user},
	}); err != nil {
		_ = cleanup()
		return nil, err
	}
	if err := client.DrainUntil(subscribeCtx, func(msg map[string]any) bool {
		return hlText(msg["channel"]) == "subscriptionResponse"
	}); err != nil {
		_ = cleanup()
		return nil, err
	}
	orderType := hlText(raw["order_type"])
	return &bench.Confirmation{
		Wait: func(ctx context.Context, submission netlatency.Result) (netlatency.Result, error) {
			return client.Wait(ctx, submission.Trace.StartedAt, func(msg map[string]any) (bool, error) {
				return matchHyperliquidConfirmation(msg, cloids, orderType)
			})
		},
		Close: cleanup,
	}, nil
}

func matchHyperliquidConfirmation(msg map[string]any, cloids map[string]struct{}, orderType string) (bool, error) {
	if hlText(msg["channel"]) != "orderUpdates" {
		return false, nil
	}
	for _, update := range hlObjectList(msg["data"]) {
		order, ok := update["order"].(map[string]any)
		if !ok || !hlHasID(cloids, order["cloid"]) {
			continue
		}
		status := strings.ToLower(hlText(update["status"]))
		if orderType == "market" || orderType == "ioc" {
			if status == "filled" {
				return true, nil
			}
			if isHyperliquidTerminalFailure(status) {
				return false, fmt.Errorf("hyperliquid order %s", status)
			}
			continue
		}
		if isHyperliquidTerminalFailure(status) {
			return false, fmt.Errorf("hyperliquid order %s", status)
		}
		return true, nil
	}
	return false, nil
}

func isHyperliquidTerminalFailure(status string) bool {
	switch status {
	case "rejected", "canceled", "marginCanceled", "openInterestCapCanceled", "selfTradeCanceled", "reduceOnlyCanceled", "siblingFilledCanceled", "delistedCanceled":
		return true
	default:
		return false
	}
}

func hlObjectList(value any) []map[string]any {
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

func hlHasID(ids map[string]struct{}, values ...any) bool {
	for _, value := range values {
		if _, ok := ids[hlText(value)]; ok {
			return true
		}
	}
	return false
}

func hlIDSet(value any) map[string]struct{} {
	ids := make(map[string]struct{})
	if items, ok := value.([]any); ok {
		for _, item := range items {
			if id := hlText(item); id != "" {
				ids[id] = struct{}{}
			}
		}
	}
	return ids
}

func hlText(value any) string {
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
