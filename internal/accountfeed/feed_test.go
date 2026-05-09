package accountfeed

import (
	"context"
	"errors"
	"testing"
	"time"

	"perps-latency-benchmark/internal/confirmws"
)

func TestFeedReplaysRecentMessage(t *testing.T) {
	feed := &Feed{key: "test"}
	start := time.Now().UTC()
	feed.dispatch(feedMessage{
		value:      map[string]any{"id": "abc"},
		body:       []byte(`{"id":"abc"}`),
		receivedAt: start.Add(time.Millisecond),
	})

	result, err := feed.Wait(context.Background(), start, func(msg map[string]any) (bool, error) {
		return msg["id"] == "abc", nil
	})
	if err != nil {
		t.Fatalf("wait error = %v", err)
	}
	if result.Trace.TotalNS != int64(time.Millisecond) {
		t.Fatalf("total ns = %d, want %d", result.Trace.TotalNS, int64(time.Millisecond))
	}
	if result.BytesRead == 0 {
		t.Fatal("expected bytes read from replayed message")
	}
}

func TestFeedDispatchesWaiter(t *testing.T) {
	feed := &Feed{key: "test"}
	start := time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := feed.Wait(ctx, start, func(msg map[string]any) (bool, error) {
			return msg["id"] == "def", nil
		})
		done <- err
	}()

	time.Sleep(10 * time.Millisecond)
	feed.dispatch(feedMessage{
		value:      map[string]any{"id": "def"},
		receivedAt: start.Add(20 * time.Millisecond),
	})

	if err := <-done; err != nil {
		t.Fatalf("wait error = %v", err)
	}
}

func TestFeedPropagatesMatchErrorToOneWaiter(t *testing.T) {
	feed := &Feed{key: "test"}
	start := time.Now().UTC()
	want := errors.New("terminal")
	feed.dispatch(feedMessage{
		value:      map[string]any{"id": "bad"},
		receivedAt: start.Add(time.Millisecond),
	})

	result, err := feed.Wait(context.Background(), start, func(msg map[string]any) (bool, error) {
		return false, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if result.Trace.StartedAt.IsZero() {
		t.Fatal("expected result trace")
	}
}

func TestPoolKeepsFeedsIsolated(t *testing.T) {
	first := NewPool()
	second := NewPool()

	if first.Feed("same") == second.Feed("same") {
		t.Fatal("expected separate pools to isolate feed instances")
	}
}

func TestFeedFromContextUsesRuntimePool(t *testing.T) {
	pool := NewPool()
	ctx := WithPool(context.Background(), pool)

	if got, want := FeedFromContext(ctx, "runtime"), pool.Feed("runtime"); got != want {
		t.Fatalf("feed = %p, want %p", got, want)
	}
}

func TestFeedInvalidatesStaleClientOnWaitTimeout(t *testing.T) {
	feed := &Feed{
		key:      "test",
		client:   &confirmws.Client{},
		lastRead: time.Now().Add(-time.Minute),
	}
	start := time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	_, err := feed.Wait(ctx, start, func(map[string]any) (bool, error) {
		return false, nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	if feed.client != nil {
		t.Fatal("expected stale client to be invalidated")
	}
}
