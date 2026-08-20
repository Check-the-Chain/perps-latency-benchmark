package bullet

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
		Venue:    "bullet",
		IDField:  "client_order_ids",
		Required: []string{"ws_url", "account"},
	}, func(plan accountfeed.Plan) (accountfeed.ConfirmationBinding, error) {
		account := plan.Text("account")
		return accountfeed.ConfirmationBinding{
			FeedKey: accountfeed.FeedKey("bullet", plan.WSURL, account),
			Options: accountfeed.FeedOptions{
				Dial: func(ctx context.Context) (*confirmws.Client, error) {
					return dialUserOrders(ctx, plan.WSURL, account)
				},
			},
			Match: func(msg map[string]any) (bool, error) {
				return matchBulletConfirmation(msg, plan.IDs, plan.Order)
			},
		}, nil
	})
}

func ConfirmCancelWebSocket(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	return accountfeed.NewCancelConfirmation(ctx, built, accountfeed.PlanOptions{
		Key:      "cancel_confirmation",
		Venue:    "bullet",
		IDField:  "client_order_ids",
		Required: []string{"ws_url", "account"},
	}, func(plan accountfeed.Plan) (accountfeed.CancelConfirmationBinding, error) {
		account := plan.Text("account")
		return accountfeed.CancelConfirmationBinding{
			FeedKey: accountfeed.FeedKey("bullet", plan.WSURL, account),
			Options: accountfeed.FeedOptions{
				Dial: func(ctx context.Context) (*confirmws.Client, error) {
					return dialUserOrders(ctx, plan.WSURL, account)
				},
			},
			Match: matchBulletCancelConfirmation,
		}, nil
	})
}

func dialUserOrders(ctx context.Context, wsURL string, account string) (*confirmws.Client, error) {
	client, err := confirmws.Dial(ctx, wsURL, http.Header{}, true)
	if err != nil {
		return nil, err
	}
	if err := client.WriteJSON(ctx, map[string]any{
		"method": "subscribe",
		"params": []string{account + "@user.orders"},
		"id":     1,
	}); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func matchBulletConfirmation(msg map[string]any, clientIDs map[string]struct{}, orderType string) (bool, error) {
	order := bulletOrder(msg)
	if len(order) == 0 {
		return false, nil
	}
	if !confirmutil.HasID(clientIDs, order["co"], order["c"]) {
		return false, nil
	}
	status := strings.ToUpper(confirmutil.Text(order["X"]))
	execution := strings.ToUpper(confirmutil.Text(order["x"]))
	if isBulletTerminalFailure(status) {
		return false, fmt.Errorf("bullet order %s", strings.ToLower(status))
	}
	filled := status == "FILLED" || status == "PARTIALLY_FILLED" || execution == "TRADE"
	resting := status == "NEW" || execution == "NEW"
	class, err := classifyBulletOrderType(orderType)
	if err != nil {
		return false, err
	}
	switch class {
	case bulletOrderTaker:
		return filled, nil
	case bulletOrderMaker:
		return resting, nil
	default:
		return resting || filled, nil
	}
}

func matchBulletCancelConfirmation(msg map[string]any, remaining map[string]struct{}) bool {
	order := bulletOrder(msg)
	if len(order) > 0 {
		id := confirmutil.FirstMatchingID(remaining, order["co"], order["c"])
		if id != "" {
			status := strings.ToUpper(confirmutil.Text(order["X"]))
			execution := strings.ToUpper(confirmutil.Text(order["x"]))
			if status == "CANCELED" || status == "CANCELLED" || status == "EXPIRED" || execution == "CANCELED" {
				delete(remaining, id)
			}
		}
	}
	return len(remaining) == 0
}

func bulletOrder(msg map[string]any) map[string]any {
	if confirmutil.Text(msg["e"]) != "orderTradeUpdate" {
		return nil
	}
	return confirmutil.Object(msg["o"])
}

func isBulletTerminalFailure(status string) bool {
	switch status {
	case "REJECTED", "EXPIRED":
		return true
	default:
		return false
	}
}

type bulletOrderClass int

const (
	// bulletOrderEither confirms on either a NEW (resting) or a fill event,
	// whichever arrives first. A plain limit order can legitimately end up
	// resting or crossing as a taker; both are genuine terminal states.
	bulletOrderEither bulletOrderClass = iota
	bulletOrderMaker
	bulletOrderTaker
)

// classifyBulletOrderType classifies orderType into a confirmation bucket.
// An empty orderType means confirmation metadata omitted it, which is a
// legitimate "unspecified" case and stays lenient (bulletOrderEither, no
// error). A non-empty orderType that matches none of the documented values
// is very likely a typo (e.g. "post_olny") and must fail loudly rather than
// silently fall back to the lenient bucket, since that bucket is the one
// setting under which no input can ever produce a confirmation timeout.
func classifyBulletOrderType(orderType string) (bulletOrderClass, error) {
	trimmed := strings.ToLower(strings.TrimSpace(orderType))
	switch trimmed {
	case "":
		return bulletOrderEither, nil
	case "limit":
		return bulletOrderEither, nil
	case "post_only", "post_only_slide", "post_only_front":
		return bulletOrderMaker, nil
	case "immediate_or_cancel", "fill_or_kill", "ioc":
		return bulletOrderTaker, nil
	default:
		return bulletOrderEither, fmt.Errorf("bullet: unrecognized order_type %q in confirmation metadata", orderType)
	}
}
