package edgex

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
	if !ok || confirmutil.Text(raw["venue"]) != "edgex" {
		return nil, nil
	}
	wsURL := confirmutil.Text(raw["ws_url"])
	if wsURL == "" {
		return nil, fmt.Errorf("edgex confirmation metadata missing ws_url")
	}
	clientIDs := confirmutil.IDSet(raw["client_order_ids"])
	if len(clientIDs) == 0 {
		return nil, fmt.Errorf("edgex confirmation metadata missing client_order_ids")
	}
	headers := http.Header{}
	if rawHeaders, ok := raw["headers"].(map[string]any); ok {
		for key, value := range rawHeaders {
			headers.Set(key, confirmutil.Text(value))
		}
	}
	if headers.Get("X-edgeX-Api-Timestamp") == "" || headers.Get("X-edgeX-Api-Signature") == "" {
		return nil, fmt.Errorf("edgex confirmation metadata missing websocket auth headers")
	}
	client, err := confirmws.Dial(ctx, wsURL, headers, false)
	if err != nil {
		return nil, err
	}
	orderType := confirmutil.Text(raw["order_type"])
	return &bench.Confirmation{
		Wait: func(ctx context.Context, submission netlatency.Result) (netlatency.Result, error) {
			return client.Wait(ctx, confirmutil.Start(submission.Trace), func(msg map[string]any) (bool, error) {
				return matchEdgeXConfirmation(msg, clientIDs, orderType)
			})
		},
		Close: client.Close,
	}, nil
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
