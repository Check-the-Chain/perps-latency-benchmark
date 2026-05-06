package bench

import (
	"testing"

	"perps-latency-benchmark/internal/lifecycle"
)

func TestSummarizeUsesAdjustedNetworkLatency(t *testing.T) {
	summary := Summarize([]Sample{
		{
			NetworkNS:         300_000_000,
			RawNetworkNS:      300_000_000,
			AdjustedNetworkNS: 100_000_000,
			SpeedBumpNS:       200_000_000,
			SpeedBumpSource:   "lighter premium taker latency tier",
			OK:                true,
			Classification:    lifecycle.Classification{Status: lifecycle.StatusAccepted},
		},
	})

	if summary.P50MS < 99.9 || summary.P50MS > 100.1 {
		t.Fatalf("adjusted p50 = %f", summary.P50MS)
	}
	if summary.RawP50MS < 299.5 || summary.RawP50MS > 300.5 {
		t.Fatalf("raw p50 = %f", summary.RawP50MS)
	}
	if summary.SpeedBumpMeanMS != 200 {
		t.Fatalf("speed bump mean = %f", summary.SpeedBumpMeanMS)
	}
}
