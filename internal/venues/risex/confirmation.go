package risex

import (
	"context"
	"fmt"
	"strings"
	"time"

	"perps-latency-benchmark/internal/accountfeed"
	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/confirmws"
	"perps-latency-benchmark/internal/payload"
	"perps-latency-benchmark/internal/venues/confirmutil"
)

// The private "orders" WebSocket channel requires an auth_v2 handshake
// (EIP-712 RegisterV2 signature over a server-issued nonce). All signing
// happens in build_payload.py, which embeds the ready-to-send auth_v2 frame
// in metadata.confirmation.auth_v2; this file only dials, sends the
// pre-signed frame, and matches subsequent order events. The exact auth_v2
// wire envelope and the presence of client_order_id on channel events are
// not independently confirmed against a live testnet WebSocket connection
// (see README.md) — verify before trusting confirmation timing.

type risexSubscriptionPlan struct {
	wsURL    string
	account  string
	marketID any
	authV2   map[string]any
}

func ConfirmWebSocket(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	return accountfeed.NewConfirmation(ctx, built, accountfeed.PlanOptions{
		Key:      "confirmation",
		Venue:    "risex",
		IDField:  "client_order_ids",
		Required: []string{"ws_url", "account"},
	}, func(plan accountfeed.Plan) (accountfeed.ConfirmationBinding, error) {
		subscription, err := risexSubscriptionFromPlan(plan, "confirmation")
		if err != nil {
			return accountfeed.ConfirmationBinding{}, err
		}
		orderType := strings.ToLower(plan.Order)
		return accountfeed.ConfirmationBinding{
			FeedKey: accountfeed.FeedKey("risex", subscription.wsURL, subscription.account),
			Options: risexFeedOptions(subscription),
			Match: func(msg map[string]any) (bool, error) {
				return matchRisexConfirmation(msg, plan.IDs, orderType)
			},
		}, nil
	})
}

func ConfirmCancelWebSocket(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	return accountfeed.NewCancelConfirmation(ctx, built, accountfeed.PlanOptions{
		Key:      "cancel_confirmation",
		Venue:    "risex",
		IDField:  "client_order_ids",
		Required: []string{"ws_url", "account"},
	}, func(plan accountfeed.Plan) (accountfeed.CancelConfirmationBinding, error) {
		subscription, err := risexSubscriptionFromPlan(plan, "cancel confirmation")
		if err != nil {
			return accountfeed.CancelConfirmationBinding{}, err
		}
		return accountfeed.CancelConfirmationBinding{
			FeedKey: accountfeed.FeedKey("risex", subscription.wsURL, subscription.account),
			Options: risexFeedOptions(subscription),
			Match:   matchRisexCancelConfirmation,
		}, nil
	})
}

func risexSubscriptionFromPlan(plan accountfeed.Plan, label string) (risexSubscriptionPlan, error) {
	authV2, _ := plan.Raw["auth_v2"].(map[string]any)
	if len(authV2) == 0 {
		return risexSubscriptionPlan{}, fmt.Errorf("risex %s metadata missing auth_v2", label)
	}
	return risexSubscriptionPlan{
		wsURL:    plan.WSURL,
		account:  plan.Text("account"),
		marketID: plan.Raw["market_id"],
		authV2:   authV2,
	}, nil
}

func risexFeedOptions(plan risexSubscriptionPlan) accountfeed.FeedOptions {
	return accountfeed.FeedOptions{
		AuthUntil: risexAuthExpiration(plan.authV2),
		Dial: func(ctx context.Context) (*confirmws.Client, error) {
			return dialRisexOrders(ctx, plan.wsURL, plan.authV2, plan.account, plan.marketID)
		},
	}
}

func risexAuthExpiration(authV2 map[string]any) time.Time {
	raw := strings.TrimSpace(confirmutil.Text(authV2["expires_at"]))
	if raw == "" {
		return time.Time{}
	}
	seconds, err := parseUnixSeconds(raw)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(seconds, 0).UTC()
}

