package cleanup

import (
	"cmp"
	"context"
	"fmt"
	"strings"
	"time"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/payload"
)

type cleanupExecution struct {
	classifier  lifecycle.Classifier
	description string
}

func (e cleanupExecution) resultFromNetwork(result netlatency.Result, err error, preparedNS int64, built payload.Built) bench.CleanupResult {
	classification := lifecycle.ClassifyResponse(lifecycle.ResponseInput{
		StatusCode: result.StatusCode,
		Body:       result.Body,
		Err:        err,
	})
	if e.classifier != nil {
		classification = e.classifier(lifecycle.ResponseInput{
			StatusCode: result.StatusCode,
			Body:       result.Body,
			Err:        err,
		})
	}
	ok := err == nil && classification.OK()
	cleanup := bench.CleanupResult{
		Attempted:   true,
		OK:          ok,
		StatusCode:  result.StatusCode,
		PreparedNS:  preparedNS,
		DurationNS:  result.Trace.TotalNS,
		SentAt:      result.Trace.StartedAt.UTC(),
		BytesRead:   result.BytesRead,
		Description: cmp.Or(cleanupDescription(built.Metadata), e.description, "cleanup"),
		Metadata:    cleanupMetadata(built.Metadata),
		Trace:       result.Trace,
	}
	if cleanup.DurationNS <= 0 && !result.Trace.StartedAt.IsZero() {
		cleanup.DurationNS = time.Since(result.Trace.StartedAt).Nanoseconds()
	}
	if err != nil {
		cleanup.Error = err.Error()
	} else if !ok {
		cleanup.Error = string(classification.Status)
		if classification.Reason != "" {
			cleanup.Error += ": " + classification.Reason
		}
	}
	return cleanup
}

func (e cleanupExecution) resultFromNetworkWithConfirmation(ctx context.Context, result netlatency.Result, err error, preparedNS int64, built payload.Built, confirmation *bench.Confirmation, timeout time.Duration) bench.CleanupResult {
	cleanup := e.resultFromNetwork(result, err, preparedNS, built)
	if confirmation == nil || confirmation.Wait == nil {
		return cleanup
	}
	defer func() {
		if confirmation.Close != nil {
			_ = confirmation.Close()
		}
	}()
	annotateCancelAck(&cleanup, result)
	if err != nil || !cleanup.OK {
		return cleanup
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	confirmCtx, cancel := context.WithTimeout(ctx, timeout)
	confirmed, confirmErr := confirmation.Wait(confirmCtx, result)
	cancel()
	if confirmed.Trace.TotalNS > 0 {
		cleanup.DurationNS = confirmed.Trace.TotalNS
		cleanup.SentAt = confirmed.Trace.StartedAt.UTC()
		cleanup.Trace = confirmed.Trace
		cleanup.BytesRead = confirmed.BytesRead
	}
	if confirmErr != nil {
		cleanup.OK = false
		cleanup.Error = fmt.Sprintf("cancel confirmation: %v", confirmErr)
		return cleanup
	}
	if cleanup.Metadata == nil {
		cleanup.Metadata = map[string]any{}
	}
	cleanup.Metadata["cleanup_confirmation"] = "account_feed"
	cleanup.Metadata["cleanup_confirmation_transport"] = confirmed.Trace.Transport
	return cleanup
}

func annotateCancelAck(cleanup *bench.CleanupResult, result netlatency.Result) {
	if cleanup.Metadata == nil {
		cleanup.Metadata = map[string]any{}
	}
	cleanup.Metadata["cancel_ack_duration_ns"] = result.Trace.TotalNS
	cleanup.Metadata["cancel_ack_status_code"] = result.StatusCode
	cleanup.Metadata["cancel_ack_bytes_read"] = result.BytesRead
	cleanup.Metadata["cancel_ack_transport"] = result.Trace.Transport
}

func annotateCleanupRoute(cleanup *bench.CleanupResult, transport string, routeErrors []string) {
	if cleanup.Metadata == nil {
		cleanup.Metadata = map[string]any{}
	}
	cleanup.Metadata["cleanup_transport"] = transport
	if len(routeErrors) > 0 {
		cleanup.Metadata["cleanup_route_fallback"] = strings.Join(routeErrors, "; ")
	}
}
