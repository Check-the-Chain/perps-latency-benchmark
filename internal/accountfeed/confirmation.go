package accountfeed

import (
	"context"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/payload"
	"perps-latency-benchmark/internal/venues/confirmutil"
)

type Matcher func(map[string]any) (bool, error)

type CancelMatcher func(map[string]any, map[string]struct{}) bool

type ConfirmationBinding struct {
	FeedKey string
	Options FeedOptions
	Match   Matcher
}

type CancelConfirmationBinding struct {
	FeedKey string
	Options FeedOptions
	Match   CancelMatcher
	Verify  CancelVerifier
}

type ConfirmationBuilder func(Plan) (ConfirmationBinding, error)

type CancelConfirmationBuilder func(Plan) (CancelConfirmationBinding, error)

type CancelVerifier func(context.Context, netlatency.Result, map[string]struct{}) (netlatency.Result, bool, error)

func NewConfirmation(ctx context.Context, built payload.Built, planOptions PlanOptions, build ConfirmationBuilder) (*bench.Confirmation, error) {
	plan, ok, err := DecodePlan(built, planOptions)
	if !ok || err != nil {
		return nil, err
	}
	if build == nil {
		return nil, nil
	}
	binding, err := build(plan)
	if err != nil {
		return nil, err
	}
	if binding.FeedKey == "" || binding.Match == nil {
		return nil, nil
	}
	return NewPersistentConfirmation(ctx, FeedFromContext(ctx, binding.FeedKey), binding.Options, binding.Match)
}

func NewCancelConfirmation(ctx context.Context, built payload.Built, planOptions PlanOptions, build CancelConfirmationBuilder) (*bench.Confirmation, error) {
	plan, ok, err := DecodePlan(built, planOptions)
	if !ok || err != nil {
		return nil, err
	}
	if build == nil {
		return nil, nil
	}
	binding, err := build(plan)
	if err != nil {
		return nil, err
	}
	if binding.FeedKey == "" || binding.Match == nil {
		return nil, nil
	}
	return NewPersistentCancelConfirmationWithVerifier(ctx, FeedFromContext(ctx, binding.FeedKey), binding.Options, plan.IDs, binding.Match, binding.Verify)
}

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
	return NewPersistentCancelConfirmationWithVerifier(ctx, feed, opts, ids, match, nil)
}

func NewPersistentCancelConfirmationWithVerifier(ctx context.Context, feed *Feed, opts FeedOptions, ids map[string]struct{}, match CancelMatcher, verify CancelVerifier) (*bench.Confirmation, error) {
	remaining := confirmutil.CopyIDSet(ids)
	confirmation, err := NewPersistentConfirmation(ctx, feed, opts, func(msg map[string]any) (bool, error) {
		return match(msg, remaining), nil
	})
	if err != nil || confirmation == nil || verify == nil {
		return confirmation, err
	}
	confirmation.Verify = func(ctx context.Context, submission netlatency.Result) (netlatency.Result, bool, error) {
		return verify(ctx, submission, confirmutil.CopyIDSet(ids))
	}
	return confirmation, nil
}
