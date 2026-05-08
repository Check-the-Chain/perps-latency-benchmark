package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/booktop"
	costadapter "perps-latency-benchmark/internal/cost"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/store"
	"perps-latency-benchmark/internal/venues/spec"
)

type coordinatedOptions struct {
	configPaths        []string
	envFiles           []string
	storePath          string
	runID              string
	interval           time.Duration
	prepareLead        time.Duration
	spinLead           time.Duration
	lockThreads        bool
	expectedFill       bool
	warmupCycles       int
	cycles             int
	retainHours        int
	actualCost         bool
	confirmLive        bool
	allowInlineSecrets bool
}

type coordinatedRunItem struct {
	venueName  string
	config     fileConfig
	benchCfg   bench.Config
	client     *netlatency.Client
	resources  *runResources
	lock       *runLock
	rateLimits *rateLimitState
	cost       *costadapter.CommandAdapter
	bookCancel context.CancelFunc
}

func newRunCoordinatedCommand() *cobra.Command {
	opts := &coordinatedOptions{}
	cmd := &cobra.Command{
		Use:   "run-coordinated",
		Short: "Run multiple benchmark configs on the same schedule with warm long-lived clients.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCoordinated(cmd.Context(), cmd, opts)
		},
	}
	flags := cmd.Flags()
	flags.StringArrayVar(&opts.configPaths, "config", nil, "JSON config file. Repeat once per venue.")
	flags.StringArrayVar(&opts.envFiles, "env-file", nil, "Load dotenv credentials file before running. Repeatable; shell environment wins.")
	flags.StringVar(&opts.storePath, "store", "data/bench.db", "SQLite result store path.")
	flags.StringVar(&opts.runID, "run-id", "", "Run identifier shared by all coordinated venue samples.")
	flags.DurationVar(&opts.interval, "interval", 5*time.Minute, "Time between coordinated send cycles.")
	flags.DurationVar(&opts.prepareLead, "prepare-lead", 8*time.Second, "How long before the send boundary to prepare signed payloads and confirmation streams.")
	flags.DurationVar(&opts.spinLead, "spin-lead", 0, "Busy-wait this long before entry and cleanup send boundaries for tighter coordination. Burns CPU while active.")
	flags.BoolVar(&opts.lockThreads, "lock-threads", false, "Lock coordinated sender goroutines to OS threads during the send barrier.")
	flags.BoolVar(&opts.expectedFill, "expected-fill", true, "Track expected entry/exit fill from warm public top-of-book streams.")
	flags.IntVar(&opts.warmupCycles, "warmup-cycles", 1, "Initial coordinated cycles excluded from stored stats.")
	flags.IntVar(&opts.cycles, "cycles", 0, "Number of measured cycles to run after warmups. Default 0 runs forever.")
	flags.IntVar(&opts.retainHours, "retain-hours", 168, "Delete stored samples older than this many hours. Set 0 to keep all samples.")
	flags.BoolVar(&opts.actualCost, "actual-cost", true, "Fetch private fills and balance snapshots outside the latency path for taker cost reconciliation when a venue adapter is available.")
	flags.BoolVar(&opts.confirmLive, "confirm-live", false, "Required for non-mock venues.")
	flags.BoolVar(&opts.allowInlineSecrets, "allow-inline-secrets", false, "Allow credentials directly in JSON config. Intended only for local debugging.")
	return cmd
}

