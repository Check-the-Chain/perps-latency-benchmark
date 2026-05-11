package store

import (
	"context"
	"database/sql"
	"time"

	"perps-latency-benchmark/internal/bench"
)

func (s *SQLite) RecentSummarySamples(ctx context.Context, since time.Time, limit int) ([]bench.Sample, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT s.completed_at, s.venue, s.run_id, s.scenario, s.transport, s.order_type, s.measurement_mode, s.batch_size,
       s.network_ns, s.raw_network_ns, s.adjusted_network_ns, s.network_floor_ns, s.speed_bump_ns, s.speed_bump_source, s.submission_ns, s.ok,
       s.cleanup_attempted, s.cleanup_ok, s.cleanup_duration_ns, s.cleanup_description, s.cleanup_confirmation,
       CASE WHEN s.cleanup_confirmation = '' THEN s.cleanup_metadata_json ELSE '' END,
       s.batch_submission,
       CASE WHEN s.batch_submission = '' AND s.scenario = 'batch' THEN s.metadata_json ELSE '' END,
       c.trade_cost_usd, c.clean
FROM samples s
LEFT JOIN sample_costs c
  ON c.completed_at = s.completed_at
 AND c.venue = s.venue
 AND c.run_id = s.run_id
WHERE s.completed_at >= ?
  AND s.transport != ''
  AND s.order_type != ''
  AND s.measurement_mode != ''
ORDER BY s.completed_at DESC
LIMIT ?
`, since.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var samples []bench.Sample
	for rows.Next() {
		var row summarySampleRow
		if err := rows.Scan(row.scanDestinations()...); err != nil {
			return nil, err
		}
		sample, err := row.toSample()
		if err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return samples, nil
}

type summarySampleRow struct {
	completedAt         string
	venue               string
	runID               string
	scenario            string
	transport           string
	orderType           string
	measurementMode     string
	batchSize           int
	networkNS           int64
	rawNetworkNS        int64
	adjustedNetworkNS   int64
	networkFloorNS      int64
	speedBumpNS         int64
	speedBumpSource     string
	submissionNS        int64
	ok                  int
	cleanupAttempted    int
	cleanupOK           int
	cleanupDurationNS   int64
	cleanupDescription  string
	cleanupConfirmation string
	cleanupMetadataJSON string
	batchSubmission     string
	metadataJSON        string
	costTradeUSD        sql.NullFloat64
	costClean           sql.NullInt64
}

func (r *summarySampleRow) scanDestinations() []any {
	return []any{
		&r.completedAt,
		&r.venue,
		&r.runID,
		&r.scenario,
		&r.transport,
		&r.orderType,
		&r.measurementMode,
		&r.batchSize,
		&r.networkNS,
		&r.rawNetworkNS,
		&r.adjustedNetworkNS,
		&r.networkFloorNS,
		&r.speedBumpNS,
		&r.speedBumpSource,
		&r.submissionNS,
		&r.ok,
		&r.cleanupAttempted,
		&r.cleanupOK,
		&r.cleanupDurationNS,
		&r.cleanupDescription,
		&r.cleanupConfirmation,
		&r.cleanupMetadataJSON,
		&r.batchSubmission,
		&r.metadataJSON,
		&r.costTradeUSD,
		&r.costClean,
	}
}

func (r summarySampleRow) toSample() (bench.Sample, error) {
	completedAt, err := time.Parse(time.RFC3339Nano, r.completedAt)
	if err != nil {
		return bench.Sample{}, err
	}
	batchSubmission := r.batchSubmission
	if batchSubmission == "" && r.scenario == string(bench.ScenarioBatch) {
		batchSubmission = legacyBatchSubmission(r.metadataJSON)
	}
	sample := bench.Sample{
		CompletedAt:       completedAt,
		Venue:             r.venue,
		RunID:             r.runID,
		Scenario:          bench.Scenario(r.scenario),
		Transport:         r.transport,
		OrderType:         r.orderType,
		MeasurementMode:   bench.MeasurementMode(r.measurementMode),
		BatchSize:         r.batchSize,
		NetworkNS:         r.networkNS,
		RawNetworkNS:      r.rawNetworkNS,
		AdjustedNetworkNS: r.adjustedNetworkNS,
		NetworkFloorNS:    r.networkFloorNS,
		SpeedBumpNS:       r.speedBumpNS,
		SpeedBumpSource:   r.speedBumpSource,
		SubmissionNS:      r.submissionNS,
		OK:                r.ok == 1,
	}
	if batchSubmission != "" {
		sample.Metadata = batchSubmissionMetadata(batchSubmission)
	}
	if r.cleanupAttempted == 1 || r.cleanupDurationNS > 0 || r.cleanupDescription != "" {
		confirmation := r.cleanupConfirmation
		if confirmation == "" {
			confirmation = legacyCleanupConfirmation(r.cleanupMetadataJSON)
		}
		sample.Cleanup = &bench.CleanupResult{
			Attempted:   r.cleanupAttempted == 1,
			OK:          r.cleanupOK == 1,
			DurationNS:  r.cleanupDurationNS,
			Description: r.cleanupDescription,
			Metadata:    cleanupConfirmationMetadata(confirmation),
		}
	}
	if r.costTradeUSD.Valid {
		sample.Cost = &bench.SampleCost{
			Venue:        r.venue,
			RunID:        r.runID,
			CompletedAt:  completedAt,
			TradeCostUSD: r.costTradeUSD.Float64,
			Clean:        r.costClean.Valid && r.costClean.Int64 == 1,
		}
	}
	return sample, nil
}

func batchSubmissionMetadata(value string) map[string]any {
	switch value {
	case "manual":
		return map[string]any{"native_batch_endpoint": false}
	case "native":
		return map[string]any{"native_batch_endpoint": true}
	default:
		return nil
	}
}

func cleanupConfirmationMetadata(value string) map[string]any {
	if value == "" {
		return nil
	}
	return map[string]any{bench.CleanupConfirmationMetadataKey: value}
}
