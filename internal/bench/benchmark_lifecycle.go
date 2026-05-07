package bench

import "context"

type benchmarkLifecycle struct {
	cfg     Config
	venue   Venue
	cleanup CleanupAdapter
}

func newBenchmarkLifecycle(cfg Config, venue Venue, cleanup CleanupAdapter) benchmarkLifecycle {
	return benchmarkLifecycle{
		cfg:     cfg.Normalized(),
		venue:   venue,
		cleanup: cleanup,
	}
}

func (l benchmarkLifecycle) result(samples []Sample) Result {
	return Result{
		Venue:           l.venue.Name(),
		RunID:           l.cfg.RunID,
		Scenario:        l.cfg.Scenario,
		LatencyMode:     l.cfg.LatencyMode,
		MeasurementMode: l.cfg.MeasurementMode,
		Samples:         samples,
	}
}

func (l benchmarkLifecycle) startupFailureResult(startup *CleanupResult) Result {
	result := l.result(nil)
	result.StartupCleanup = startup
	return result
}

func (l benchmarkLifecycle) beforeRun(ctx context.Context) *CleanupResult {
	if l.cleanup == nil || !l.cfg.Cleanup.Enabled {
		return nil
	}
	hooks, ok := l.cleanup.(RunCleanupAdapter)
	if !ok {
		return nil
	}
	cleanup := hooks.BeforeRun(ctx, CleanupRun{
		Venue:      l.venue.Name(),
		RunID:      l.cfg.RunID,
		Scenario:   l.cfg.Scenario,
		Iterations: l.cfg.Iterations,
		Warmups:    l.cfg.Warmups,
		BatchSize:  l.cfg.BatchSize,
	})
	return &cleanup
}

func (l benchmarkLifecycle) afterRun(ctx context.Context, result Result) *CleanupResult {
	if l.cleanup == nil || !l.cfg.Cleanup.Enabled {
		return nil
	}
	hooks, ok := l.cleanup.(RunCleanupAdapter)
	if !ok {
		return nil
	}
	cleanup := hooks.AfterRun(ctx, result)
	return &cleanup
}

func (l benchmarkLifecycle) strictStartupFailed(cleanup *CleanupResult) bool {
	return cleanup != nil && !cleanup.OK && l.cfg.Cleanup.Mode == CleanupModeStrict
}
