package nado

import (
	"context"
	"testing"
	"time"
)

func TestNadoFeedReplaysRecentMessage(t *testing.T) {
	feed := &nadoFeed{}
	start := time.Now().UTC()
	feed.dispatch(nadoFeedMessage{
		value: map[string]any{
			"type":   "order_update",
			"digest": "0xabc",
			"reason": "placed",
		},
		body:       []byte(`{"type":"order_update","digest":"0xabc","reason":"placed"}`),
		receivedAt: start.Add(time.Millisecond),
	})

	result, err := feed.wait(context.Background(), start, func(msg map[string]any) (bool, error) {
		return matchNadoConfirmation(msg, map[string]struct{}{"0xabc": {}}, "post_only")
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

func TestNadoFeedDispatchesWaiter(t *testing.T) {
	feed := &nadoFeed{}
	start := time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := feed.wait(ctx, start, func(msg map[string]any) (bool, error) {
			return matchNadoConfirmation(msg, map[string]struct{}{"0xdef": {}}, "post_only")
		})
		done <- err
	}()

	time.Sleep(10 * time.Millisecond)
	feed.dispatch(nadoFeedMessage{
		value: map[string]any{
			"type":   "order_update",
			"digest": "0xdef",
			"reason": "placed",
		},
		receivedAt: start.Add(20 * time.Millisecond),
	})

	if err := <-done; err != nil {
		t.Fatalf("wait error = %v", err)
	}
}
