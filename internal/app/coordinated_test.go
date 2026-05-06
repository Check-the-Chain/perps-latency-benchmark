package app

import (
	"testing"

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
