package hyperliquid

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"perps-latency-benchmark/internal/accountfeed"
	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/confirmws"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/payload"
	"perps-latency-benchmark/internal/venues/confirmutil"
)

func ConfirmWebSocket(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	return accountfeed.NewConfirmation(ctx, built, accountfeed.PlanOptions{
		Key:      "confirmation",
		Venue:    "hyperliquid",
		IDField:  "cloids",
		Required: []string{"ws_url", "user"},
	}, func(plan accountfeed.Plan) (accountfeed.ConfirmationBinding, error) {
		user := plan.Text("user")
		return accountfeed.ConfirmationBinding{
			FeedKey: accountfeed.FeedKey("hyperliquid", plan.WSURL, user),
			Options: accountfeed.FeedOptions{
				Dial: func(ctx context.Context) (*confirmws.Client, error) {
					return dialOrderUpdates(ctx, plan.WSURL, user)
				},
			},
			Match: func(msg map[string]any) (bool, error) {
				return matchHyperliquidConfirmation(msg, plan.IDs, plan.Order)
			},
		}, nil
	})
}

func ConfirmCancelWebSocket(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	return accountfeed.NewCancelConfirmation(ctx, built, accountfeed.PlanOptions{
		Key:      "cancel_confirmation",
		Venue:    "hyperliquid",
		IDField:  "cloids",
		Required: []string{"ws_url", "user"},
	}, func(plan accountfeed.Plan) (accountfeed.CancelConfirmationBinding, error) {
		user := plan.Text("user")
		return accountfeed.CancelConfirmationBinding{
			FeedKey: accountfeed.FeedKey("hyperliquid", plan.WSURL, user),
			Options: accountfeed.FeedOptions{
				Dial: func(ctx context.Context) (*confirmws.Client, error) {
					return dialOrderUpdates(ctx, plan.WSURL, user)
				},
			},
			Match: matchHyperliquidCancelConfirmation,
			Verify: func(ctx context.Context, submission netlatency.Result, ids map[string]struct{}) (netlatency.Result, bool, error) {
				return verifyHyperliquidCancelByOpenOrders(ctx, plan.WSURL, user, ids)
			},
		}, nil
	})
}

func verifyHyperliquidCancelByOpenOrders(ctx context.Context, wsURL string, user string, cloids map[string]struct{}) (netlatency.Result, bool, error) {
	start := time.Now()
	open, bytesRead, err := fetchHyperliquidOpenCloids(ctx, hyperliquidAPIBaseURL(wsURL), user)
	result := netlatency.Result{
		BytesRead: bytesRead,
		Trace: netlatency.Trace{
			StartedAt: start.UTC(),
			TotalNS:   time.Since(start).Nanoseconds(),
			Transport: "http_state_poll",
		},
	}
	if err != nil {
		return result, false, err
	}
	for cloid := range cloids {
		if _, ok := open[cloid]; ok {
			return result, false, nil
		}
	}
	return result, true, nil
}

func fetchHyperliquidOpenCloids(ctx context.Context, baseURL string, user string) (map[string]struct{}, int64, error) {
	body, err := json.Marshal(map[string]any{"type": "openOrders", "user": user})
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/info", bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	bytesRead := int64(len(respBody))
	if readErr != nil {
		return nil, bytesRead, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, bytesRead, fmt.Errorf("hyperliquid openOrders status %d", resp.StatusCode)
	}
	var orders []map[string]any
	if err := json.Unmarshal(respBody, &orders); err != nil {
		return nil, bytesRead, err
	}
	cloids := make(map[string]struct{})
	for _, order := range orders {
		cloid := confirmutil.Text(order["cloid"])
		if cloid != "" {
			cloids[cloid] = struct{}{}
		}
	}
	return cloids, bytesRead, nil
}

func hyperliquidAPIBaseURL(wsURL string) string {
	parsed, err := url.Parse(wsURL)
	if err != nil || parsed.Host == "" {
		return DefaultBaseURL
	}
	scheme := "https"
	if parsed.Scheme == "ws" {
		scheme = "http"
	}
	return scheme + "://" + parsed.Host
}

func dialOrderUpdates(ctx context.Context, wsURL string, user string) (*confirmws.Client, error) {
	client, err := confirmws.Dial(ctx, wsURL, http.Header{}, false)
	if err != nil {
		return nil, err
	}
	subscribeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.WriteJSON(subscribeCtx, map[string]any{
		"method":       "subscribe",
		"subscription": map[string]any{"type": "orderUpdates", "user": user},
	}); err != nil {
		_ = client.Close()
		return nil, err
	}
	if err := client.DrainUntil(subscribeCtx, func(msg map[string]any) bool {
		return confirmutil.Text(msg["channel"]) == "subscriptionResponse"
	}); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
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
		id := confirmutil.FirstMatchingID(remaining, order["cloid"])
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
	status = strings.ToLower(strings.TrimSpace(status))
	return strings.Contains(status, "rejected") || strings.Contains(status, "canceled")
}
