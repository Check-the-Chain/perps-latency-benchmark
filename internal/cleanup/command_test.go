package cleanup

import (
	"testing"

	"perps-latency-benchmark/internal/bench"
)

func TestCleanupMetadataPreservesCleanupOrders(t *testing.T) {
	metadata := map[string]any{
		"cleanup":        "neutralize_position",
		"cleanup_orders": []any{map[string]any{"client_order_id": "close-1"}},
		"reconciliation": map[string]any{"position_before": "long"},
	}

	got := cleanupMetadata(metadata)
	if _, ok := got["cleanup"]; ok {
		t.Fatalf("cleanup description leaked into metadata: %#v", got)
	}
	if got["cleanup_orders"] == nil {
		t.Fatalf("cleanup_orders missing: %#v", got)
	}
	if got["reconciliation"] == nil {
		t.Fatalf("reconciliation missing: %#v", got)
	}
}

func TestRetryableCleanupResult(t *testing.T) {
	tests := []struct {
		name    string
		cleanup bench.CleanupResult
		want    bool
	}{
		{
			name:    "ok",
			cleanup: bench.CleanupResult{OK: true},
			want:    false,
		},
		{
			name:    "device time",
			cleanup: bench.CleanupResult{Error: "rejected: Your device time must match the actual time"},
			want:    true,
		},
		{
			name:    "nonce",
			cleanup: bench.CleanupResult{Error: "nonce_error: timestamp outside recvWindow"},
			want:    true,
		},
		{
			name:    "hard rejection",
			cleanup: bench.CleanupResult{Error: "rejected: insufficient margin"},
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retryableCleanupResult(tt.cleanup); got != tt.want {
				t.Fatalf("retryableCleanupResult() = %v, want %v", got, tt.want)
			}
		})
	}
}