func runCoordinated(ctx context.Context, cmd *cobra.Command, opts *coordinatedOptions) error {
	if len(opts.configPaths) == 0 {
		return fmt.Errorf("run-coordinated requires at least one --config")
	}
	if opts.interval <= 0 {
		return fmt.Errorf("interval must be positive")
	}
	if opts.prepareLead < 0 {
		return fmt.Errorf("prepare-lead must be non-negative")
	}
	if opts.warmupCycles < 0 {
		return fmt.Errorf("warmup-cycles cannot be negative")
	}
	if opts.cycles < 0 {
		return fmt.Errorf("cycles cannot be negative")
	}

	runID := opts.runID
	if runID == "" {
		runID = bench.NewRunID()
	}

	items, benchItems, err := buildCoordinatedItems(ctx, opts, runID)
	if err != nil {
		return err
	}
	defer releaseCoordinatedItems(ctx, items, benchItems)

	db, err := store.OpenSQLite(opts.storePath)
	if err != nil {
		return err
	}
	defer db.Close()

	startupCleanup := bench.BeforeCoordinatedRun(ctx, benchItems, runID)
	for _, item := range benchItems {
		cleanup := startupCleanup[item.Venue.Name()]
		if cleanup == nil || cleanup.OK || item.Config.Cleanup.Mode != bench.CleanupModeStrict {
			continue
		}
		return fmt.Errorf("%s startup cleanup failed: %s", item.Venue.Name(), cleanupErrorText(cleanup))
	}
	retainedSamples := make([]bench.Sample, 0, coordinatedSampleRetentionLimit(opts, len(benchItems)))
	for cycleIndex := 0; ; cycleIndex++ {
		if opts.cycles > 0 && cycleIndex >= opts.warmupCycles+opts.cycles {
			break
		}
		select {
		case <-ctx.Done():
			_ = bench.AfterCoordinatedRun(ctx, benchItems, runID, retainedSamples)
			return nil
		default:
		}
		warmup := cycleIndex < opts.warmupCycles
		iteration := cycleIndex - opts.warmupCycles
		target := nextCoordinatedBoundary(time.Now(), opts.interval)
		prepareAt := target.Add(-opts.prepareLead)
		if err := sleepUntil(ctx, prepareAt); err != nil {
			_ = bench.AfterCoordinatedRun(ctx, benchItems, runID, retainedSamples)
			return nil
		}
		if ok := coordinatedRateLimitPreflight(ctx, cmd, items, opts.interval); !ok {
			continue
		}
		balancesBefore := coordinatedBalances(ctx, cmd, items)
		samples := bench.RunCoordinatedCycle(ctx, benchItems, bench.CoordinatedCycle{
			Index:       cycleIndex,
			Iteration:   iteration,
			Warmup:      warmup,
			ScheduledAt: target,
			SpinLead:    opts.spinLead,
			LockThreads: opts.lockThreads,
		})
		retainedSamples = retainCoordinatedSamples(retainedSamples, samples, coordinatedSampleRetentionLimit(opts, len(benchItems)))
		if err := db.WriteSamples(ctx, coordinatedSampleRecords(samples, items)); err != nil {
			return err
		}
		if err := writeCoordinatedCosts(ctx, cmd, db, items, samples, balancesBefore); err != nil {
			return err
		}
		if opts.retainHours > 0 {
			if err := db.DeleteBefore(ctx, time.Now().Add(-time.Duration(opts.retainHours)*time.Hour)); err != nil {
				return err
			}
		}
		fmt.Fprintln(cmd.OutOrStdout(), formatCoordinatedCycleSummary(samples, warmup))
		if err := coordinatedStrictCleanupError(samples, benchItems); err != nil {
			return err
		}
	}
	_ = bench.AfterCoordinatedRun(ctx, benchItems, runID, retainedSamples)
	return nil
}

func coordinatedSampleRetentionLimit(opts *coordinatedOptions, itemCount int) int {
	if itemCount <= 0 {
		return 0
	}
	if opts.cycles > 0 {
		return itemCount * (opts.warmupCycles + opts.cycles)
	}
	if opts.retainHours > 0 && opts.interval > 0 {
		cycles := int((time.Duration(opts.retainHours) * time.Hour) / opts.interval)
		if cycles < 1 {
			cycles = 1
		}
		return itemCount * (opts.warmupCycles + cycles + 1)
	}
	const fallbackCycles = 256
	return itemCount * (opts.warmupCycles + fallbackCycles)
}

func retainCoordinatedSamples(current []bench.Sample, next []bench.Sample, limit int) []bench.Sample {
	if limit <= 0 {
		return nil
	}
	current = append(current, next...)
	if len(current) <= limit {
		return current
	}
	copy(current, current[len(current)-limit:])
	return current[:limit]
}

func coordinatedStrictCleanupError(samples []bench.Sample, items []bench.CoordinatedItem) error {
	strict := make(map[string]bool, len(items))
	for _, item := range items {
		strict[item.Venue.Name()] = item.Config.Cleanup.Mode == bench.CleanupModeStrict
	}
	for _, sample := range samples {
		if !strict[sample.Venue] || sample.Cleanup == nil || sample.Cleanup.OK {
			continue
		}
		return fmt.Errorf("%s cleanup failed: %s", sample.Venue, cleanupErrorText(sample.Cleanup))
	}
	return nil
}

func cleanupErrorText(cleanup *bench.CleanupResult) string {
	if cleanup == nil {
		return "unknown cleanup failure"
	}
	if cleanup.Error != "" {
		return cleanup.Error
	}
	if cleanup.Description != "" {
		return cleanup.Description
	}
	return "unknown cleanup failure"
}

