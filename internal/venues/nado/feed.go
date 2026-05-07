package nado

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"perps-latency-benchmark/internal/confirmws"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/venues/confirmutil"
)

const (
	nadoFeedRecentLimit = 1024
	nadoFeedRecentAge   = 10 * time.Second
	nadoFeedAuthSkew    = 10 * time.Second
)

var nadoFeeds sync.Map

type nadoFeed struct {
	key        string
	wsURL      string
	subaccount string
	productID  any

	connectMu sync.Mutex
	mu        sync.Mutex
	client    *confirmws.Client
	authUntil time.Time
	waiters   map[int]*nadoFeedWaiter
	recent    []nadoFeedMessage
	nextID    int
}

type nadoFeedWaiter struct {
	start time.Time
	match func(map[string]any) (bool, error)
	ch    chan nadoFeedResult
}

type nadoFeedMessage struct {
	value      map[string]any
	body       []byte
	receivedAt time.Time
}

type nadoFeedResult struct {
	result netlatency.Result
	err    error
}

func sharedNadoFeed(plan nadoSubscriptionPlan) *nadoFeed {
	key := strings.Join([]string{
		plan.wsURL,
		plan.subaccount,
		confirmutil.Text(plan.productID),
	}, "\x00")
	value, _ := nadoFeeds.LoadOrStore(key, &nadoFeed{
		key:        key,
		wsURL:      plan.wsURL,
		subaccount: plan.subaccount,
		productID:  plan.productID,
	})
	return value.(*nadoFeed)
}

func (f *nadoFeed) ensure(ctx context.Context, auth map[string]any) error {
	f.connectMu.Lock()
	defer f.connectMu.Unlock()

	if f.ready() {
		return nil
	}

	client, authUntil, err := dialSubscribedOrderUpdates(ctx, f.wsURL, auth, f.subaccount, f.productID)
	if err != nil {
		return err
	}

	f.mu.Lock()
	old := f.client
	f.client = client
	f.authUntil = authUntil
	f.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	go f.readLoop(client)
	return nil
}

func (f *nadoFeed) ready() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.client != nil && (f.authUntil.IsZero() || time.Until(f.authUntil) > nadoFeedAuthSkew)
}

func (f *nadoFeed) wait(ctx context.Context, start time.Time, match func(map[string]any) (bool, error)) (netlatency.Result, error) {
	if start.IsZero() {
		start = time.Now().UTC()
	}
	waiter := &nadoFeedWaiter{
		start: start,
		match: match,
		ch:    make(chan nadoFeedResult, 1),
	}

	f.mu.Lock()
	f.trimRecentLocked(time.Now())
	for _, msg := range f.recent {
		if msg.receivedAt.Before(start) {
			continue
		}
		ok, err := match(msg.value)
		if err != nil {
			f.mu.Unlock()
			return nadoFeedNetResult(start, msg.receivedAt, msg.body), err
		}
		if ok {
			f.mu.Unlock()
			return nadoFeedNetResult(start, msg.receivedAt, msg.body), nil
		}
	}
	if f.waiters == nil {
		f.waiters = make(map[int]*nadoFeedWaiter)
	}
	id := f.nextID
	f.nextID++
	f.waiters[id] = waiter
	f.mu.Unlock()

	select {
	case <-ctx.Done():
		f.mu.Lock()
		delete(f.waiters, id)
		f.mu.Unlock()
		return nadoFeedNetResult(start, time.Now(), nil), ctx.Err()
	case result := <-waiter.ch:
		return result.result, result.err
	}
}

func (f *nadoFeed) readLoop(client *confirmws.Client) {
	for {
		msg, body, receivedAt, err := client.ReadJSON(context.Background())
		if err != nil {
			f.fail(client, err)
			return
		}
		f.dispatch(nadoFeedMessage{value: msg, body: body, receivedAt: receivedAt})
	}
}

func (f *nadoFeed) dispatch(msg nadoFeedMessage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recent = append(f.recent, msg)
	f.trimRecentLocked(msg.receivedAt)
	for id, waiter := range f.waiters {
		if msg.receivedAt.Before(waiter.start) {
			continue
		}
		ok, err := waiter.match(msg.value)
		if err != nil || ok {
			waiter.ch <- nadoFeedResult{result: nadoFeedNetResult(waiter.start, msg.receivedAt, msg.body), err: err}
			delete(f.waiters, id)
		}
	}
}

func (f *nadoFeed) fail(client *confirmws.Client, err error) {
	f.mu.Lock()
	if f.client != client {
		f.mu.Unlock()
		_ = client.Close()
		return
	}
	f.client = nil
	f.authUntil = time.Time{}
	waiters := f.waiters
	f.waiters = nil
	f.mu.Unlock()

	_ = client.Close()
	for _, waiter := range waiters {
		waiter.ch <- nadoFeedResult{result: nadoFeedNetResult(waiter.start, time.Now(), nil), err: err}
	}
}

func (f *nadoFeed) trimRecentLocked(now time.Time) {
	min := 0
	if len(f.recent) > nadoFeedRecentLimit {
		min = len(f.recent) - nadoFeedRecentLimit
	}
	cutoff := now.Add(-nadoFeedRecentAge)
	for min < len(f.recent) && f.recent[min].receivedAt.Before(cutoff) {
		min++
	}
	if min > 0 {
		copy(f.recent, f.recent[min:])
		f.recent = f.recent[:len(f.recent)-min]
	}
}

func nadoFeedNetResult(start time.Time, finish time.Time, body []byte) netlatency.Result {
	if start.IsZero() {
		start = finish
	}
	if finish.IsZero() {
		finish = time.Now().UTC()
	}
	if finish.Before(start) {
		finish = start
	}
	totalNS := finish.Sub(start).Nanoseconds()
	return netlatency.Result{
		BytesRead: int64(len(body)),
		Body:      body,
		Trace: netlatency.Trace{
			StartedAt:      start.UTC(),
			TotalNS:        totalNS,
			Transport:      "websocket",
			TTFBNS:         totalNS,
			ResponseWaitNS: totalNS,
		},
	}
}

func nadoAuthExpiration(auth map[string]any) time.Time {
	tx, ok := auth["tx"].(map[string]any)
	if !ok {
		return time.Time{}
	}
	raw := strings.TrimSpace(confirmutil.Text(tx["expiration"]))
	if raw == "" {
		return time.Time{}
	}
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || ms <= 0 {
		return time.Time{}
	}
	return time.Unix(0, ms*int64(time.Millisecond)).UTC()
}

func dialSubscribedOrderUpdates(ctx context.Context, wsURL string, auth map[string]any, subaccount string, productID any) (*confirmws.Client, time.Time, error) {
	client, err := dialOrderUpdates(ctx, wsURL, auth, subaccount, productID)
	if err != nil {
		return nil, time.Time{}, err
	}
	authUntil := nadoAuthExpiration(auth)
	if !authUntil.IsZero() && time.Until(authUntil) <= 0 {
		_ = client.Close()
		return nil, time.Time{}, fmt.Errorf("nado subscription auth already expired")
	}
	return client, authUntil, nil
}
