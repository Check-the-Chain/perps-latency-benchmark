package bench

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/netlatency"
)

type Runner struct {
	Config  Config
	Client  *netlatency.Client
	Venue   Venue
	Cleanup CleanupAdapter
}

func (r Runner) Run(ctx context.Context) (Result, error) {
	cfg := r.Config.Normalized()
	if r.Client == nil {
		return Result{}, fmt.Errorf("missing network client")
	}
	if r.Venue == nil {
		return Result{}, fmt.Errorf("missing venue")
	}
	defer r.Venue.Close(ctx)
	if r.Cleanup != nil {
		defer r.Cleanup.Close(ctx)
	}

	if cfg.RatePerSecond > 0 {
		return r.runOpenLoop(ctx, cfg), nil
	}
	return r.runClosedLoop(ctx, cfg), nil
}

func (r Runner) runClosedLoop(ctx context.Context, cfg Config) Result {
	total := cfg.Warmups + cfg.Iterations
	samples := make([]Sample, 0, total)
	for index := range total {
		warmup := index < cfg.Warmups
		iteration := index - cfg.Warmups
		sample := r.runOnce(ctx, cfg, index, iteration, warmup, time.Time{})
		samples = append(samples, sample)
		if !sample.OK && cfg.StopOnError {
			break
		}
	}

	return Result{
		Venue:       r.Venue.Name(),
		Scenario:    cfg.Scenario,
		LatencyMode: cfg.LatencyMode,
		Samples:     samples,
	}
}

func (r Runner) runOpenLoop(ctx context.Context, cfg Config) Result {
	total := cfg.Warmups + cfg.Iterations
	samples := make([]Sample, 0, total)
	results := make(chan Sample, total)
	sem := make(chan struct{}, cfg.MaxInFlight)
	var wg sync.WaitGroup

	interval := time.Duration(float64(time.Second) / cfg.RatePerSecond)
	if interval <= 0 {
		interval = time.Nanosecond
	}
	startAt := time.Now().Add(100 * time.Millisecond)

schedule:
	for index := range total {
		scheduledAt := startAt.Add(time.Duration(index) * interval)
		if delay := time.Until(scheduledAt); delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				break schedule
			case <-timer.C:
			}
		}

		warmup := index < cfg.Warmups
		iteration := index - cfg.Warmups
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results <- r.runOnce(ctx, cfg, index, iteration, warmup, scheduledAt)
		}()
	}

	wg.Wait()
	close(results)

	for sample := range results {
		samples = append(samples, sample)
	}
	sort.Slice(samples, func(i, j int) bool {
		return samples[i].Index < samples[j].Index
	})
	if cfg.StopOnError {
		for i, sample := range samples {
			if !sample.OK {
				samples = samples[:i+1]
				break
			}
		}
	}

	return Result{
		Venue:       r.Venue.Name(),
		Scenario:    cfg.Scenario,
		LatencyMode: cfg.LatencyMode,
		Samples:     samples,
	}
}

