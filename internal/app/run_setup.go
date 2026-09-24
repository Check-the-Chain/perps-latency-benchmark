package app

import (
	"cmp"
	"context"
	"errors"
	"time"

	"perps-latency-benchmark/internal/accountfeed"
	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/netbaseline"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/venues/spec"
)

type runResources struct {
	VenueName             string
	Config                fileConfig
	Bench                 bench.Config
	Client                *netlatency.Client
	Venue                 bench.Venue
	Cleanup               bench.CleanupAdapter
	FeedPool              *accountfeed.Pool
	NetworkBaseline       bench.NetworkBaselineObserver
	NetworkBaselineCancel context.CancelFunc
	BookCancel            context.CancelFunc
	Definition            spec.Definition
	Runtime               spec.RuntimeConfig
}

func (r *runResources) Context(ctx context.Context) context.Context {
	if r == nil {
		return ctx
	}
	return accountfeed.WithPool(ctx, r.FeedPool)
}

func (r *runResources) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}
	var err error
	r.closeBackground()
	if r.Cleanup != nil {
		err = errors.Join(err, r.Cleanup.Close(ctx))
	}
	if r.Venue != nil {
		err = errors.Join(err, r.Venue.Close(ctx))
	}
	if r.Client != nil {
		r.Client.CloseIdleConnections()
	}
	return err
}

func (r *runResources) CloseBackground() {
	if r == nil {
		return
	}
	r.closeBackground()
	if r.Client != nil {
		r.Client.CloseIdleConnections()
	}
}

func (r *runResources) closeBackground() {
	if r.NetworkBaselineCancel != nil {
		r.NetworkBaselineCancel()
		r.NetworkBaselineCancel = nil
	}
	if r.BookCancel != nil {
		r.BookCancel()
		r.BookCancel = nil
	}
}

func buildRunResources(ctx context.Context, venueName string, cfg fileConfig, runID string, feedPool *accountfeed.Pool) (*runResources, error) {
	benchConfig := cfg.Benchmark.toBenchConfig()
	if runID != "" {
		benchConfig.RunID = runID
	}
	if benchConfig.RunID == "" {
		benchConfig.RunID = bench.NewRunID()
	}
	benchConfig.Cleanup = cfg.Cleanup.toBenchCleanupConfig()
	cfg.Benchmark.RunID = benchConfig.RunID
	injectRunID(&cfg, venueName, benchConfig.RunID)

	resources := &runResources{
		VenueName: venueName,
		Config:    cfg,
		Bench:     benchConfig,
		FeedPool:  feedPool,
	}
	if resources.FeedPool == nil {
		resources.FeedPool = accountfeed.NewPool()
	}
	if runtime, ok := resolveVenueRuntime(venueName, cfg); ok {
		resources.Definition = runtime.Definition
		resources.Runtime = runtime.RuntimeConfig()
		resources.NetworkBaseline, resources.NetworkBaselineCancel = buildNetworkBaseline(ctx, runtime, cfg)
	}
	var builderHook prebuiltBuilderHook
	if resources.Definition.Name != "" && dynamicPostOnlyPricingEnabled(resources.Runtime.Params) {
		if dynamicPostOnlyHTTPPricingEnabled(resources.Definition.Name, resources.Runtime.Params) {
			builderHook = dynamicPostOnlyHTTPPriceHook(resources.Definition.Name, resources.Runtime.Params, resources.Definition, resources.Runtime)
		} else {
			tracker, cancel := startBookTracker(ctx, resources.Definition, resources.Runtime)
			if tracker == nil {
				resources.CloseBackground()
				return nil, dynamicPostOnlyBookUnavailable(resources.Definition.Name)
			}
			resources.BookCancel = cancel
			builderHook = dynamicPostOnlyPriceHook(resources.Definition.Name, resources.Runtime.Params, tracker)
		}
	}

	client := netlatency.NewClient(netlatency.ClientConfig{
		Timeout:             durationMS(cfg.HTTP.TimeoutMS),
		MaxIdleConns:        cfg.HTTP.MaxIdleConns,
		MaxIdleConnsPerHost: cfg.HTTP.MaxIdleConnsPerHost,
		DisableCompression:  cfg.HTTP.DisableCompression,
	})
	resources.Client = client

	venue, err := buildVenue(venueName, cfg, resources.NetworkBaseline, builderHook)
	if err != nil {
		resources.CloseBackground()
		return nil, err
	}
	resources.Venue = venue

	cleanupAdapter, err := buildCleanupAdapter(venueName, cfg, client)
	if err != nil {
		_ = resources.Close(context.Background())
		return nil, err
	}
	resources.Cleanup = cleanupAdapter
	return resources, nil
}

func buildNetworkBaseline(ctx context.Context, runtime resolvedVenueRuntime, cfg fileConfig) (bench.NetworkBaselineObserver, context.CancelFunc) {
	rawURL := networkBaselineURL(runtime, cfg)
	target, ok := netbaseline.TargetFromURL(rawURL)
	if !ok {
		return nil, nil
	}
	monitor := netbaseline.NewMonitor(target, 30*time.Second, 60)
	primeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	_ = monitor.Prime(primeCtx)
	cancel()
	baselineCtx, stop := context.WithCancel(ctx)
	monitor.Start(baselineCtx)
	return monitor, stop
}

func networkBaselineURL(runtime resolvedVenueRuntime, cfg fileConfig) string {
	transport := normalizedTransport(runtime.Request.Transport)
	if transport == "websocket" {
		if runtime.Request.WSBatchURL != "" && cfg.Benchmark.toBenchConfig().Scenario == bench.ScenarioBatch {
			return runtime.Request.WSBatchURL
		}
		return cmp.Or(runtime.Request.WSURL, runtime.Definition.DefaultWSURL)
	}
	if runtime.Request.BatchURL != "" && cfg.Benchmark.toBenchConfig().Scenario == bench.ScenarioBatch {
		return runtime.Request.BatchURL
	}
	return cmp.Or(runtime.Request.URL, runtime.Definition.HTTPURL(runtime.Config.BaseURL), runtime.BaseURL())
}
