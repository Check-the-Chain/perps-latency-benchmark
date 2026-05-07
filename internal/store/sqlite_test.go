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
	scheduledAt := now.Add(-2 * time.Second)
	sentAt := scheduledAt.Add(300 * time.Microsecond)
	cleanupScheduledAt := now.Add(100 * time.Millisecond)
	cleanupSentAt := cleanupScheduledAt.Add(250 * time.Microsecond)
	err = db.WriteSamples(context.Background(), []SampleRecord{
		{
			LatencyMode: bench.LatencyModeTotal,
			Sample: bench.Sample{
				Venue:             "mock",
				RunID:             "run-test",
				Scenario:          bench.ScenarioSingle,
				Transport:         "https",
				OrderType:         "market",
				MeasurementMode:   bench.MeasurementModeWSConfirmation,
				Iteration:         0,
				BatchSize:         1,
				NetworkNS:         1_000_000,
				RawNetworkNS:      1_000_000,
				AdjustedNetworkNS: 800_000,
				SpeedBumpNS:       200_000,
				SpeedBumpSource:   "test speed bump",
				OK:                true,
				Classification:    lifecycle.Classification{Status: lifecycle.StatusAccepted},
				Cleanup: &bench.CleanupResult{
					Attempted:    true,
					OK:           true,
					StatusCode:   200,
					DurationNS:   2_500_000,
					PreparedNS:   700_000,
					ScheduledAt:  cleanupScheduledAt,
					SentAt:       cleanupSentAt,
					StartDelayNS: cleanupSentAt.Sub(cleanupScheduledAt).Nanoseconds(),
					WriteDelayNS: 120_000,
					BytesRead:    64,
					Description:  "cancel mock benchmark orders",
					Metadata: map[string]any{
						"cleanup_confirmation":   "account_feed",
						"cancel_ack_duration_ns": float64(1_200_000),
					},
				},
				OrderRefs: []bench.OrderRef{
					{Venue: "lighter", Market: "1", ClientOrderIndex: "123"},
				},
				CloseoutOrderRefs: []bench.OrderRef{
					{Venue: "lighter", Market: "1", ClientOrderIndex: "124"},
				},
				ExpectedEntryFill: &bench.ExpectedFill{
					Phase:          "entry",
					Side:           "buy",
					Size:           0.001,
					ExpectedPrice:  100000,
					TopAsk:         100000,
					TopAskSize:     1,
					TopAvailable:   1,
					TopSufficient:  true,
					BookAgeNS:      1_000,
					BookReceivedAt: now.Add(-time.Millisecond),
				},
				ExpectedExitFill: &bench.ExpectedFill{
					Phase:          "exit",
					Side:           "sell",
					Size:           0.001,
					ExpectedPrice:  99999,
					TopBid:         99999,
					TopBidSize:     1,
					TopAvailable:   1,
					TopSufficient:  true,
					BookAgeNS:      2_000,
					BookReceivedAt: now,
				},
				Metadata:     map[string]any{"builder": "test"},
				ScheduledAt:  scheduledAt,
				SentAt:       sentAt,
				StartDelayNS: sentAt.Sub(scheduledAt).Nanoseconds(),
				WriteDelayNS: 450_000,
				CompletedAt:  now,
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
		{
			LatencyMode: bench.LatencyModeTotal,
			Sample: bench.Sample{
				Venue:       "mock",
				RunID:       "prepare-failure",
				Scenario:    bench.ScenarioSingle,
				BatchSize:   1,
				NetworkNS:   0,
				OK:          false,
				Error:       "prepare failed before transport was known",
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
	if samples[0].Venue != "mock" || samples[0].OrderType != "market" || samples[0].NetworkNS != 1_000_000 || samples[0].AdjustedNetworkNS != 800_000 || samples[0].SpeedBumpNS != 200_000 || !samples[0].Cleanup.OK {
		t.Fatalf("sample = %+v", samples[0])
	}
	if !samples[0].ScheduledAt.Equal(scheduledAt) || !samples[0].SentAt.Equal(sentAt) || samples[0].StartDelayNS != 300_000 || samples[0].WriteDelayNS != 450_000 {
		t.Fatalf("timing fields = scheduled %s sent %s start %d write %d", samples[0].ScheduledAt, samples[0].SentAt, samples[0].StartDelayNS, samples[0].WriteDelayNS)
	}
	if samples[0].Cleanup.DurationNS != 2_500_000 || samples[0].Cleanup.StatusCode != 200 || samples[0].Cleanup.PreparedNS != 700_000 || samples[0].Cleanup.WriteDelayNS != 120_000 || samples[0].Cleanup.BytesRead != 64 || samples[0].Cleanup.Description != "cancel mock benchmark orders" {
		t.Fatalf("cleanup fields = %+v", samples[0].Cleanup)
	}
	if !samples[0].Cleanup.ScheduledAt.Equal(cleanupScheduledAt) || !samples[0].Cleanup.SentAt.Equal(cleanupSentAt) || samples[0].Cleanup.StartDelayNS != 250_000 {
		t.Fatalf("cleanup timing = scheduled %s sent %s start %d", samples[0].Cleanup.ScheduledAt, samples[0].Cleanup.SentAt, samples[0].Cleanup.StartDelayNS)
	}
	if samples[0].Cleanup.Metadata["cleanup_confirmation"] != "account_feed" || samples[0].Cleanup.Metadata["cancel_ack_duration_ns"] != float64(1_200_000) {
		t.Fatalf("cleanup metadata = %#v", samples[0].Cleanup.Metadata)
	}
	if len(samples[0].OrderRefs) != 1 || samples[0].OrderRefs[0].ClientOrderIndex != "123" {
		t.Fatalf("order refs = %#v", samples[0].OrderRefs)
	}
	if len(samples[0].CloseoutOrderRefs) != 1 || samples[0].CloseoutOrderRefs[0].ClientOrderIndex != "124" {
		t.Fatalf("closeout order refs = %#v", samples[0].CloseoutOrderRefs)
	}
	if samples[0].ExpectedEntryFill == nil || samples[0].ExpectedEntryFill.ExpectedPrice != 100000 {
		t.Fatalf("expected entry fill = %#v", samples[0].ExpectedEntryFill)
	}
	if samples[0].ExpectedExitFill == nil || samples[0].ExpectedExitFill.ExpectedPrice != 99999 {
		t.Fatalf("expected exit fill = %#v", samples[0].ExpectedExitFill)
	}
	if samples[0].Metadata["builder"] != "test" {
		t.Fatalf("metadata = %#v", samples[0].Metadata)
	}

	cost := bench.SampleCost{
		Venue:                 "mock",
		RunID:                 "run-test",
		CompletedAt:           now,
		EntryOrderID:          "entry",
		ExitOrderID:           "exit",
		EntryValueUSD:         100,
		ExitValueUSD:          99,
		EntryFeeUSD:           0.01,
		ExitFeeUSD:            0.01,
		PriceMoveCostUSD:      1,
		TradeCostUSD:          1.02,
		BalanceBeforeUSD:      10,
		BalanceAfterUSD:       8.98,
		BalanceDeltaUSD:       -1.02,
		ReconciliationDiffUSD: 0,
		Clean:                 true,
		BalanceBefore:         bench.BalanceSnapshot{Venue: "mock", BalanceUSD: 10, CapturedAt: now},
		BalanceAfter:          bench.BalanceSnapshot{Venue: "mock", BalanceUSD: 8.98, CapturedAt: now},
	}
	if err := db.WriteSampleCosts(context.Background(), []bench.SampleCost{cost}); err != nil {
		t.Fatal(err)
	}
	samples, err = db.RecentSamples(context.Background(), now.Add(-time.Minute), 10)
	if err != nil {
		t.Fatal(err)
	}
	if samples[0].Cost == nil || samples[0].Cost.TradeCostUSD != 1.02 || !samples[0].Cost.Clean {
		t.Fatalf("cost = %+v", samples[0].Cost)
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

func TestSQLiteDSNIncludesConnectionPragmas(t *testing.T) {
	dsn := sqliteDSN(filepath.Join(t.TempDir(), "bench.db"))
	if dsn == "" {
		t.Fatal("empty dsn")
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var timeout int
	if err := db.QueryRow(`PRAGMA busy_timeout`).Scan(&timeout); err != nil {
		t.Fatal(err)
	}
	if timeout != 10000 {
		t.Fatalf("busy_timeout = %d, want 10000", timeout)
	}
}

func TestSQLiteOpensRelativePath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	db, err := OpenSQLite(filepath.Join("data", "bench.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}
