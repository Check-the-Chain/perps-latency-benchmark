package accountfeed

import (
	"context"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/venues/confirmutil"
)

type Matcher func(map[string]any) (bool, error)

type CancelMatcher func(map[string]any, map[string]struct{}) bool

func NewPersistentConfirmation(ctx context.Context, feed *Feed, opts FeedOptions, match Matcher) (*bench.Confirmation, error) {
	if err := feed.Ensure(ctx, opts); err != nil {
		return nil, err
	}
	return &bench.Confirmation{
		Wait: func(ctx context.Context, submission netlatency.Result) (netlatency.Result, error) {
			if err := feed.Ensure(ctx, opts); err != nil {
				return netlatency.Result{}, err
			}
			return feed.Wait(ctx, confirmutil.Start(submission.Trace), match)
		},
	}, nil
}

func NewPersistentCancelConfirmation(ctx context.Context, feed *Feed, opts FeedOptions, ids map[string]struct{}, match CancelMatcher) (*bench.Confirmation, error) {
	remaining := confirmutil.CopyIDSet(ids)
	return NewPersistentConfirmation(ctx, feed, opts, func(msg map[string]any) (bool, error) {
		return match(msg, remaining), nil
	})
}
