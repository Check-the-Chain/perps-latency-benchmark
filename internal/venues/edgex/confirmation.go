package edgex

import (
	"context"
	"fmt"
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
		Venue:    "edgex",
		IDField:  "client_order_ids",
		Required: []string{"ws_url"},
	}, func(plan accountfeed.Plan) (accountfeed.ConfirmationBinding, error) {
		headers := plan.Headers("headers")
		if headers.Get("X-edgeX-Api-Timestamp") == "" || headers.Get("X-edgeX-Api-Signature") == "" {
			return accountfeed.ConfirmationBinding{}, fmt.Errorf("edgex confirmation metadata missing websocket auth headers")
		}
		return accountfeed.ConfirmationBinding{
			FeedKey: accountfeed.FeedKey("edgex", plan.WSURL),
			Options: accountfeed.FeedOptions{
				Dial: func(ctx context.Context) (*confirmws.Client, error) {
					return confirmws.Dial(ctx, plan.WSURL, headers, false)
				},
			},
			Match: func(msg map[string]any) (bool, error) {
				return matchEdgeXConfirmation(msg, plan.IDs, plan.Order)
			},
		}, nil
	})
}

func matchEdgeXConfirmation(msg map[string]any, clientIDs map[string]struct{}, orderType string) (bool, error) {
	order := findEdgeXOrder(msg, clientIDs)
	if order == nil {
		return false, nil
	}
	status := strings.ToUpper(confirmutil.Text(firstEdgeValue(order, "status", "orderStatus")))
	if status == "" {
		return true, nil
	}
	if orderType == "market" || orderType == "immediate_or_cancel" || orderType == "fill_or_kill" {
		if strings.Contains(status, "FILLED") || strings.Contains(status, "MATCHED") || strings.Contains(status, "APPROVED") {
			return true, nil
		}
		if isEdgeXTerminalFailure(status) {
			return false, fmt.Errorf("edgex order %s", status)
		}
		return false, nil
	}
	if isEdgeXTerminalFailure(status) {
		return false, fmt.Errorf("edgex order %s", status)
	}
	return true, nil
}

func findEdgeXOrder(value any, clientIDs map[string]struct{}) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		if confirmutil.HasID(clientIDs, typed["clientOrderId"], typed["client_order_id"], typed["clientId"], typed["client_id"]) {
			return typed
		}
		for _, child := range typed {
			if found := findEdgeXOrder(child, clientIDs); found != nil {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := findEdgeXOrder(child, clientIDs); found != nil {
				return found
			}
		}
	}
	return nil
}

func isEdgeXTerminalFailure(status string) bool {
	switch status {
	case "FAILED", "REJECTED", "CANCELED", "CANCELLED", "EXPIRED", "FAILED_ORDER_NOT_FOUND", "FAILED_ORDER_FILLED", "FAILED_ORDER_UNKNOWN_STATUS":
		return true
	default:
		return strings.Contains(status, "REJECT") || strings.Contains(status, "FAIL")
	}
}

func firstEdgeValue(value map[string]any, keys ...string) any {
	for _, key := range keys {
		if raw, ok := value[key]; ok {
			return raw
		}
	}
	return nil
}
