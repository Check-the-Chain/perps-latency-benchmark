package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/lifecycle"
)

func TestSQLiteWritesAndReadsSamples(t *testing.T) {
	db, err := OpenSQLite(filepath.Join(t.TempDir(), "bench.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC()
	err = db.WriteSamples(context.Background(), []SampleRecord{
		{
			LatencyMode: bench.LatencyModeTotal,
			Sample: bench.Sample{
				Venue:          "mock",
				RunID:          "run-test",
				Scenario:       bench.ScenarioSingle,
				Transport:      "https",
				Iteration:      0,
				BatchSize:      1,
				NetworkNS:      1_000_000,
				OK:             true,
				Classification: lifecycle.Classification{Status: lifecycle.StatusAccepted},
				Cleanup:        &bench.CleanupResult{Attempted: true, OK: true},
				CompletedAt:    now,
			},
		},
		{
			LatencyMode: bench.LatencyModeTotal,
			Sample: bench.Sample{
				Venue:       "mock",
				Warmup:      true,
				CompletedAt: now,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	samples, err := db.RecentSamples(context.Background(), now.Add(-time.Minute), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 {
		t.Fatalf("len(samples) = %d", len(samples))
	}
	if samples[0].Venue != "mock" || samples[0].NetworkNS != 1_000_000 || !samples[0].Cleanup.OK {
		t.Fatalf("sample = %+v", samples[0])
	}
}
