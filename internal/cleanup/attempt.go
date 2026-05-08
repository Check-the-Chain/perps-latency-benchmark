package cleanup

import (
	"context"
	"fmt"
	"time"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/payload"
)

type cleanupAttempt struct {
	adapter    *CommandAdapter
	start      time.Time
	preparedNS int64
	built      payload.Built
}

func (a cleanupAttempt) prepareRoute(ctx context.Context, route cleanupRoute, routeErrors []string) (bench.PreparedCleanup, bool, string, error) {
	switch route.kind {
	case cleanupRouteWebSocket:
		return a.prepareWebSocket(ctx, route, routeErrors)
	case cleanupRouteHTTP:
		return a.prepareHTTP(ctx, route, routeErrors)
	default:
		return bench.PreparedCleanup{}, false, "", fmt.Errorf("unknown cleanup route kind %q", route.kind)
	}
}

func (a cleanupAttempt) prepareWebSocket(ctx context.Context, route cleanupRoute, routeErrors []string) (bench.PreparedCleanup, bool, string, error) {
	headers := payload.MergeHeaders(a.adapter.headers, a.built.Headers)
	client, err := a.adapter.webSocketClient(route.wsURL, headers)
	if err != nil {
		return bench.PreparedCleanup{}, false, "", err
	}
	if err := client.EnsureConnected(ctx); err != nil {
		return bench.PreparedCleanup{}, false, "websocket prepare failed: " + err.Error(), nil
	}
	confirmation, err := a.adapter.prepareCancelConfirmation(ctx, a.built)
	if err != nil {
		return bench.PreparedCleanup{}, false, "", err
	}
	preparedNS := time.Since(a.start).Nanoseconds()
	return bench.PreparedCleanup{
		PreparedNS: preparedNS,
		Execute: func(execCtx context.Context) bench.CleanupResult {
			result, err := client.Do(execCtx, route.wsBody)
			return a.outcome(execCtx, result, err, preparedNS, confirmation, cleanupRouteWebSocket, routeErrors)
		},
	}, true, "", nil
}

func (a cleanupAttempt) prepareHTTP(ctx context.Context, route cleanupRoute, routeErrors []string) (bench.PreparedCleanup, bool, string, error) {
	confirmation, err := a.adapter.prepareCancelConfirmation(ctx, a.built)
	if err != nil {
		return bench.PreparedCleanup{}, false, "", err
	}
	return bench.PreparedCleanup{
		PreparedNS: a.preparedNS,
		Execute: func(execCtx context.Context) bench.CleanupResult {
			result, err := a.adapter.cfg.Client.Do(execCtx, route.http)
			return a.outcome(execCtx, result, err, a.preparedNS, confirmation, cleanupRouteHTTP, routeErrors)
		},
	}, true, "", nil
}

func (a cleanupAttempt) outcome(ctx context.Context, result netlatency.Result, err error, preparedNS int64, confirmation *bench.Confirmation, route cleanupRouteKind, routeErrors []string) bench.CleanupResult {
	cleanup := a.adapter.execution().resultFromNetworkWithConfirmation(ctx, result, err, preparedNS, a.built, confirmation, a.adapter.cfg.Timeout)
	annotateCleanupRoute(&cleanup, string(route), routeErrors)
	return cleanup
}
