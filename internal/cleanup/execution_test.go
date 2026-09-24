package cleanup

import (
	"context"
	"testing"
	"time"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/payload"
)

func TestResultFromNetworkWithConfirmationUsesAccountFeedLatency(t *testing.T) {
	start := time.Now().Add(-10 * time.Millisecond)
	ack := netlatency.Result{
		StatusCode: 200,
		BytesRead:  17,
		Body:       []byte(`{"ok":true}`),
		Trace: netlatency.Trace{
			StartedAt: start,
			TotalNS:   int64(time.Millisecond),
			Transport: "http",
		},
	}
	confirmed := netlatency.Result{
		BytesRead: 3,
		Trace: netlatency.Trace{
			StartedAt: start,
			TotalNS:   int64(5 * time.Millisecond),
			Transport: "websocket",
		},
	}

	cleanup := cleanupExecution{}.resultFromNetworkWithConfirmation(
		context.Background(),
		ack,
		nil,
		123,
		payload.Built{Metadata: map[string]any{
			"cleanup":              "cancel benchmark orders",
			"cancel_confirmation":  map[string]any{"auth_token": "secret"},
			"non_sensitive_marker": "kept",
		}},
		&bench.Confirmation{
			Wait: func(context.Context, netlatency.Result) (netlatency.Result, error) {
				return confirmed, nil
			},
		},
		time.Second,
	)

	if !cleanup.OK {
		t.Fatalf("cleanup.OK = false, error = %q", cleanup.Error)
	}
	if cleanup.DurationNS != int64(5*time.Millisecond) || cleanup.BytesRead != 3 || cleanup.Trace.Transport != "websocket" {
		t.Fatalf("cleanup confirmation timing = duration %d bytes %d transport %q", cleanup.DurationNS, cleanup.BytesRead, cleanup.Trace.Transport)
	}
	if cleanup.Metadata["cleanup_confirmation"] != "account_feed" || cleanup.Metadata["cleanup_confirmation_transport"] != "websocket" {
		t.Fatalf("confirmation metadata = %#v", cleanup.Metadata)
	}
	if _, ok := cleanup.Metadata["cancel_confirmation"]; ok || cleanup.Metadata["non_sensitive_marker"] != "kept" {
		t.Fatalf("sanitized metadata = %#v", cleanup.Metadata)
	}
	if cleanup.Metadata["cancel_ack_duration_ns"] != int64(time.Millisecond) || cleanup.Metadata["cancel_ack_status_code"] != 200 || cleanup.Metadata["cancel_ack_bytes_read"] != int64(17) || cleanup.Metadata["cancel_ack_transport"] != "http" {
		t.Fatalf("ack metadata = %#v", cleanup.Metadata)
	}
}
