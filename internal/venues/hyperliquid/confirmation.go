package hyperliquid

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
	if !ok || raw["venue"] != "hyperliquid" {
		return nil, nil
	}
	wsURL := confirmutil.Text(raw["ws_url"])
	user := confirmutil.Text(raw["user"])
	if wsURL == "" || user == "" {
		return nil, fmt.Errorf("hyperliquid confirmation metadata missing ws_url or user")
	}
	cloids := confirmutil.IDSet(raw["cloids"])
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
		return confirmutil.Text(msg["channel"]) == "subscriptionResponse"
	}); err != nil {
		_ = cleanup()
		return nil, err
	}
	orderType := confirmutil.Text(raw["order_type"])
	return &bench.Confirmation{
		Wait: func(ctx context.Context, submission netlatency.Result) (netlatency.Result, error) {
			return client.Wait(ctx, confirmutil.Start(submission.Trace), func(msg map[string]any) (bool, error) {
				return matchHyperliquidConfirmation(msg, cloids, orderType)
			})
		},
		Close: cleanup,
	}, nil
}

func ConfirmCancelWebSocket(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	raw, ok := built.Metadata["cancel_confirmation"].(map[string]any)
	if !ok || raw["venue"] != "hyperliquid" {
		return nil, nil
	}
	wsURL := confirmutil.Text(raw["ws_url"])
	user := confirmutil.Text(raw["user"])
	if wsURL == "" || user == "" {
		return nil, fmt.Errorf("hyperliquid cancel confirmation metadata missing ws_url or user")
	}
	cloids := confirmutil.IDSet(raw["cloids"])
	if len(cloids) == 0 {
		return nil, fmt.Errorf("hyperliquid cancel confirmation metadata missing cloids")
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
		return confirmutil.Text(msg["channel"]) == "subscriptionResponse"
	}); err != nil {
		_ = cleanup()
		return nil, err
	}
	return &bench.Confirmation{
		Wait: func(ctx context.Context, submission netlatency.Result) (netlatency.Result, error) {
			remaining := copyIDSet(cloids)
			return client.Wait(ctx, confirmutil.Start(submission.Trace), func(msg map[string]any) (bool, error) {
				return matchHyperliquidCancelConfirmation(msg, remaining), nil
			})
		},
		Close: cleanup,
	}, nil
}

func matchHyperliquidConfirmation(msg map[string]any, cloids map[string]struct{}, orderType string) (bool, error) {
	if confirmutil.Text(msg["channel"]) != "orderUpdates" {
		return false, nil
	}
	for _, update := range confirmutil.ObjectList(msg["data"]) {
		order, ok := update["order"].(map[string]any)
		if !ok || !confirmutil.HasID(cloids, order["cloid"]) {
			continue
		}
		status := strings.ToLower(confirmutil.Text(update["status"]))
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

func matchHyperliquidCancelConfirmation(msg map[string]any, remaining map[string]struct{}) bool {
	if confirmutil.Text(msg["channel"]) != "orderUpdates" {
		return false
	}
	for _, update := range confirmutil.ObjectList(msg["data"]) {
		order, ok := update["order"].(map[string]any)
		if !ok {
			continue
		}
		id := firstMatchingID(remaining, order["cloid"])
		if id == "" {
			continue
		}
		status := strings.ToLower(confirmutil.Text(update["status"]))
		if strings.Contains(status, "cancel") {
			delete(remaining, id)
		}
	}
	return len(remaining) == 0
}

func isHyperliquidTerminalFailure(status string) bool {
	switch status {
	case "rejected", "canceled", "marginCanceled", "openInterestCapCanceled", "selfTradeCanceled", "reduceOnlyCanceled", "siblingFilledCanceled", "delistedCanceled":
		return true
	default:
		return false
	}
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
