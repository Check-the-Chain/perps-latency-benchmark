package app

import (
	"slices"
	"strconv"
	"time"

	"perps-latency-benchmark/internal/bench"
)

type latestReadModel struct {
	UpdatedAt time.Time    `json:"updated_at"`
	Window    string       `json:"window"`
	Summaries []summaryRow `json:"summaries"`
}

type summaryRow struct {
	Venue           string  `json:"venue"`
	Transport       string  `json:"transport"`
	Scenario        string  `json:"scenario"`
	OrderType       string  `json:"order_type"`
	MeasurementMode string  `json:"measurement_mode"`
	BatchSize       int     `json:"batch_size"`
	Count           int     `json:"count"`
	OK              int     `json:"ok"`
	Failed          int     `json:"failed"`
	MeanMS          float64 `json:"mean_ms"`
	P50MS           float64 `json:"p50_ms"`
	P95MS           float64 `json:"p95_ms"`
	P99MS           float64 `json:"p99_ms"`
	RawMeanMS       float64 `json:"raw_mean_ms,omitempty"`
	RawP50MS        float64 `json:"raw_p50_ms,omitempty"`
	RawP95MS        float64 `json:"raw_p95_ms,omitempty"`
	RawP99MS        float64 `json:"raw_p99_ms,omitempty"`
	SpeedBumpMS     float64 `json:"speed_bump_ms,omitempty"`
	SpeedBumpSource string  `json:"speed_bump_source,omitempty"`
	SubmissionP50MS float64 `json:"submission_p50_ms,omitempty"`
	SubmissionP95MS float64 `json:"submission_p95_ms,omitempty"`
	SubmissionP99MS float64 `json:"submission_p99_ms,omitempty"`
	CleanupMeanMS   float64 `json:"cleanup_mean_ms,omitempty"`
	CleanupP50MS    float64 `json:"cleanup_p50_ms,omitempty"`
	CleanupP95MS    float64 `json:"cleanup_p95_ms,omitempty"`
	CleanupP99MS    float64 `json:"cleanup_p99_ms,omitempty"`
	CleanupOK       int     `json:"cleanup_ok"`
	CleanupFail     int     `json:"cleanup_failed"`
	CostCount       int     `json:"cost_count,omitempty"`
	CostMeanUSD     float64 `json:"cost_mean_usd,omitempty"`
	CostTotalUSD    float64 `json:"cost_total_usd,omitempty"`
}

func newLatestReadModel(updatedAt time.Time, window time.Duration, samples []bench.Sample) latestReadModel {
	return latestReadModel{
		UpdatedAt: updatedAt,
		Window:    window.String(),
		Summaries: summarizeGroups(samples),
	}
}

func summarizeGroups(samples []bench.Sample) []summaryRow {
	groups := make(map[string][]bench.Sample)
	for _, sample := range samples {
		key := sample.Venue + "\x00" + sample.Transport + "\x00" + string(sample.Scenario) + "\x00" + sample.OrderType + "\x00" + strconv.Itoa(sample.BatchSize) + "\x00" + string(sample.MeasurementMode)
		groups[key] = append(groups[key], sample)
	}
	rows := make([]summaryRow, 0, len(groups))
	for _, grouped := range groups {
		if len(grouped) == 0 {
			continue
		}
		summary := bench.Summarize(grouped)
		first := grouped[0]
		costCount, costMean, costTotal := summarizeCosts(grouped)
		rows = append(rows, summaryRow{
			Venue:           first.Venue,
			Transport:       first.Transport,
			Scenario:        string(first.Scenario),
			OrderType:       first.OrderType,
			MeasurementMode: string(first.MeasurementMode),
			BatchSize:       first.BatchSize,
			Count:           summary.Count,
			OK:              summary.OK,
			Failed:          summary.Failed,
			MeanMS:          summary.MeanMS,
			P50MS:           summary.P50MS,
			P95MS:           summary.P95MS,
			P99MS:           summary.P99MS,
			RawMeanMS:       summary.RawMeanMS,
			RawP50MS:        summary.RawP50MS,
			RawP95MS:        summary.RawP95MS,
			RawP99MS:        summary.RawP99MS,
			SpeedBumpMS:     summary.SpeedBumpMeanMS,
			SpeedBumpSource: summary.SpeedBumpSource,
			SubmissionP50MS: summary.SubmissionP50MS,
			SubmissionP95MS: summary.SubmissionP95MS,
			SubmissionP99MS: summary.SubmissionP99MS,
			CleanupMeanMS:   summary.CleanupMeanMS,
			CleanupP50MS:    summary.CleanupP50MS,
			CleanupP95MS:    summary.CleanupP95MS,
			CleanupP99MS:    summary.CleanupP99MS,
			CleanupOK:       summary.Cleanup.OK,
			CleanupFail:     summary.Cleanup.Failed,
			CostCount:       costCount,
			CostMeanUSD:     costMean,
			CostTotalUSD:    costTotal,
		})
	}
	slices.SortFunc(rows, func(a, b summaryRow) int {
		if a.Venue != b.Venue {
			if a.Venue < b.Venue {
				return -1
			}
			return 1
		}
		if a.Transport < b.Transport {
			return -1
		}
		if a.Transport > b.Transport {
			return 1
		}
		if a.OrderType < b.OrderType {
			return -1
		}
		if a.OrderType > b.OrderType {
			return 1
		}
		return 0
	})
	return rows
}

func summarizeCosts(samples []bench.Sample) (int, float64, float64) {
	var count int
	var total float64
	for _, sample := range samples {
		if sample.Cost == nil || !sample.Cost.Clean {
			continue
		}
		count++
		total += sample.Cost.TradeCostUSD
	}
	if count == 0 {
		return 0, 0, 0
	}
	return count, total / float64(count), total
}