func buildCoordinatedItems(ctx context.Context, opts *coordinatedOptions, runID string) ([]coordinatedRunItem, []bench.CoordinatedItem, error) {
	_ = ctx
	items := make([]coordinatedRunItem, 0, len(opts.configPaths))
	benchItems := make([]bench.CoordinatedItem, 0, len(opts.configPaths))
	fail := func(err error) ([]coordinatedRunItem, []bench.CoordinatedItem, error) {
		releaseCoordinatedItems(context.Background(), items, benchItems)
		return nil, nil, err
	}
	for _, configPath := range opts.configPaths {
		plan, err := prepareRunPlan(ctx, runPlanOptions{
			ConfigPath:         configPath,
			EnvFiles:           opts.envFiles,
			FallbackVenue:      "mock",
			ConfirmLive:        opts.confirmLive,
			AllowInlineSecrets: opts.allowInlineSecrets,
		})
		if err != nil {
			return fail(fmt.Errorf("%s: %w", configPath, err))
		}
		cfg := plan.Config
		venueName := plan.VenueName
		lock, err := acquireRunLock(venueName, cfg)
		if err != nil {
			return fail(fmt.Errorf("%s: %w", venueName, err))
		}
		resources, err := buildRunResources(ctx, venueName, cfg, runID)
		if err != nil {
			lock.Release()
			return fail(fmt.Errorf("%s: %w", venueName, err))
		}
		var book *booktop.Tracker
		var expectedFillBookCancel context.CancelFunc
		if opts.expectedFill {
			if tracker, cancel := startBookTracker(context.Background(), resources.Definition, resources.Runtime); tracker != nil {
				book = tracker
				expectedFillBookCancel = cancel
			}
		}
		expected, _ := resources.Definition.ExpectedFillOrder(resources.Runtime)
		item := coordinatedRunItem{
			venueName:  venueName,
			config:     resources.Config,
			benchCfg:   resources.Bench,
			client:     resources.Client,
			resources:  resources,
			lock:       lock,
			rateLimits: &rateLimitState{},
			bookCancel: expectedFillBookCancel,
		}
		if opts.actualCost {
			costAdapter, err := buildCostAdapter(venueName, resources.Config)
			if err != nil {
				if expectedFillBookCancel != nil {
					expectedFillBookCancel()
				}
				_ = resources.Close(context.Background())
				lock.Release()
				return fail(fmt.Errorf("%s cost: %w", venueName, err))
			}
			item.cost = costAdapter
		}
		items = append(items, item)
		benchItems = append(benchItems, bench.CoordinatedItem{
			Config:          resources.Bench,
			Client:          resources.Client,
			Venue:           resources.Venue,
			Cleanup:         resources.Cleanup,
			NetworkBaseline: resources.NetworkBaseline,
			SpinLead:        opts.spinLead,
			LockThreads:     opts.lockThreads,
			Book:            book,
			OrderSide:       expected.Side,
			OrderSize:       expected.Size,
		})
	}
	return items, benchItems, nil
}

func releaseCoordinatedItems(ctx context.Context, items []coordinatedRunItem, benchItems []bench.CoordinatedItem) {
	for _, item := range items {
		if item.bookCancel != nil {
			item.bookCancel()
		}
		if item.resources != nil {
			_ = item.resources.Close(ctx)
		}
		if item.cost != nil {
			_ = item.cost.Close(ctx)
		}
		if item.lock != nil {
			item.lock.Release()
		}
	}
}

func coordinatedBalances(ctx context.Context, cmd *cobra.Command, items []coordinatedRunItem) map[string]bench.BalanceSnapshot {
	balances := make(map[string]bench.BalanceSnapshot)
	for _, item := range items {
		if item.cost == nil {
			continue
		}
		balance, err := item.cost.Balance(ctx, item.venueName)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s cost balance: %v\n", item.venueName, err)
			continue
		}
		balances[item.venueName] = balance
	}
	return balances
}

func writeCoordinatedCosts(ctx context.Context, cmd *cobra.Command, db *store.SQLite, items []coordinatedRunItem, samples []bench.Sample, before map[string]bench.BalanceSnapshot) error {
	byVenue := make(map[string]coordinatedRunItem, len(items))
	for _, item := range items {
		byVenue[item.venueName] = item
	}
	costs := make([]bench.SampleCost, 0, len(samples))
	for _, sample := range samples {
		item := byVenue[sample.Venue]
		if item.cost == nil || sample.Warmup || !sample.OK {
			continue
		}
		cost, err := sampleCostWithRetries(ctx, item.cost, sample, before[sample.Venue])
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s cost sample: %v\n", sample.Venue, err)
			continue
		}
		costs = append(costs, cost)
	}
	return db.WriteSampleCosts(ctx, costs)
}

