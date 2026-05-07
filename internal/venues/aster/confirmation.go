package aster

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/confirmws"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/payload"
	"perps-latency-benchmark/internal/venues/confirmutil"
)

func ConfirmWebSocket(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	raw, ok := built.Metadata["confirmation"].(map[string]any)
	if !ok || confirmutil.Text(raw["venue"]) != "aster" {
		return nil, nil
	}
	wsURL := confirmutil.Text(raw["ws_url"])
	if wsURL == "" {
		return nil, fmt.Errorf("aster confirmation metadata missing ws_url")
	}
	clientIDs := confirmutil.IDSet(raw["client_order_ids"])
	if len(clientIDs) == 0 {
		return nil, fmt.Errorf("aster confirmation metadata missing client_order_ids")
	}
	client, err := confirmws.Dial(ctx, wsURL, http.Header{}, false)
	if err != nil {
		return nil, err
	}
	orderType := confirmutil.Text(raw["order_type"])
	return &bench.Confirmation{
		Wait: func(ctx context.Context, submission netlatency.Result) (netlatency.Result, error) {
			return client.Wait(ctx, confirmutil.Start(submission.Trace), func(msg map[string]any) (bool, error) {
				return matchAsterConfirmation(msg, clientIDs, orderType)
			})
		},
		Close: client.Close,
	}, nil
}

func ConfirmCancelWebSocket(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	raw, ok := built.Metadata["cancel_confirmation"].(map[string]any)
	if !ok || confirmutil.Text(raw["venue"]) != "aster" {
		return nil, nil
	}
	wsURL := confirmutil.Text(raw["ws_url"])
	if wsURL == "" {
		return nil, fmt.Errorf("aster cancel confirmation metadata missing ws_url")
	}
	clientIDs := confirmutil.IDSet(raw["client_order_ids"])
	if len(clientIDs) == 0 {
		return nil, fmt.Errorf("aster cancel confirmation metadata missing client_order_ids")
	}
	client, err := confirmws.Dial(ctx, wsURL, http.Header{}, false)
	if err != nil {
		return nil, err
	}
	return &bench.Confirmation{
		Wait: func(ctx context.Context, submission netlatency.Result) (netlatency.Result, error) {
			remaining := copyIDSet(clientIDs)
			return client.Wait(ctx, confirmutil.Start(submission.Trace), func(msg map[string]any) (bool, error) {
				return matchAsterCancelConfirmation(msg, remaining), nil
			})
		},
		Close: client.Close,
	}, nil
}

func matchAsterConfirmation(msg map[string]any, clientIDs map[string]struct{}, orderType string) (bool, error) {
	if confirmutil.Text(msg["e"]) != "ORDER_TRADE_UPDATE" {
		return false, nil
	}
	order, ok := msg["o"].(map[string]any)
	if !ok || !confirmutil.HasID(clientIDs, order["c"], order["C"]) {
		return false, nil
	}
	status := strings.ToUpper(confirmutil.Text(order["X"]))
	execution := strings.ToUpper(confirmutil.Text(order["x"]))
	if orderType == "market" || orderType == "ioc" || orderType == "fok" {
		if status == "FILLED" || execution == "TRADE" {
			return true, nil
		}
		if isAsterTerminalFailure(status) {
			return false, fmt.Errorf("aster order %s", status)
		}
		return false, nil
	}
	if isAsterTerminalFailure(status) {
		return false, fmt.Errorf("aster order %s", status)
	}
	return true, nil
}

func matchAsterCancelConfirmation(msg map[string]any, remaining map[string]struct{}) bool {
	if confirmutil.Text(msg["e"]) != "ORDER_TRADE_UPDATE" {
		return false
	}
	order, ok := msg["o"].(map[string]any)
	if !ok {
		return false
	}
	id := firstMatchingID(remaining, order["c"], order["C"])
	if id == "" {
		return false
	}
	status := strings.ToUpper(confirmutil.Text(order["X"]))
	execution := strings.ToUpper(confirmutil.Text(order["x"]))
	if status == "CANCELED" || status == "CANCELLED" || execution == "CANCELED" || execution == "CANCELLED" {
		delete(remaining, id)
	}
	return len(remaining) == 0
}

func isAsterTerminalFailure(status string) bool {
	switch status {
	case "CANCELED", "CANCELLED", "REJECTED", "EXPIRED":
		return true
	default:
		return strings.Contains(status, "REJECT") || strings.Contains(status, "EXPIRE")
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
