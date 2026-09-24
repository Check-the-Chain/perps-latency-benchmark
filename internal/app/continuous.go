package app

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"perps-latency-benchmark/internal/accountfeed"
	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/store"
)

type continuousOptions struct {
	runOptions
	storePath       string
	chunkIterations int
	retainHours     int
}

func newRunContinuousCommand() *cobra.Command {
	opts := &continuousOptions{}
	cmd := &cobra.Command{
		Use:   "run-continuous",
		Short: "Run benchmark chunks continuously and write samples to SQLite.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runContinuous(cmd.Context(), cmd, opts)
		},
	}
	addRunFlags(cmd, &opts.runOptions)
	cmd.Flags().StringVar(&opts.storePath, "store", "data/bench.db", "SQLite result store path.")
	cmd.Flags().IntVar(&opts.chunkIterations, "chunk-iterations", 10, "Measured iterations per benchmark chunk.")
	cmd.Flags().IntVar(&opts.retainHours, "retain-hours", 168, "Delete stored samples older than this many hours. Set 0 to keep all samples.")
	return cmd
}

func runContinuous(ctx context.Context, cmd *cobra.Command, opts *continuousOptions) error {
	plan, err := prepareRunPlan(ctx, runPlanOptions{
		ConfigPath:         opts.configPath,
		FallbackVenue:      "mock",
		ConfirmLive:        opts.confirmLive,
		AllowInlineSecrets: opts.allowInlineSecrets,
		ApplyOverrides: func(cfg *fileConfig) {
			applyFlagOverrides(cmd, &opts.runOptions, cfg)
		},
	})
	if err != nil {
		return err
	}
	cfg := plan.Config
	venueName := plan.VenueName
	if opts.chunkIterations <= 0 {
		return fmt.Errorf("chunk-iterations must be positive")
	}
	if cfg.Benchmark.RatePerSecond <= 0 {
		return fmt.Errorf("run-continuous requires --rate or benchmark.rate_per_second")
	}

	lock, err := acquireRunLock(venueName, cfg)
	if err != nil {
		return err
	}
	defer lock.Release()

	db, err := store.OpenSQLite(opts.storePath)
	if err != nil {
		return err
	}
	defer db.Close()

	baseRunID := cfg.Benchmark.RunID
	if baseRunID == "" {
		baseRunID = bench.NewRunID()
	}
	session := newContinuousRunSession(ctx)
	rateLimits := &rateLimitState{}
	supervisor := continuousSupervisor{
		baseDelay: 5 * time.Second,
		maxDelay:  5 * time.Minute,
	}
	emitProcessEvent("run-continuous", venueName, baseRunID, "start", "", nil)
	for chunk := 0; ; chunk++ {
		chunkStarted := time.Now()
		select {
		case <-ctx.Done():
			emitProcessEvent("run-continuous", venueName, baseRunID, "stop", ctx.Err().Error(), nil)
			return nil
		default:
		}

		chunkCfg := cloneFileConfig(cfg)
		chunkCfg.Benchmark.RunID = fmt.Sprintf("%s-%06d", baseRunID, chunk)
		chunkCfg.Benchmark.Iterations = opts.chunkIterations
		if chunk > 0 {
			chunkCfg.Benchmark.Warmups = 0
		}
		if err := rateLimits.preflight(session.ctx, venueName, chunkCfg); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "rate limit preflight: %v\n", err)
			if err := sleepContinuousChunk(session.ctx, chunkStarted, cfg.Benchmark.RatePerSecond, chunkCfg.Benchmark.Warmups+chunkCfg.Benchmark.Iterations); err != nil {
				return nil
			}
			continue
		}
		result, err := runWithConfig(session.ctx, venueName, chunkCfg, session.feedPool)
		if err != nil {
			delay := supervisor.NextDelay()
			logContinuousRetry(cmd.ErrOrStderr(), "run", err, delay)
			emitProcessEvent("run-continuous", venueName, chunkCfg.Benchmark.RunID, "retry", err.Error(), map[string]any{"phase": "run", "delay_ms": delay.Milliseconds()})
			session = newContinuousRunSession(ctx)
			if err := sleepDuration(ctx, delay); err != nil {
				return nil
			}
			continue
		}
		if err := db.WriteSamples(ctx, store.SampleRecords(result)); err != nil {
			delay := supervisor.NextDelay()
			logContinuousRetry(cmd.ErrOrStderr(), "write samples", err, delay)
			emitProcessEvent("run-continuous", venueName, chunkCfg.Benchmark.RunID, "retry", err.Error(), map[string]any{"phase": "write_samples", "delay_ms": delay.Milliseconds()})
			if err := sleepDuration(ctx, delay); err != nil {
				return nil
			}
			continue
		}
		if opts.retainHours > 0 {
			if err := db.DeleteBefore(ctx, time.Now().Add(-time.Duration(opts.retainHours)*time.Hour)); err != nil {
				delay := supervisor.NextDelay()
				logContinuousRetry(cmd.ErrOrStderr(), "retention", err, delay)
				emitProcessEvent("run-continuous", venueName, chunkCfg.Benchmark.RunID, "retry", err.Error(), map[string]any{"phase": "retention", "delay_ms": delay.Milliseconds()})
				if err := sleepDuration(ctx, delay); err != nil {
					return nil
				}
				continue
			}
		}
		fmt.Fprintln(cmd.OutOrStdout(), bench.FormatSummary(result))
		if err := strictRunCleanupError(venueName, chunkCfg.Cleanup.toBenchCleanupConfig(), result); err != nil {
			delay := supervisor.NextDelay()
			logContinuousRetry(cmd.ErrOrStderr(), "strict cleanup", err, delay)
			emitProcessEvent("run-continuous", venueName, chunkCfg.Benchmark.RunID, "retry", err.Error(), map[string]any{"phase": "strict_cleanup", "delay_ms": delay.Milliseconds()})
			session = newContinuousRunSession(ctx)
			if err := sleepDuration(ctx, delay); err != nil {
				return nil
			}
			continue
		}
		supervisor.Reset()
		if err := sleepContinuousChunk(session.ctx, chunkStarted, cfg.Benchmark.RatePerSecond, chunkCfg.Benchmark.Warmups+chunkCfg.Benchmark.Iterations); err != nil {
			return nil
		}
	}
}

