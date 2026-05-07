package accountfeed

import (
	"context"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/confirmws"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/venues/confirmutil"
)

type Matcher func(map[string]any) (bool, error)

type CancelMatcher func(map[string]any, map[string]struct{}) bool

func NewConfirmation(client *confirmws.Client, match Matcher) *bench.Confirmation {
	return &bench.Confirmation{
		Wait: func(ctx context.Context, submission netlatency.Result) (netlatency.Result, error) {
			return client.Wait(ctx, confirmutil.Start(submission.Trace), match)
		},
		Close: client.Close,
	}
}

func NewCancelConfirmation(client *confirmws.Client, ids map[string]struct{}, match CancelMatcher) *bench.Confirmation {
	remaining := confirmutil.CopyIDSet(ids)
	return NewConfirmation(client, func(msg map[string]any) (bool, error) {
		return match(msg, remaining), nil
	})
}
