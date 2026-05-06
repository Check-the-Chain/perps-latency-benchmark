package extended

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
	if !ok || confirmutil.Text(raw["venue"]) != "extended" {
		return nil, nil
	}
	wsURL := confirmutil.Text(raw["ws_url"])
	apiKey := confirmutil.Text(raw["api_key"])
	if wsURL == "" || apiKey == "" {
		return nil, fmt.Errorf("extended confirmation metadata missing ws_url or api_key")
	}
	externalIDs := confirmutil.IDSet(raw["external_ids"])
	if len(externalIDs) == 0 {
		return nil, fmt.Errorf("extended confirmation metadata missing external_ids")
	}
	headers := http.Header{}
	headers.Set("X-Api-Key", apiKey)
	headers.Set("User-Agent", "perps-latency-benchmark")
	client, err := confirmws.Dial(ctx, wsURL, headers, false)
	if err != nil {
		return nil, err
	}
	orderType := confirmutil.Text(raw["order_type"])
	return &bench.Confirmation{
		Wait: func(ctx context.Context, submission netlatency.Result) (netlatency.Result, error) {
			return client.Wait(ctx, confirmutil.Start(submission.Trace), func(msg map[string]any) (bool, error) {
				return matchExtendedConfirmation(msg, externalIDs, orderType)
			})
		},
		Close: client.Close,
	}, nil
}

func matchExtendedConfirmation(msg map[string]any, externalIDs map[string]struct{}, orderType string) (bool, error) {
	data := confirmutil.Object(msg["data"])
	for _, trade := range confirmutil.ObjectList(data["trades"]) {
		if confirmutil.HasID(externalIDs, trade["externalOrderId"], trade["external_order_id"], trade["externalId"], trade["external_id"]) {
			return true, nil
		}
	}
	for _, order := range confirmutil.ObjectList(data["orders"]) {
		if !confirmutil.HasID(externalIDs, order["externalId"], order["external_id"]) {
			continue
		}
		status := strings.ToLower(confirmutil.Text(order["status"]))
		if orderType == "market" || orderType == "ioc" || orderType == "fok" {
			if status == "filled" || status == "partially_filled" || status == "partially-filled" {
				return true, nil
			}
			if isExtendedTerminalFailure(status) {
				return false, fmt.Errorf("extended order %s", status)
			}
			continue
		}
		if isExtendedTerminalFailure(status) {
			return false, fmt.Errorf("extended order %s", status)
		}
		return true, nil
	}
	return false, nil
}

func isExtendedTerminalFailure(status string) bool {
	switch status {
	case "rejected", "cancelled", "canceled", "expired", "failed":
		return true
	default:
		return false
	}
}