func sampleCostWithRetries(ctx context.Context, adapter *costadapter.CommandAdapter, sample bench.Sample, before bench.BalanceSnapshot) (bench.SampleCost, error) {
	const attempts = 4
	const interval = 3 * time.Second
	var last bench.SampleCost
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		cost, err := adapter.SampleCost(ctx, sample, before)
		if err != nil {
			lastErr = err
			if attempt == attempts-1 {
				return bench.SampleCost{}, err
			}
			select {
			case <-ctx.Done():
				return bench.SampleCost{}, ctx.Err()
			case <-time.After(interval):
			}
			continue
		}
		last = cost
		if !retryableCost(cost) || attempt == attempts-1 {
			return cost, nil
		}
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(interval):
		}
	}
	if lastErr != nil {
		return bench.SampleCost{}, lastErr
	}
	return last, nil
}

func retryableCost(cost bench.SampleCost) bool {
	if cost.Clean {
		return false
	}
	reason := strings.ToLower(cost.QualityReason)
	if reason == "" {
		return true
	}
	if strings.Contains(reason, "existing") || strings.Contains(reason, "contaminat") {
		return false
	}
	return strings.Contains(reason, "missing entry or exit fill") ||
		strings.Contains(reason, "incomplete entry or exit fill") ||
		strings.Contains(reason, "balance reconciliation differs") ||
		strings.Contains(reason, "balance source changed") ||
		strings.Contains(reason, "temporarily") ||
		strings.Contains(reason, "not visible")
}

func startBookTracker(parent context.Context, definition spec.Definition, runtime spec.RuntimeConfig) (*booktop.Tracker, context.CancelFunc) {
	bookCfg, ok := definition.BookTopConfig(runtime)
	if !ok {
		return nil, nil
	}
	ctx, cancel := context.WithCancel(parent)
	tracker := booktop.NewTracker(bookCfg)
	go tracker.Run(ctx)
	return tracker, cancel
}

func coordinatedRateLimitPreflight(ctx context.Context, cmd *cobra.Command, items []coordinatedRunItem, interval time.Duration) bool {
	for _, item := range items {
		if err := item.rateLimits.preflight(ctx, item.venueName, item.config); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s rate limit preflight: %v\n", item.venueName, err)
			if interval > 0 {
				_ = sleepUntil(ctx, time.Now().Add(interval))
			}
			return false
		}
	}
	return true
}

func coordinatedSampleRecords(samples []bench.Sample, items []coordinatedRunItem) []store.SampleRecord {
	latencyModes := make(map[string]bench.LatencyMode, len(items))
	for _, item := range items {
		latencyModes[item.venueName] = item.benchCfg.LatencyMode
	}
	return store.SampleRecordsByVenue(samples, latencyModes)
}

func formatCoordinatedCycleSummary(samples []bench.Sample, warmup bool) string {
	label := "measured"
	if warmup {
		label = "warmup"
	}
	out := fmt.Sprintf("coordinated %s cycle:", label)
	for _, sample := range samples {
		status := "failed"
		if sample.OK {
			status = "ok"
		}
		out += fmt.Sprintf(" %s=%s %.2fms raw=%.2fms start=%.2fms write=%.2fms close_write=%.2fms",
			sample.Venue,
			status,
			float64(bench.AdjustedNetworkNS(sample))/1_000_000,
			float64(bench.RawNetworkNS(sample))/1_000_000,
			float64(sample.StartDelayNS)/1_000_000,
			float64(sample.WriteDelayNS)/1_000_000,
			cleanupWriteDelayMS(sample.Cleanup),
		)
	}
	return out
}

func cleanupWriteDelayMS(cleanup *bench.CleanupResult) float64 {
	if cleanup == nil {
		return 0
	}
	return float64(cleanup.WriteDelayNS) / 1_000_000
}

func nextCoordinatedBoundary(now time.Time, interval time.Duration) time.Time {
	if interval <= 0 {
		return now
	}
	truncated := now.Truncate(interval)
	next := truncated.Add(interval)
	if !next.After(now) {
		next = next.Add(interval)
	}
	return next
}

func sleepUntil(ctx context.Context, target time.Time) error {
	delay := time.Until(target)
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
