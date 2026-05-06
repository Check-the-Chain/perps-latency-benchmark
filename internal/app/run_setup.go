package app

import (
	"context"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/venues/spec"
)

type runResources struct {
	VenueName  string
	Config     fileConfig
	Bench      bench.Config
	Client     *netlatency.Client
	Venue      bench.Venue
	Cleanup    bench.CleanupAdapter
	Definition spec.Definition
	Runtime    spec.RuntimeConfig
}

func buildRunResources(ctx context.Context, venueName string, cfg fileConfig, runID string) (*runResources, error) {
	_ = ctx
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
	}
	if runtime, ok := resolveVenueRuntime(venueName, cfg); ok {
		resources.Definition = runtime.Definition
		resources.Runtime = runtime.RuntimeConfig()
	}

	client := netlatency.NewClient(netlatency.ClientConfig{
		Timeout:             durationMS(cfg.HTTP.TimeoutMS),
		MaxIdleConns:        cfg.HTTP.MaxIdleConns,
		MaxIdleConnsPerHost: cfg.HTTP.MaxIdleConnsPerHost,
		DisableCompression:  cfg.HTTP.DisableCompression,
	})
	resources.Client = client

	venue, err := buildVenue(venueName, cfg)
	if err != nil {
		client.CloseIdleConnections()
		return nil, err
	}
	resources.Venue = venue

	cleanupAdapter, err := buildCleanupAdapter(venueName, cfg, client)
	if err != nil {
		_ = venue.Close(context.Background())
		client.CloseIdleConnections()
		return nil, err
	}
	resources.Cleanup = cleanupAdapter
	return resources, nil
}
