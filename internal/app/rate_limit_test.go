package app

import (
	"testing"

	"perps-latency-benchmark/internal/venues/spec"
)

func TestRateLimitRemaining(t *testing.T) {
	status := spec.RateLimitStatus{
		RequestsUsed:    13419,
		RequestsCap:     13340,
		RequestsSurplus: 500,
	}
	if got := rateLimitRemaining(status); got != 421 {
		t.Fatalf("rateLimitRemaining() = %d, want 421", got)
	}
}
