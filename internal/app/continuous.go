package app

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

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
	cfg, err := loadFileConfig(opts.configPath)
	if err != nil {
		return err
	}
	applyFlagOverrides(cmd, &opts.runOptions, &cfg)
	normalizeFileConfig(&cfg)
	if err := prepareRuntimeEnvironment(cfg, &opts.runOptions); err != nil {
		return err
	}
	if opts.chunkIterations <= 0 {
		return fmt.Errorf("chunk-iterations must be positive")
	}
	if cfg.Benchmark.RatePerSecond <= 0 {
		return fmt.Errorf("run-continuous requires --rate or benchmark.rate_per_second")
	}

	venueName := normalizedVenue(cfg.Venue, "mock")
	if venueName != "mock" && !opts.confirmLive {
		return fmt.Errorf("refusing to run live venue %q without --confirm-live", venueName)
	}
	if err := validateRunConfig(venueName, cfg); err != nil {
		return err
	}
	if err := validateLifecycleForRun(venueName, cfg); err != nil {
		return err
	}
	if err := validateCleanupForRun(venueName, cfg); err != nil {
		return err
	}
	if err := checkAccountsForRun(venueName, cfg); err != nil {
		return err
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
	for chunk := 0; ; chunk++ {
		chunkStarted := time.Now()
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		chunkCfg := cloneFileConfig(cfg)
		chunkCfg.Benchmark.RunID = fmt.Sprintf("%s-%06d", baseRunID, chunk)
		chunkCfg.Benchmark.Iterations = opts.chunkIterations
		if chunk > 0 {
			chunkCfg.Benchmark.Warmups = 0
		}
		result, err := runWithConfig(ctx, venueName, chunkCfg)
		if err != nil {
			return err
		}
		if err := db.WriteSamples(ctx, sampleRecords(result)); err != nil {
			return err
		}
		if opts.retainHours > 0 {
			if err := db.DeleteBefore(ctx, time.Now().Add(-time.Duration(opts.retainHours)*time.Hour)); err != nil {
				return err
			}
		}
		fmt.Fprintln(cmd.OutOrStdout(), bench.FormatSummary(result))
		if err := sleepContinuousChunk(ctx, chunkStarted, cfg.Benchmark.RatePerSecond, chunkCfg.Benchmark.Warmups+chunkCfg.Benchmark.Iterations); err != nil {
			return nil
		}
	}
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

func sampleRecords(result bench.Result) []store.SampleRecord {
	records := make([]store.SampleRecord, 0, len(result.Samples))
	for _, sample := range result.Samples {
		records = append(records, store.SampleRecord{Sample: sample, LatencyMode: result.LatencyMode})
	}
	return records
}