func parseUnixSeconds(raw string) (int64, error) {
	var seconds int64
	_, err := fmt.Sscanf(raw, "%d", &seconds)
	return seconds, err
}

func dialRisexOrders(ctx context.Context, wsURL string, authV2 map[string]any, account string, marketID any) (*confirmws.Client, error) {
	client, err := confirmws.Dial(ctx, wsURL, nil, false)
	if err != nil {
		return nil, err
	}
	client.StartPingFrames(15*time.Second, 5*time.Second)
	setupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.WriteJSON(setupCtx, map[string]any{
		"method": "auth_v2",
		"params": risexAuthParams(authV2),
	}); err != nil {
		_ = client.Close()
		return nil, err
	}
	if err := client.DrainUntil(setupCtx, isRisexAuthAck); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("risex ws auth_v2: %w", err)
	}
	if err := client.WriteJSON(setupCtx, map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel":    "orders",
			"market_ids": []any{marketID},
			"makers":     []any{account},
		},
	}); err != nil {
		_ = client.Close()
		return nil, err
	}
	if err := client.DrainUntil(setupCtx, func(msg map[string]any) bool {
		return isRisexChannelAck(msg, "orders")
	}); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("risex ws subscribe orders: %w", err)
	}
	return client, nil
}

func risexAuthParams(authV2 map[string]any) map[string]any {
	params := make(map[string]any, len(authV2))
	for key, value := range authV2 {
		if key == "expires_at" {
			continue
		}
		params[key] = value
	}
	return params
}

func isRisexAuthAck(msg map[string]any) bool {
	if strings.EqualFold(confirmutil.Text(msg["type"]), "error") {
		return false
	}
	method := strings.ToLower(confirmutil.Text(msg["method"]))
	return method == "auth_v2" || strings.Contains(strings.ToLower(confirmutil.Text(msg["type"])), "auth")
}

func isRisexChannelAck(msg map[string]any, channel string) bool {
	if !strings.EqualFold(confirmutil.Text(msg["channel"]), channel) {
		return false
	}
	msgType := strings.ToLower(confirmutil.Text(msg["type"]))
	return msgType == "subscribed" || msgType == "snapshot"
}

func matchRisexConfirmation(msg map[string]any, clientOrderIDs map[string]struct{}, orderType string) (bool, error) {
	for _, event := range risexOrderEvents(msg) {
		if !confirmutil.HasID(clientOrderIDs, event["client_order_id"]) {
			continue
		}
		status := strings.ToUpper(confirmutil.Text(event["status"]))
		if orderType == "ioc" || orderType == "fok" || orderType == "market" {
			if strings.Contains(status, "FILLED") || strings.Contains(status, "OPEN") {
				return true, nil
			}
			if strings.Contains(status, "CANCEL") || strings.Contains(status, "REJECT") {
				return false, fmt.Errorf("risex order %s", status)
			}
			continue
		}
		if strings.Contains(status, "CANCEL") || strings.Contains(status, "REJECT") {
			return false, fmt.Errorf("risex order %s", status)
		}
		if strings.Contains(status, "OPEN") || strings.Contains(status, "FILLED") {
			return true, nil
		}
	}
	return false, nil
}

func matchRisexCancelConfirmation(msg map[string]any, remaining map[string]struct{}) bool {
	for _, event := range risexOrderEvents(msg) {
		id := confirmutil.FirstMatchingID(remaining, event["client_order_id"])
		if id == "" {
			continue
		}
		status := strings.ToUpper(confirmutil.Text(event["status"]))
		if strings.Contains(status, "CANCEL") {
			delete(remaining, id)
		}
	}
	return len(remaining) == 0
}

func risexOrderEvents(msg map[string]any) []map[string]any {
	if !strings.EqualFold(confirmutil.Text(msg["channel"]), "orders") {
		return nil
	}
	switch data := msg["data"].(type) {
	case map[string]any:
		return []map[string]any{data}
	case []any:
		out := make([]map[string]any, 0, len(data))
		for _, item := range data {
			if object, ok := item.(map[string]any); ok {
				out = append(out, object)
			}
		}
		return out
	}
	return nil
}
