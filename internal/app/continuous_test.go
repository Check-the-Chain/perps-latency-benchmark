package app

import (
	"testing"
	"time"
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
