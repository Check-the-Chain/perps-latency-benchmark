package nado

import (
	"context"
	"fmt"
	"strings"
	"time"

	"perps-latency-benchmark/internal/accountfeed"
	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/confirmws"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/payload"
	"perps-latency-benchmark/internal/venues/confirmutil"
)

type nadoSubscriptionPlan struct {
	wsURL      string
	subaccount string
	productID  any
	auth       map[string]any
}

func ConfirmWebSocket(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	plan, ok, err := accountfeed.DecodePlan(built, accountfeed.PlanOptions{
		Key:      "confirmation",
		Venue:    "nado",
		IDField:  "digests",
		Required: []string{"ws_url", "subaccount"},
	})
	if !ok || err != nil {
		return nil, err
	}
	subscription, err := nadoSubscriptionFromPlan(plan, "confirmation")
	if err != nil {
		return nil, err
	}
	feed := sharedNadoFeed(subscription)
	if err := feed.ensure(ctx, subscription.auth); err != nil {
		return nil, err
	}
	orderType := strings.ToLower(plan.Order)
	return &bench.Confirmation{
		Wait: func(ctx context.Context, submission netlatency.Result) (netlatency.Result, error) {
			if err := feed.ensure(ctx, subscription.auth); err != nil {
				return netlatency.Result{}, err
			}
			return feed.wait(ctx, confirmutil.Start(submission.Trace), func(msg map[string]any) (bool, error) {
				return matchNadoConfirmation(msg, plan.IDs, orderType)
			})
		},
	}, nil
}

func ConfirmCancelWebSocket(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	plan, ok, err := accountfeed.DecodePlan(built, accountfeed.PlanOptions{
		Key:      "cancel_confirmation",
		Venue:    "nado",
		IDField:  "digests",
		Required: []string{"ws_url", "subaccount"},
	})
	if !ok || err != nil {
		return nil, err
	}
	subscription, err := nadoSubscriptionFromPlan(plan, "cancel confirmation")
	if err != nil {
		return nil, err
	}
	feed := sharedNadoFeed(subscription)
	if err := feed.ensure(ctx, subscription.auth); err != nil {
		return nil, err
	}
	remaining := confirmutil.CopyIDSet(plan.IDs)
	return &bench.Confirmation{
		Wait: func(ctx context.Context, submission netlatency.Result) (netlatency.Result, error) {
			if err := feed.ensure(ctx, subscription.auth); err != nil {
				return netlatency.Result{}, err
			}
			return feed.wait(ctx, confirmutil.Start(submission.Trace), func(msg map[string]any) (bool, error) {
				return matchNadoCancelConfirmation(msg, remaining), nil
			})
		},
	}, nil
}

func nadoSubscriptionFromPlan(plan accountfeed.Plan, label string) (nadoSubscriptionPlan, error) {
	auth, ok := plan.Raw["auth"].(map[string]any)
	if !ok || len(auth) == 0 {
		return nadoSubscriptionPlan{}, fmt.Errorf("nado %s metadata missing auth", label)
	}
	return nadoSubscriptionPlan{
		wsURL:      plan.WSURL,
		subaccount: plan.Text("subaccount"),
		productID:  plan.Raw["product_id"],
		auth:       auth,
	}, nil
}

func dialOrderUpdates(ctx context.Context, wsURL string, auth map[string]any, subaccount string, productID any) (*confirmws.Client, error) {
	client, err := confirmws.DialWithOptions(ctx, wsURL, nil, false, confirmws.DialOptions{CompressionContextTakeover: true})
	if err != nil {
		return nil, err
	}
	client.StartPingFrames(25*time.Second, 5*time.Second)
	setupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.WriteJSON(setupCtx, auth); err != nil {
		_ = client.Close()
		return nil, err
	}
	authID := confirmutil.Text(auth["id"])
	if err := client.DrainUntil(setupCtx, func(msg map[string]any) bool {
		return confirmsRequestID(msg, authID)
	}); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("nado subscription authenticate: %w", err)
	}
	subscribeID := int(time.Now().UnixNano() & 0x7fffffff)
	if err := client.WriteJSON(setupCtx, map[string]any{
		"method": "subscribe",
		"stream": map[string]any{
			"type":       "order_update",
			"subaccount": subaccount,
			"product_id": productID,
		},
		"id": subscribeID,
	}); err != nil {
		_ = client.Close()
		return nil, err
	}
	if err := client.DrainUntil(setupCtx, func(msg map[string]any) bool {
		return confirmsRequestID(msg, confirmutil.Text(subscribeID))
	}); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("nado subscription order_update: %w", err)
	}
	return client, nil
}

func confirmsRequestID(msg map[string]any, id string) bool {
	if id == "" {
		return false
	}
	if confirmutil.Text(msg["id"]) != id {
		return false
	}
	if _, hasResult := msg["result"]; hasResult {
		return true
	}
	if status := strings.ToLower(confirmutil.Text(msg["status"])); status == "success" || status == "ok" {
		return true
	}
	return false
}

func matchNadoConfirmation(msg map[string]any, digests map[string]struct{}, orderType string) (bool, error) {
	for _, event := range nadoEvents(msg) {
		if strings.ToLower(confirmutil.Text(event["type"])) != "order_update" || !confirmutil.HasID(digests, event["digest"]) {
			continue
		}
		reason := strings.ToLower(confirmutil.Text(event["reason"]))
		if orderType == "ioc" || orderType == "fok" || orderType == "market" {
			if reason == "filled" || reason == "placed" {
				return true, nil
			}
			if reason == "cancelled" {
				return false, fmt.Errorf("nado order cancelled")
			}
			continue
		}
		if reason == "cancelled" {
			return false, fmt.Errorf("nado order cancelled")
		}
		if reason == "placed" || reason == "filled" {
			return true, nil
		}
	}
	return false, nil
}

func matchNadoCancelConfirmation(msg map[string]any, remaining map[string]struct{}) bool {
	for _, event := range nadoEvents(msg) {
		if strings.ToLower(confirmutil.Text(event["type"])) != "order_update" {
			continue
		}
		digest := confirmutil.FirstMatchingID(remaining, event["digest"])
		if digest == "" {
			continue
		}
		if strings.ToLower(confirmutil.Text(event["reason"])) == "cancelled" {
			delete(remaining, digest)
		}
	}
	return len(remaining) == 0
}

func nadoEvents(msg map[string]any) []map[string]any {
	if strings.ToLower(confirmutil.Text(msg["type"])) == "order_update" {
		return []map[string]any{msg}
	}
	for _, key := range []string{"event", "data", "result"} {
		if child, ok := msg[key].(map[string]any); ok {
			if events := nadoEvents(child); len(events) > 0 {
				return events
			}
		}
		if list, ok := msg[key].([]any); ok {
			out := make([]map[string]any, 0, len(list))
			for _, item := range list {
				if object, ok := item.(map[string]any); ok {
					out = append(out, nadoEvents(object)...)
				}
			}
			if len(out) > 0 {
				return out
			}
		}
	}
	return nil
}
