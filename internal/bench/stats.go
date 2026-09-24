package bench

import (
	"fmt"
	"strings"
)

type Summary struct {
	Count                        int            `json:"count"`
	OK                           int            `json:"ok"`
	Failed                       int            `json:"failed"`
	Cleanup                      CleanupSummary `json:"cleanup"`
	MinMS                        float64        `json:"min_ms"`
	MeanMS                       float64        `json:"mean_ms"`
	P50MS                        float64        `json:"p50_ms"`
	P95MS                        float64        `json:"p95_ms"`
	P99MS                        float64        `json:"p99_ms"`
	P999MS                       float64        `json:"p999_ms"`
	MaxMS                        float64        `json:"max_ms"`
	RawMeanMS                    float64        `json:"raw_mean_ms,omitempty"`
	RawP50MS                     float64        `json:"raw_p50_ms,omitempty"`
	RawP95MS                     float64        `json:"raw_p95_ms,omitempty"`
	RawP99MS                     float64        `json:"raw_p99_ms,omitempty"`
	RawP999MS                    float64        `json:"raw_p999_ms,omitempty"`
	NetworkFloorMeanMS           float64        `json:"network_floor_mean_ms,omitempty"`
	NetworkFloorP50MS            float64        `json:"network_floor_p50_ms,omitempty"`
	NetworkFloorP95MS            float64        `json:"network_floor_p95_ms,omitempty"`
	NetworkFloorP99MS            float64        `json:"network_floor_p99_ms,omitempty"`
	NetworkFloorP999MS           float64        `json:"network_floor_p999_ms,omitempty"`
	NetworkAdjustedMeanMS        float64        `json:"network_adjusted_mean_ms,omitempty"`
	NetworkAdjustedP50MS         float64        `json:"network_adjusted_p50_ms,omitempty"`
	NetworkAdjustedP95MS         float64        `json:"network_adjusted_p95_ms,omitempty"`
	NetworkAdjustedP99MS         float64        `json:"network_adjusted_p99_ms,omitempty"`
	NetworkAdjustedP999MS        float64        `json:"network_adjusted_p999_ms,omitempty"`
	SpeedBumpMeanMS              float64        `json:"speed_bump_mean_ms,omitempty"`
	SpeedBumpSource              string         `json:"speed_bump_source,omitempty"`
	SubmissionP50MS              float64        `json:"submission_p50_ms,omitempty"`
	SubmissionP95MS              float64        `json:"submission_p95_ms,omitempty"`
	SubmissionP99MS              float64        `json:"submission_p99_ms,omitempty"`
	SubmissionP999MS             float64        `json:"submission_p999_ms,omitempty"`
	CleanupMeanMS                float64        `json:"cleanup_mean_ms,omitempty"`
	CleanupP50MS                 float64        `json:"cleanup_p50_ms,omitempty"`
	CleanupP95MS                 float64        `json:"cleanup_p95_ms,omitempty"`
	CleanupP99MS                 float64        `json:"cleanup_p99_ms,omitempty"`
	CleanupP999MS                float64        `json:"cleanup_p999_ms,omitempty"`
	NetworkAdjustedCleanupMeanMS float64        `json:"network_adjusted_cleanup_mean_ms,omitempty"`
	NetworkAdjustedCleanupP50MS  float64        `json:"network_adjusted_cleanup_p50_ms,omitempty"`
	NetworkAdjustedCleanupP95MS  float64        `json:"network_adjusted_cleanup_p95_ms,omitempty"`
	NetworkAdjustedCleanupP99MS  float64        `json:"network_adjusted_cleanup_p99_ms,omitempty"`
	NetworkAdjustedCleanupP999MS float64        `json:"network_adjusted_cleanup_p999_ms,omitempty"`
}