func (r Runner) runOnce(ctx context.Context, cfg Config, index int, iteration int, warmup bool, scheduledAt time.Time) Sample {
	batchSize := 1
	if cfg.Scenario == ScenarioBatch {
		batchSize = cfg.BatchSize
	}

	prepareStart := time.Now()
	prepared, err := r.Venue.Prepare(ctx, cfg.Scenario, iteration, batchSize)
	preparedNS := time.Since(prepareStart).Nanoseconds()
	if err != nil {
		return Sample{
			Venue:       r.Venue.Name(),
			Scenario:    cfg.Scenario,
			Index:       index,
			Iteration:   iteration,
			Warmup:      warmup,
			BatchSize:   batchSize,
			ScheduledAt: scheduledAt.UTC(),
			PreparedNS:  preparedNS,
			OK:          false,
			Error:       fmt.Sprintf("prepare: %v", err),
			Classification: lifecycle.Classification{
				Status: lifecycle.StatusTransportError,
				Reason: err.Error(),
			},
			CompletedAt: time.Now().UTC(),
		}
	}

	netResult, err := r.execute(ctx, prepared)
	classification := classify(prepared, lifecycle.ResponseInput{
		StatusCode: netResult.StatusCode,
		Body:       netResult.Body,
		Err:        err,
	})
	networkNS := networkDurationNS(cfg.LatencyMode, netResult.Trace)
	sentAt := netResult.Trace.StartedAt
	if scheduledAt.IsZero() {
		scheduledAt = sentAt
	}
	completedAt := time.Now().UTC()
	sample := Sample{
		Venue:          r.Venue.Name(),
		Scenario:       cfg.Scenario,
		Transport:      transportName(prepared.Transport, netResult.Trace.Transport),
		Index:          index,
		Iteration:      iteration,
		Warmup:         warmup,
		BatchSize:      batchSize,
		ScheduledAt:    scheduledAt.UTC(),
		SentAt:         sentAt.UTC(),
		PreparedNS:     preparedNS,
		NetworkNS:      networkNS,
		CorrectedNS:    correctedDurationNS(scheduledAt, completedAt),
		StartDelayNS:   startDelayNS(scheduledAt, sentAt),
		StatusCode:     netResult.StatusCode,
		BytesRead:      netResult.BytesRead,
		OK:             err == nil && classification.OK(),
		Classification: classification,
		Trace:          netResult.Trace,
		Metadata:       prepared.Metadata,
		CompletedAt:    completedAt,
	}
	if err != nil {
		sample.Error = fmt.Sprintf("request: %v", err)
	} else if !classification.OK() {
		sample.Error = fmt.Sprintf("request: %s", classification.Status)
		if classification.Reason != "" {
			sample.Error += ": " + classification.Reason
		}
	}
	if cleanup := r.cleanupAfterSample(ctx, cfg, sample); cleanup != nil {
		sample.Cleanup = cleanup
		if cfg.Cleanup.Mode == CleanupModeStrict && !cleanup.OK {
			sample.OK = false
			sample.Error = appendError(sample.Error, "cleanup: "+cleanup.Error)
		}
	}
	return sample
}

func (r Runner) cleanupAfterSample(ctx context.Context, cfg Config, sample Sample) *CleanupResult {
	if r.Cleanup == nil || !cfg.Cleanup.Enabled || cfg.Cleanup.Scope != CleanupScopeAfterSample {
		return nil
	}
	if !sample.OK {
		return nil
	}
	cleanup := r.Cleanup.AfterSample(ctx, sample)
	return &cleanup
}

func (r Runner) execute(ctx context.Context, prepared PreparedRequest) (netlatency.Result, error) {
	if prepared.Execute != nil {
		return prepared.Execute(ctx)
	}
	return r.Client.Do(ctx, prepared.Request)
}

func classify(prepared PreparedRequest, input lifecycle.ResponseInput) lifecycle.Classification {
	if prepared.Classifier != nil {
		return prepared.Classifier(input)
	}
	return lifecycle.ClassifyResponse(input)
}

func networkDurationNS(mode LatencyMode, trace netlatency.Trace) int64 {
	if mode == LatencyModeTTFB && trace.TTFBNS > 0 {
		return trace.TTFBNS
	}
	return trace.TotalNS
}

func correctedDurationNS(scheduledAt time.Time, completedAt time.Time) int64 {
	if scheduledAt.IsZero() || completedAt.IsZero() || completedAt.Before(scheduledAt) {
		return 0
	}
	return completedAt.Sub(scheduledAt).Nanoseconds()
}

func startDelayNS(scheduledAt time.Time, sentAt time.Time) int64 {
	if scheduledAt.IsZero() || sentAt.IsZero() || sentAt.Before(scheduledAt) {
		return 0
	}
	return sentAt.Sub(scheduledAt).Nanoseconds()
}

func transportName(prepared string, measured string) string {
	if prepared != "" {
		return prepared
	}
	if measured != "" {
		return measured
	}
	return "http"
}

func appendError(current string, extra string) string {
	if current == "" {
		return extra
	}
	if extra == "" {
		return current
	}
	return current + "; " + extra
}
