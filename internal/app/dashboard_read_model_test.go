package app

import (
	"testing"
	"time"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/lifecycle"
)

func TestLatestReadModelGroupsAndProjectsDashboardContract(t *testing.T) {
	updatedAt := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	model := newLatestReadModel(updatedAt, 12*time.Hour, []bench.Sample{
		{
			Venue:           "hyperliquid",
			Transport:       "https",
			Scenario:        bench.ScenarioSingle,
			OrderType:       "post_only",
			MeasurementMode: bench.MeasurementModeWSConfirmation,
			BatchSize:       1,
			NetworkNS:       2_000_000,
			OK:              true,
			Classification:  lifecycle.Classification{Status: lifecycle.StatusAccepted},
			Cleanup:         &bench.CleanupResult{Attempted: true, OK: true, DurationNS: 500_000, Description: "cancel hyperliquid benchmark orders"},
			Cost:            &bench.SampleCost{Clean: true, TradeCostUSD: 0.03},
		},
		{
			Venue:           "hyperliquid",
			Transport:       "websocket",
			Scenario:        bench.ScenarioSingle,
			OrderType:       "post_only",
			MeasurementMode: bench.MeasurementModeWSConfirmation,
			BatchSize:       1,
			NetworkNS:       4_000_000,
			OK:              true,
			Classification:  lifecycle.Classification{Status: lifecycle.StatusAccepted},
			Cleanup:         &bench.CleanupResult{Attempted: true, OK: true, DurationNS: 900_000, Description: "neutralize position"},
			Cost:            &bench.SampleCost{Clean: true, TradeCostUSD: 0.05},
		},
		{
			Venue:           "aster",
			Transport:       "https",
			Scenario:        bench.ScenarioBatch,
			OrderType:       "post_only",
			MeasurementMode: bench.MeasurementModeAck,
			BatchSize:       4,
			NetworkNS:       3_000_000,
			OK:              false,
			Classification:  lifecycle.Classification{Status: lifecycle.StatusRejected},
		},
	})

	if !model.UpdatedAt.Equal(updatedAt) || model.Window != "12h0m0s" {
		t.Fatalf("model metadata = %+v", model)
	}
	if len(model.Summaries) != 2 {
		t.Fatalf("summaries = %+v", model.Summaries)
	}
	if model.Summaries[0].Venue != "aster" || model.Summaries[1].Venue != "hyperliquid" {
		t.Fatalf("summary order = %+v", model.Summaries)
	}
	hyperliquid := model.Summaries[1]
	if hyperliquid.Count != 2 || hyperliquid.OK != 2 || hyperliquid.CleanupOK != 2 {
		t.Fatalf("hyperliquid summary = %+v", hyperliquid)
	}
	if hyperliquid.Transport != "mixed" {
		t.Fatalf("hyperliquid transport = %q", hyperliquid.Transport)
	}
	if hyperliquid.CleanupMeanMS != 0.5 {
		t.Fatalf("cancel cleanup mean = %f", hyperliquid.CleanupMeanMS)
	}
	if hyperliquid.CostCount != 2 || hyperliquid.CostTotalUSD != 0.08 || hyperliquid.CostMeanUSD != 0.04 {
		t.Fatalf("cost projection = %+v", hyperliquid)
	}
}
