package app

import (
	"strings"
	"testing"
	"time"

	"perps-latency-benchmark/internal/bench"
)

func TestContinuousChunkSpan(t *testing.T) {
	tests := []struct {
		name    string
		rate    float64
		samples int
		want    time.Duration
	}{
		{name: "one per minute", rate: 1.0 / 60.0, samples: 1, want: time.Minute},
		{name: "multiple samples", rate: 2, samples: 3, want: 1500 * time.Millisecond},
		{name: "disabled", rate: 0, samples: 1, want: 0},
		{name: "empty", rate: 1, samples: 0, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := continuousChunkSpan(tt.rate, tt.samples); got != tt.want {
				t.Fatalf("continuousChunkSpan() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestStrictRunCleanupErrorBlocksOnStartupGuard(t *testing.T) {
	err := strictRunCleanupError("lighter", bench.CleanupConfig{
		Enabled: true,
		Mode:    bench.CleanupModeStrict,
		Scope:   bench.CleanupScopeAfterSample,
	}, bench.Result{
		StartupCleanup: &bench.CleanupResult{
			OK:    false,
			Error: "lighter account has preexisting position",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "preexisting position") {
		t.Fatalf("strictRunCleanupError = %v", err)
	}
}

func TestStrictRunCleanupErrorRestartsAfterStartupSelfHeal(t *testing.T) {
	err := strictRunCleanupError("hyperliquid", bench.CleanupConfig{
		Enabled: true,
		Mode:    bench.CleanupModeStrict,
		Scope:   bench.CleanupScopeAfterSample,
	}, bench.Result{
		StartupCleanup: &bench.CleanupResult{
			OK: true,
			Metadata: map[string]any{
				bench.CleanupSelfHealPositionMetadataKey:         true,
				bench.CleanupRestartBeforeMeasurementMetadataKey: true,
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "restarting before measuring") {
		t.Fatalf("strictRunCleanupError = %v", err)
	}
}

func TestStrictRunCleanupErrorAllowsBestEffortFailure(t *testing.T) {
	err := strictRunCleanupError("lighter", bench.CleanupConfig{
		Enabled: true,
		Mode:    bench.CleanupModeBestEffort,
		Scope:   bench.CleanupScopeAfterSample,
	}, bench.Result{
		Reconciliation: &bench.CleanupResult{
			OK:    false,
			Error: "position changed",
		},
	})
	if err != nil {
		t.Fatalf("strictRunCleanupError = %v", err)
	}
}

func TestContinuousSupervisorBackoffCapsAndResets(t *testing.T) {
	supervisor := continuousSupervisor{
		baseDelay: time.Second,
		maxDelay:  3 * time.Second,
	}
	for _, want := range []time.Duration{time.Second, 2 * time.Second, 3 * time.Second, 3 * time.Second} {
		if got := supervisor.NextDelay(); got != want {
			t.Fatalf("NextDelay() = %s, want %s", got, want)
		}
	}
	supervisor.Reset()
	if got := supervisor.NextDelay(); got != time.Second {
		t.Fatalf("NextDelay() after reset = %s, want 1s", got)
	}
}
