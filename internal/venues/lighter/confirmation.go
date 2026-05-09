package lighter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"perps-latency-benchmark/internal/accountfeed"
	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/confirmws"
	"perps-latency-benchmark/internal/payload"
	"perps-latency-benchmark/internal/venues/confirmutil"
)

func ConfirmWebSocket(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	return accountfeed.NewConfirmation(ctx, built, accountfeed.PlanOptions{
		Key:      "confirmation",
		Venue:    "lighter",
		IDField:  "order_indices",
		Required: []string{"ws_url", "auth_token", "account_index", "market_index"},
	}, func(plan accountfeed.Plan) (accountfeed.ConfirmationBinding, error) {
		marketIndex := plan.Text("market_index")
		auth := plan.Text("auth_token")
		accountIndex := plan.Text("account_index")
		return accountfeed.ConfirmationBinding{
			FeedKey: lighterFeedKey(plan, accountIndex, marketIndex),
			Options: lighterFeedOptions(plan, auth, accountIndex, marketIndex),
			Match: func(msg map[string]any) (bool, error) {
				return matchLighterConfirmation(msg, marketIndex, plan.IDs, plan.Order)
			},
		}, nil
	})
}

func lighterFeedOptions(plan accountfeed.Plan, auth string, accountIndex string, marketIndex string) accountfeed.FeedOptions {
	return accountfeed.FeedOptions{
		AuthUntil: lighterAuthExpiration(auth),
		DialKey:   lighterFeedKey(plan, accountIndex, marketIndex),
		Dial: func(ctx context.Context) (*confirmws.Client, error) {
			return dialAccountFeed(ctx, plan.WSURL, auth, accountIndex, marketIndex, true)
		},
	}
}

func lighterFeedKey(plan accountfeed.Plan, accountIndex string, marketIndex string) string {
	return accountfeed.FeedKey("lighter", plan.WSURL, accountIndex, marketIndex, plan.Text("api_key_index"))
}

func ConfirmCancelWebSocket(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	return accountfeed.NewCancelConfirmation(ctx, built, accountfeed.PlanOptions{
		Key:      "cancel_confirmation",
		Venue:    "lighter",
		IDField:  "order_indices",
		Required: []string{"ws_url", "auth_token", "account_index", "market_index"},
	}, func(plan accountfeed.Plan) (accountfeed.CancelConfirmationBinding, error) {
		marketIndex := plan.Text("market_index")
		auth := plan.Text("auth_token")
		accountIndex := plan.Text("account_index")
		return accountfeed.CancelConfirmationBinding{
			FeedKey: lighterFeedKey(plan, accountIndex, marketIndex),
			Options: lighterFeedOptions(plan, auth, accountIndex, marketIndex),
			Match: func(msg map[string]any, remaining map[string]struct{}) bool {
				return matchLighterCancelConfirmation(msg, marketIndex, remaining)
			},
		}, nil
	})
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

func lighterAuthExpiration(auth string) time.Time {
	parts := strings.Split(auth, ".")
	if len(parts) < 2 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}
	}
	return claimExpiration(claims)
}

func claimExpiration(claims map[string]any) time.Time {
	for _, key := range []string{"exp", "expires_at", "expiration"} {
		if at, ok := unixClaimTime(claims[key]); ok {
			return at
		}
	}
	for _, key := range []string{"exp_ms", "expires_at_ms", "expiration_ms"} {
		if at, ok := unixMillisClaimTime(claims[key]); ok {
			return at
		}
	}
	return time.Time{}
}

func unixClaimTime(value any) (time.Time, bool) {
	seconds, ok := numericClaim(value)
	if !ok || seconds <= 0 {
		return time.Time{}, false
	}
	return time.Unix(seconds, 0).UTC(), true
}

func unixMillisClaimTime(value any) (time.Time, bool) {
	millis, ok := numericClaim(value)
	if !ok || millis <= 0 {
		return time.Time{}, false
	}
	return time.Unix(0, millis*int64(time.Millisecond)).UTC(), true
}

func numericClaim(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), true
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
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
