package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

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
				OrderType:      "market",
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
	if samples[0].Venue != "mock" || samples[0].OrderType != "market" || samples[0].NetworkNS != 1_000_000 || !samples[0].Cleanup.OK {
		t.Fatalf("sample = %+v", samples[0])
	}
}

func TestSQLiteMigratesOrderTypeColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bench.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.Exec(`
CREATE TABLE samples (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  completed_at TEXT NOT NULL,
  venue TEXT NOT NULL,
  run_id TEXT NOT NULL,
  scenario TEXT NOT NULL,
  transport TEXT NOT NULL,
  latency_mode TEXT NOT NULL,
  iteration INTEGER NOT NULL,
  batch_size INTEGER NOT NULL,
  network_ns INTEGER NOT NULL,
  ok INTEGER NOT NULL,
  classification TEXT NOT NULL,
  classification_reason TEXT NOT NULL,
  cleanup_attempted INTEGER NOT NULL,
  cleanup_ok INTEGER NOT NULL,
  error TEXT NOT NULL
)`)
	if closeErr := raw.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}

	db, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC()
	err = db.WriteSamples(context.Background(), []SampleRecord{{
		LatencyMode: bench.LatencyModeTotal,
		Sample: bench.Sample{
			Venue:          "mock",
			Scenario:       bench.ScenarioSingle,
			Transport:      "websocket",
			OrderType:      "post_only",
			BatchSize:      1,
			Classification: lifecycle.Classification{Status: lifecycle.StatusAccepted},
			CompletedAt:    now,
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
}
