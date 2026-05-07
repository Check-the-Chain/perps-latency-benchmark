package accountfeed

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"perps-latency-benchmark/internal/confirmws"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/venues/confirmutil"
)

const (
	feedRecentLimit = 1024
	feedRecentAge   = 10 * time.Second
	feedAuthSkew    = 10 * time.Second
)

var sharedFeeds sync.Map

type Feed struct {
	key string

	connectMu sync.Mutex
	mu        sync.Mutex
	client    *confirmws.Client
	authUntil time.Time
	dialKey   string
	waiters   map[int]*feedWaiter
	recent    []feedMessage
	nextID    int
}

type FeedOptions struct {
	AuthUntil time.Time
	DialKey   string
	Dial      func(context.Context) (*confirmws.Client, error)
}

type feedWaiter struct {
	start time.Time
	match Matcher
	ch    chan feedResult
}

type feedMessage struct {
	value      map[string]any
	body       []byte
	receivedAt time.Time
}

type feedResult struct {
	result netlatency.Result
	err    error
}

func SharedFeed(key string) *Feed {
	value, _ := sharedFeeds.LoadOrStore(key, &Feed{key: key})
	return value.(*Feed)
}

func FeedKey(parts ...any) string {
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		values = append(values, confirmutil.Text(part))
	}
	return strings.Join(values, "\x00")
}

func (f *Feed) Ensure(ctx context.Context, opts FeedOptions) error {
	if opts.Dial == nil {
		return fmt.Errorf("account feed %q missing dial function", f.key)
	}
	f.connectMu.Lock()
	defer f.connectMu.Unlock()

	if f.readyLockedTime(opts) {
		return nil
	}
	if !opts.AuthUntil.IsZero() && time.Until(opts.AuthUntil) <= 0 {
		return fmt.Errorf("account feed %q auth already expired", f.key)
	}

	client, err := opts.Dial(ctx)
	if err != nil {
		return err
	}

	f.mu.Lock()
	old := f.client
	waiters := f.waiters
	f.client = client
	f.authUntil = opts.AuthUntil
	f.dialKey = opts.DialKey
	f.waiters = nil
	f.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	for _, waiter := range waiters {
		waiter.ch <- feedResult{result: feedNetResult(waiter.start, time.Now(), nil), err: fmt.Errorf("account feed %q reconnecting", f.key)}
	}
	go f.readLoop(client)
	return nil
}

func (f *Feed) Wait(ctx context.Context, start time.Time, match Matcher) (netlatency.Result, error) {
	if match == nil {
		return feedNetResult(start, time.Now(), nil), fmt.Errorf("account feed %q missing match function", f.key)
	}
	if start.IsZero() {
		start = time.Now().UTC()
	}
	waiter := &feedWaiter{
		start: start,
		match: match,
		ch:    make(chan feedResult, 1),
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
			return feedNetResult(start, msg.receivedAt, msg.body), err
		}
		if ok {
			f.mu.Unlock()
			return feedNetResult(start, msg.receivedAt, msg.body), nil
		}
	}
	if f.waiters == nil {
		f.waiters = make(map[int]*feedWaiter)
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
		return feedNetResult(start, time.Now(), nil), ctx.Err()
	case result := <-waiter.ch:
		return result.result, result.err
	}
}

func (f *Feed) readyLockedTime(opts FeedOptions) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.client == nil {
		return false
	}
	if opts.DialKey != "" && f.dialKey != opts.DialKey {
		return false
	}
	return f.authUntil.IsZero() || time.Until(f.authUntil) > feedAuthSkew
}

func (f *Feed) readLoop(client *confirmws.Client) {
	for {
		msg, body, receivedAt, err := client.ReadJSON(context.Background())
		if err != nil {
			f.fail(client, err)
			return
		}
		f.dispatch(feedMessage{value: msg, body: body, receivedAt: receivedAt})
	}
}

func (f *Feed) dispatch(msg feedMessage) {
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
			waiter.ch <- feedResult{result: feedNetResult(waiter.start, msg.receivedAt, msg.body), err: err}
			delete(f.waiters, id)
		}
	}
}

func (f *Feed) fail(client *confirmws.Client, err error) {
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
		waiter.ch <- feedResult{result: feedNetResult(waiter.start, time.Now(), nil), err: err}
	}
}

func (f *Feed) trimRecentLocked(now time.Time) {
	min := 0
	if len(f.recent) > feedRecentLimit {
		min = len(f.recent) - feedRecentLimit
	}
	cutoff := now.Add(-feedRecentAge)
	for min < len(f.recent) && f.recent[min].receivedAt.Before(cutoff) {
		min++
	}
	if min > 0 {
		copy(f.recent, f.recent[min:])
		f.recent = f.recent[:len(f.recent)-min]
	}
}

func feedNetResult(start time.Time, finish time.Time, body []byte) netlatency.Result {
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
