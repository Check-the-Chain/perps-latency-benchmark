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

func TestLatencyForSampleCentralizesAdjustments(t *testing.T) {
	sample := Sample{
		Venue:          "extended",
		OrderType:      "market",
		NetworkNS:      300_000_000,
		RawNetworkNS:   300_000_000,
		NetworkFloorNS: 20_000_000,
		Cleanup: &CleanupResult{
			Attempted:   true,
			OK:          true,
			DurationNS:  90_000_000,
			Description: "cancel_order",
			Metadata: map[string]any{
				CleanupConfirmationMetadataKey: CleanupConfirmationAccountFeed,
			},
		},
	}

	latency := LatencyForSample(sample)
	if latency.SpeedBumpNS != ExtendedSpeedBumpNS || latency.ConfirmationNS != 150_000_000 {
		t.Fatalf("confirmation latency = %#v", latency)
	}
	if latency.NetworkAdjustedNS != 130_000_000 {
		t.Fatalf("network adjusted latency = %#v", latency)
	}
	if !latency.HasCancel || latency.CancelNS != 90_000_000 || latency.NetworkAdjustedCleanupNS != 70_000_000 {
		t.Fatalf("cancel latency = %#v", latency)
	}
}

func TestSummarizeAppliesExtendedSpeedBumpFallback(t *testing.T) {
	summary := Summarize([]Sample{
		{
			Venue:             "extended",
			OrderType:         "market",
			NetworkNS:         300_000_000,
			RawNetworkNS:      300_000_000,
			AdjustedNetworkNS: 300_000_000,
			OK:                true,
			Classification:    lifecycle.Classification{Status: lifecycle.StatusAccepted},
		},
	})

	if summary.P50MS < 149.9 || summary.P50MS > 150.1 {
		t.Fatalf("adjusted p50 = %f", summary.P50MS)
	}
	if summary.RawP50MS < 299.5 || summary.RawP50MS > 300.5 {
		t.Fatalf("raw p50 = %f", summary.RawP50MS)
	}
	if summary.SpeedBumpMeanMS != 150 {
		t.Fatalf("speed bump mean = %f", summary.SpeedBumpMeanMS)
	}
	if summary.SpeedBumpSource != ExtendedSpeedBumpSource {
		t.Fatalf("speed bump source = %q", summary.SpeedBumpSource)
	}
}

func TestSummarizeDoesNotApplyExtendedSpeedBumpToPostOnly(t *testing.T) {
	summary := Summarize([]Sample{
		{
			Venue:             "extended",
			OrderType:         "post_only",
			NetworkNS:         300_000_000,
			RawNetworkNS:      300_000_000,
			AdjustedNetworkNS: 150_000_000,
			SpeedBumpNS:       150_000_000,
			OK:                true,
			Classification:    lifecycle.Classification{Status: lifecycle.StatusAccepted},
		},
	})

	if summary.P50MS < 299.5 || summary.P50MS > 300.5 {
		t.Fatalf("post-only p50 = %f", summary.P50MS)
	}
	if summary.SpeedBumpMeanMS != 0 {
		t.Fatalf("post-only speed bump mean = %f", summary.SpeedBumpMeanMS)
	}
}

func TestSummarizeNetworkAdjustedLatency(t *testing.T) {
	summary := Summarize([]Sample{
		{
			NetworkNS:      100_000_000,
			NetworkFloorNS: 2_000_000,
			OK:             true,
			Classification: lifecycle.Classification{Status: lifecycle.StatusAccepted},
		},
		{
			NetworkNS:      140_000_000,
			NetworkFloorNS: 4_000_000,
			OK:             true,
			Classification: lifecycle.Classification{Status: lifecycle.StatusAccepted},
		},
	})

	if summary.NetworkFloorP50MS < 1.9 || summary.NetworkFloorP50MS > 2.1 {
		t.Fatalf("network floor p50 = %f", summary.NetworkFloorP50MS)
	}
	if summary.NetworkAdjustedP50MS < 97.9 || summary.NetworkAdjustedP50MS > 98.1 {
		t.Fatalf("network adjusted p50 = %f", summary.NetworkAdjustedP50MS)
	}
}

func TestSummarizeCleanupLatency(t *testing.T) {
	summary := Summarize([]Sample{
		{
			NetworkNS:      100_000_000,
			OK:             true,
			Classification: lifecycle.Classification{Status: lifecycle.StatusAccepted},
			Cleanup:        accountFeedCancel(2_000_000),
		},
		{
			NetworkNS:      120_000_000,
			OK:             true,
			Classification: lifecycle.Classification{Status: lifecycle.StatusAccepted},
			Cleanup:        accountFeedCancel(4_000_000),
		},
		{
			NetworkNS:      140_000_000,
			OK:             true,
			Classification: lifecycle.Classification{Status: lifecycle.StatusAccepted},
			Cleanup:        &CleanupResult{Attempted: true, OK: false, DurationNS: 1_000_000, Description: "cancel_order"},
		},
	})

	if summary.Cleanup.OK != 2 || summary.Cleanup.Failed != 1 {
		t.Fatalf("cleanup summary = %+v", summary.Cleanup)
	}
	if summary.CleanupMeanMS != 3 {
		t.Fatalf("cleanup mean = %f", summary.CleanupMeanMS)
	}
	if summary.CleanupP50MS < 1.9 || summary.CleanupP50MS > 2.1 {
		t.Fatalf("cleanup p50 = %f", summary.CleanupP50MS)
	}
	if summary.CleanupP95MS < 3.9 || summary.CleanupP95MS > 4.1 {
		t.Fatalf("cleanup p95 = %f", summary.CleanupP95MS)
	}
}

func TestCancelLatencyNSExcludesNeutralizeCleanup(t *testing.T) {
	_, ok := CancelLatencyNS(Sample{
		Cleanup: &CleanupResult{
			Attempted:   true,
			OK:          true,
			DurationNS:  3_000_000,
			Description: "neutralize_position",
		},
	})
	if ok {
		t.Fatal("neutralize cleanup counted as cancel latency")
	}
}

func TestCancelLatencyNSRequiresAccountFeedConfirmation(t *testing.T) {
	_, ok := CancelLatencyNS(Sample{
		Cleanup: &CleanupResult{
			Attempted:   true,
			OK:          true,
			DurationNS:  3_000_000,
			Description: "cancel_order",
		},
	})
	if ok {
		t.Fatal("cancel ack counted as account-feed cancel latency")
	}
}

func accountFeedCancel(durationNS int64) *CleanupResult {
	return &CleanupResult{
		Attempted:   true,
		OK:          true,
		DurationNS:  durationNS,
		Description: "cancel_order",
		Metadata: map[string]any{
			CleanupConfirmationMetadataKey: CleanupConfirmationAccountFeed,
		},
	}
}
