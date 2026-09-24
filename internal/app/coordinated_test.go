package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"perps-latency-benchmark/internal/bench"
)

func TestRetryableCost(t *testing.T) {
	tests := []struct {
		name string
		cost bench.SampleCost
		want bool
	}{
		{
			name: "clean",
			cost: bench.SampleCost{Clean: true},
			want: false,
		},
		{
			name: "missing fills",
			cost: bench.SampleCost{QualityReason: "missing entry or exit fill"},
			want: true,
		},
		{
			name: "incomplete fills",
			cost: bench.SampleCost{QualityReason: "incomplete entry or exit fill"},
			want: true,
		},
		{
			name: "balance lag",
			cost: bench.SampleCost{QualityReason: "balance reconciliation differs from fill cost"},
			want: true,
		},
		{
			name: "preexisting position",
			cost: bench.SampleCost{QualityReason: "account has an existing Lighter position; balance audit cannot isolate benchmark cost"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retryableCost(tt.cost); got != tt.want {
				t.Fatalf("retryableCost() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetainCoordinatedSamplesBoundsMemory(t *testing.T) {
	current := []bench.Sample{{Iteration: 1}, {Iteration: 2}}
	next := []bench.Sample{{Iteration: 3}, {Iteration: 4}}

	got := retainCoordinatedSamples(current, next, 3)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for index, want := range []int{2, 3, 4} {
		if got[index].Iteration != want {
			t.Fatalf("sample[%d].Iteration = %d, want %d", index, got[index].Iteration, want)
		}
	}
}

func TestCoordinatedSampleRetentionLimitUsesStoreWindowForContinuousRuns(t *testing.T) {
	limit := coordinatedSampleRetentionLimit(&coordinatedOptions{
		interval:     10 * time.Minute,
		retainHours:  2,
		warmupCycles: 1,
	}, 5)

	if limit != 70 {
		t.Fatalf("limit = %d, want 70", limit)
	}
}

func TestCoordinatedStrictStartupCleanupErrorRestartsAfterSelfHeal(t *testing.T) {
	err := coordinatedStrictStartupCleanupError(
		map[string]*bench.CleanupResult{
			"hyperliquid": {
				OK: true,
				Metadata: map[string]any{
					bench.CleanupSelfHealPositionMetadataKey:         true,
					bench.CleanupRestartBeforeMeasurementMetadataKey: true,
				},
			},
		},
		[]bench.CoordinatedItem{{
			Config: bench.Config{
				Cleanup: bench.CleanupConfig{Enabled: true, Mode: bench.CleanupModeStrict, Scope: bench.CleanupScopeAfterSample},
			},
			Venue: namedVenue{name: "hyperliquid"},
		}},
		"pre-cycle",
	)

	if err == nil || !strings.Contains(err.Error(), "pre-cycle cleanup repaired preexisting position") {
		t.Fatalf("coordinatedStrictStartupCleanupError = %v", err)
	}
}

type namedVenue struct {
	name string
}

func (v namedVenue) Name() string {
	return v.name
}

func (v namedVenue) Prepare(_ context.Context, _ bench.Scenario, _ int, _ int) (bench.PreparedRequest, error) {
	return bench.PreparedRequest{}, nil
}

func (v namedVenue) Close(context.Context) error {
	return nil
}
