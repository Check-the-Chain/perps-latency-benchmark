package exchangetps

import (
	"context"
	"sync"
	"time"
)

type Aggregator struct {
	venue  string
	bucket time.Duration

	mu      sync.Mutex
	pending map[int64]BucketDelta
}

func NewAggregator(venue string, bucket time.Duration) *Aggregator {
	return &Aggregator{
		venue:   venue,
		bucket:  bucket,
		pending: make(map[int64]BucketDelta),
	}
}

func (a *Aggregator) BucketSeconds() int64 {
	return int64(a.bucket / time.Second)
}

func (a *Aggregator) AddBlock(timestampMs, txCount int64) {
	a.add(timestampMs, BucketDelta{TxCount: txCount, BlockCount: 1})
}

func (a *Aggregator) AddTransaction(timestampMs int64, action, errorText string) {
	delta := BucketDelta{}
	if IsOrderAction(action) {
		delta.OrderCount = 1
	}
	if IsPlaceAction(action) {
		delta.PlaceCount = 1
	}
	if IsCancelAction(action) {
		delta.CancelCount = 1
	}
	if errorText != "" {
		delta.ErrorCount = 1
	}
	if delta.OrderCount == 0 && delta.ErrorCount == 0 {
		return
	}
	a.add(timestampMs, delta)
}

func (a *Aggregator) Flush(ctx context.Context, store *Store, all bool) error {
	now := time.Now()
	deltas := a.drain(now, all)
	if len(deltas) == 0 {
		return nil
	}

	var from, to time.Time
	for i, delta := range deltas {
		if err := store.RecordLiveDelta1m(ctx, delta); err != nil {
			return err
		}
		if i == 0 || delta.BucketStart.Before(from) {
			from = delta.BucketStart
		}
		if i == 0 || delta.BucketStart.After(to) {
			to = delta.BucketStart
		}
	}
	return store.RefreshRollups(ctx, a.venue, from, to)
}

func (a *Aggregator) add(timestampMs int64, delta BucketDelta) {
	if timestampMs <= 0 {
		timestampMs = time.Now().UnixMilli()
	}
	start := a.bucketStart(timestampMs)
	a.mu.Lock()
	defer a.mu.Unlock()

	current := a.pending[start.Unix()]
	current.Venue = a.venue
	current.BucketStart = start
	current.TxCount += delta.TxCount
	current.BlockCount += delta.BlockCount
	current.OrderCount += delta.OrderCount
	current.PlaceCount += delta.PlaceCount
	current.CancelCount += delta.CancelCount
	current.ErrorCount += delta.ErrorCount
	a.pending[start.Unix()] = current
}

func (a *Aggregator) drain(now time.Time, all bool) []BucketDelta {
	a.mu.Lock()
	defer a.mu.Unlock()

	finalizedBefore := now.Add(-a.bucket - time.Second).Unix()
	deltas := make([]BucketDelta, 0, len(a.pending))
	for bucketUnix, delta := range a.pending {
		if all || bucketUnix <= finalizedBefore {
			deltas = append(deltas, delta)
			delete(a.pending, bucketUnix)
		}
	}
	return deltas
}

func (a *Aggregator) bucketStart(timestampMs int64) time.Time {
	ts := time.UnixMilli(timestampMs).UTC()
	bucketSeconds := int64(a.bucket / time.Second)
	return time.Unix(floorUnix(ts.Unix(), bucketSeconds), 0).UTC()
}

func IsOrderAction(action string) bool {
	return IsPlaceAction(action) || IsCancelAction(action)
}

func IsPlaceAction(action string) bool {
	switch action {
	case "PlaceOrder", "PlaceStrategy":
		return true
	default:
		return false
	}
}

func IsCancelAction(action string) bool {
	switch action {
	case "CancelOrder", "CancelOrders", "CountdownCancelAll":
		return true
	default:
		return false
	}
}
