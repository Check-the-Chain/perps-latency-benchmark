package bench

import (
	"fmt"
	"strings"

	hdrhistogram "github.com/HdrHistogram/hdrhistogram-go"
)

type Summary struct {
	Count   int            `json:"count"`
	OK      int            `json:"ok"`
	Failed  int            `json:"failed"`
	Cleanup CleanupSummary `json:"cleanup"`
	MinMS   float64        `json:"min_ms"`
	MeanMS  float64        `json:"mean_ms"`
	P50MS   float64        `json:"p50_ms"`
	P95MS   float64        `json:"p95_ms"`
	P99MS   float64        `json:"p99_ms"`
	MaxMS   float64        `json:"max_ms"`
}

type CleanupSummary struct {
	Attempted int `json:"attempted"`
	OK        int `json:"ok"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}

func Summarize(samples []Sample) Summary {
	hist := hdrhistogram.New(1, 10*60*1_000_000_000, 3)
	var totalNS int64
	var ok int
	var failed int
	var cleanup CleanupSummary

	for _, sample := range samples {
		if sample.Warmup {
			continue
		}
		if sample.Cleanup != nil {
			if sample.Cleanup.Attempted {
				cleanup.Attempted++
				if sample.Cleanup.OK {
					cleanup.OK++
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
		value := sample.NetworkNS
		if value < 1 {
			value = 1
		}
		_ = hist.RecordValue(value)
		totalNS += value
		ok++
	}

	summary := Summary{Count: ok + failed, OK: ok, Failed: failed, Cleanup: cleanup}
	if ok == 0 {
		return summary
	}

	summary.MinMS = nsToMS(hist.Min())
	summary.MeanMS = nsToMS(totalNS / int64(ok))
	summary.P50MS = nsToMS(hist.ValueAtQuantile(50))
	summary.P95MS = nsToMS(hist.ValueAtQuantile(95))
	summary.P99MS = nsToMS(hist.ValueAtQuantile(99))
	summary.MaxMS = nsToMS(hist.Max())
	return summary
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
			"cleanup attempted=%d ok=%d failed=%d skipped=%d",
			summary.Cleanup.Attempted,
			summary.Cleanup.OK,
			summary.Cleanup.Failed,
			summary.Cleanup.Skipped,
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
