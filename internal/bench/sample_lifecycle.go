package bench

import "perps-latency-benchmark/internal/netlatency"

type sampleLifecycle struct {
	client          *netlatency.Client
	venue           Venue
	cleanup         CleanupAdapter
	networkBaseline NetworkBaselineObserver
}

func (r Runner) sampleLifecycle() sampleLifecycle {
	return sampleLifecycle{
		client:          r.Client,
		venue:           r.Venue,
		cleanup:         r.Cleanup,
		networkBaseline: r.NetworkBaseline,
	}
}
