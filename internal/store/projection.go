package store

import "perps-latency-benchmark/internal/bench"

func SampleRecords(result bench.Result) []SampleRecord {
	records := make([]SampleRecord, 0, len(result.Samples))
	for _, sample := range result.Samples {
		records = append(records, SampleRecord{Sample: sample, LatencyMode: result.LatencyMode})
	}
	return records
}

func SampleRecordsByVenue(samples []bench.Sample, latencyModes map[string]bench.LatencyMode) []SampleRecord {
	records := make([]SampleRecord, 0, len(samples))
	for _, sample := range samples {
		records = append(records, SampleRecord{
			Sample:      sample,
			LatencyMode: latencyModes[sample.Venue],
		})
	}
	return records
}