type CleanupSummary struct {
	Attempted int `json:"attempted"`
	OK        int `json:"ok"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}

func Summarize(samples []Sample) Summary {
	latency := newLatencyAccumulator()
	rawLatency := newLatencyAccumulator()
	submissionLatency := newLatencyAccumulator()
	cleanupLatency := newLatencyAccumulator()
	networkFloorLatency := newLatencyAccumulator()
	networkAdjustedLatency := newLatencyAccumulator()
	networkAdjustedCleanupLatency := newLatencyAccumulator()
	var speedBumpTotalNS int64
	var ok int
	var failed int
	var cleanup CleanupSummary

	for _, sample := range samples {
		if sample.Warmup {
			continue
		}
		semantics := LatencyForSample(sample)
		if sample.Cleanup != nil {
			if sample.Cleanup.Attempted {
				cleanup.Attempted++
				if sample.Cleanup.OK {
					cleanup.OK++
					if semantics.HasCancel {
						cleanupLatency.record(semantics.CancelNS)
						if semantics.HasNetworkFloor {
							networkAdjustedCleanupLatency.record(semantics.NetworkAdjustedCleanupNS)
						}
					}
				} else {
					cleanup.Failed++
				}
			} else {
				cleanup.Skipped++
			}
		}
		if !sample.OK {
			failed++
			continue
		}
		latency.record(semantics.ConfirmationNS)
		rawLatency.record(semantics.RawNetworkNS)
		if semantics.HasNetworkFloor {
			networkFloorLatency.record(semantics.NetworkFloorNS)
			networkAdjustedLatency.record(semantics.NetworkAdjustedNS)
		}
		if sample.SubmissionNS > 0 {
			submissionLatency.record(sample.SubmissionNS)
		}
		speedBumpTotalNS += semantics.SpeedBumpNS
		ok++
	}

	summary := Summary{Count: ok + failed, OK: ok, Failed: failed, Cleanup: cleanup}
	if cleanupLatency.recorded() {
		summary.CleanupMeanMS = cleanupLatency.meanMS()
		summary.CleanupP50MS = cleanupLatency.p50MS()
		summary.CleanupP95MS = cleanupLatency.p95MS()
		summary.CleanupP99MS = cleanupLatency.p99MS()
		summary.CleanupP999MS = cleanupLatency.p999MS()
	}
	if networkAdjustedCleanupLatency.recorded() {
		summary.NetworkAdjustedCleanupMeanMS = networkAdjustedCleanupLatency.meanMS()
		summary.NetworkAdjustedCleanupP50MS = networkAdjustedCleanupLatency.p50MS()
		summary.NetworkAdjustedCleanupP95MS = networkAdjustedCleanupLatency.p95MS()
		summary.NetworkAdjustedCleanupP99MS = networkAdjustedCleanupLatency.p99MS()
		summary.NetworkAdjustedCleanupP999MS = networkAdjustedCleanupLatency.p999MS()
	}
	if ok == 0 {
		return summary
	}

	summary.MinMS = latency.minMS()
	summary.MeanMS = latency.meanMS()
	summary.P50MS = latency.p50MS()
	summary.P95MS = latency.p95MS()
	summary.P99MS = latency.p99MS()
	summary.P999MS = latency.p999MS()
	summary.MaxMS = latency.maxMS()
	summary.RawMeanMS = rawLatency.meanMS()
	summary.RawP50MS = rawLatency.p50MS()
	summary.RawP95MS = rawLatency.p95MS()
	summary.RawP99MS = rawLatency.p99MS()
	summary.RawP999MS = rawLatency.p999MS()
	if networkFloorLatency.recorded() {
		summary.NetworkFloorMeanMS = networkFloorLatency.meanMS()
		summary.NetworkFloorP50MS = networkFloorLatency.p50MS()
		summary.NetworkFloorP95MS = networkFloorLatency.p95MS()
		summary.NetworkFloorP99MS = networkFloorLatency.p99MS()
		summary.NetworkFloorP999MS = networkFloorLatency.p999MS()
	}
	if networkAdjustedLatency.recorded() {
		summary.NetworkAdjustedMeanMS = networkAdjustedLatency.meanMS()
		summary.NetworkAdjustedP50MS = networkAdjustedLatency.p50MS()
		summary.NetworkAdjustedP95MS = networkAdjustedLatency.p95MS()
		summary.NetworkAdjustedP99MS = networkAdjustedLatency.p99MS()
		summary.NetworkAdjustedP999MS = networkAdjustedLatency.p999MS()
	}
	summary.SpeedBumpMeanMS = nsToMS(speedBumpTotalNS / int64(ok))
	summary.SpeedBumpSource = speedBumpSource(samples)
	if submissionLatency.recorded() {
		summary.SubmissionP50MS = submissionLatency.p50MS()
		summary.SubmissionP95MS = submissionLatency.p95MS()
		summary.SubmissionP99MS = submissionLatency.p99MS()
		summary.SubmissionP999MS = submissionLatency.p999MS()
	}
	return summary
}

func speedBumpSource(samples []Sample) string {
	for _, sample := range samples {
		if sample.Warmup || !sample.OK {
			continue
		}
		if source := SpeedBumpSource(sample); source != "" {
			return source
		}
	}
	return ""
}

func FormatSummary(result Result) string {
	summary := Summarize(result.Samples)
	lines := []string{
		fmt.Sprintf(
			"venue=%s run_id=%s scenario=%s latency_mode=%s count=%d ok=%d failed=%d",
			result.Venue,
			result.RunID,
			result.Scenario,
			result.LatencyMode,
			summary.Count,
			summary.OK,
			summary.Failed,
		),
	}
	if summary.OK > 0 {
		lines = append(lines, fmt.Sprintf(
			"latency_ms min=%.3f mean=%.3f p50=%.3f p95=%.3f p99=%.3f max=%.3f",
			summary.MinMS,
			summary.MeanMS,
			summary.P50MS,
			summary.P95MS,
			summary.P99MS,
			summary.MaxMS,
		))
	}
	if summary.Cleanup.Attempted > 0 || summary.Cleanup.Skipped > 0 {
		lines = append(lines, fmt.Sprintf(
			"cleanup attempted=%d ok=%d failed=%d skipped=%d p50_ms=%.3f p95_ms=%.3f",
			summary.Cleanup.Attempted,
			summary.Cleanup.OK,
			summary.Cleanup.Failed,
			summary.Cleanup.Skipped,
			summary.CleanupP50MS,
			summary.CleanupP95MS,
		))
	}
	if result.StartupCleanup != nil {
		lines = append(lines, fmt.Sprintf("startup_cleanup attempted=%t ok=%t", result.StartupCleanup.Attempted, result.StartupCleanup.OK))
	}
	if result.Reconciliation != nil {
		lines = append(lines, fmt.Sprintf("reconciliation attempted=%t ok=%t", result.Reconciliation.Attempted, result.Reconciliation.OK))
	}
	for _, sample := range result.Samples {
		if sample.Error != "" {
			lines = append(lines, "error="+sample.Error)
			if len(lines) >= 5 {
				break
			}
		}
	}
	return strings.Join(lines, "\n")
}

func nsToMS(value int64) float64 {
	return float64(value) / 1_000_000
}