type continuousRunSession struct {
	feedPool *accountfeed.Pool
	ctx      context.Context
}

func newContinuousRunSession(ctx context.Context) continuousRunSession {
	feedPool := accountfeed.NewPool()
	return continuousRunSession{
		feedPool: feedPool,
		ctx:      accountfeed.WithPool(ctx, feedPool),
	}
}

type continuousSupervisor struct {
	failures  int
	baseDelay time.Duration
	maxDelay  time.Duration
}

func (s *continuousSupervisor) NextDelay() time.Duration {
	base := s.baseDelay
	if base <= 0 {
		base = time.Second
	}
	maxDelay := s.maxDelay
	if maxDelay <= 0 {
		maxDelay = time.Minute
	}
	delay := base
	for range s.failures {
		delay *= 2
		if delay >= maxDelay {
			delay = maxDelay
			break
		}
	}
	s.failures++
	return delay
}

func (s *continuousSupervisor) Reset() {
	s.failures = 0
}

func logContinuousRetry(w io.Writer, phase string, err error, delay time.Duration) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "continuous %s failed: %v; retrying in %s\n", phase, err, delay)
}

func sleepDuration(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func strictRunCleanupError(venueName string, cleanupCfg bench.CleanupConfig, result bench.Result) error {
	if cleanupCfg.Normalized().Mode != bench.CleanupModeStrict {
		return nil
	}
	if result.StartupCleanup != nil && bench.CleanupRequiresRestartBeforeMeasurement(result.StartupCleanup) {
		if bench.IsPositionSelfHealCleanup(result.StartupCleanup) {
			return fmt.Errorf("%s startup cleanup repaired preexisting position; restarting before measuring", venueName)
		}
		return fmt.Errorf("%s startup cleanup repaired stale state; restarting before measuring", venueName)
	}
	if result.StartupCleanup != nil && !result.StartupCleanup.OK {
		return fmt.Errorf("%s startup cleanup failed: %s", venueName, cleanupErrorText(result.StartupCleanup))
	}
	if result.Reconciliation != nil && !result.Reconciliation.OK {
		return fmt.Errorf("%s reconciliation failed: %s", venueName, cleanupErrorText(result.Reconciliation))
	}
	return nil
}

func sleepContinuousChunk(ctx context.Context, started time.Time, rate float64, samples int) error {
	if samples <= 0 {
		return nil
	}
	span := continuousChunkSpan(rate, samples)
	if span <= 0 {
		return nil
	}
	delay := time.Until(started.Add(span))
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func continuousChunkSpan(rate float64, samples int) time.Duration {
	if rate <= 0 || samples <= 0 {
		return 0
	}
	interval := time.Duration(float64(time.Second) / rate)
	if interval <= 0 {
		interval = time.Nanosecond
	}
	return time.Duration(samples) * interval
}
