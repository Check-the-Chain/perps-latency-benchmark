package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/lifecycle"
)

type SQLite struct {
	db *sql.DB
}

type SampleRecord struct {
	Sample      bench.Sample
	LatencyMode bench.LatencyMode
}

func OpenSQLite(path string) (*SQLite, error) {
	if path == "" {
		return nil, errors.New("sqlite store path is required")
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &SQLite{db: db}
	if err := store.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func sqliteDSN(path string) string {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(10000)")
	q.Add("_pragma", "journal_mode(WAL)")
	return path + "?" + q.Encode()
}

func (s *SQLite) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLite) init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
PRAGMA journal_mode=WAL;
PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS samples (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  completed_at TEXT NOT NULL,
  scheduled_at TEXT NOT NULL DEFAULT '',
  sent_at TEXT NOT NULL DEFAULT '',
  venue TEXT NOT NULL,
  run_id TEXT NOT NULL,
  scenario TEXT NOT NULL,
  transport TEXT NOT NULL,
  order_type TEXT NOT NULL DEFAULT '',
  latency_mode TEXT NOT NULL,
  measurement_mode TEXT NOT NULL DEFAULT '',
  iteration INTEGER NOT NULL,
  batch_size INTEGER NOT NULL,
  network_ns INTEGER NOT NULL,
  raw_network_ns INTEGER NOT NULL DEFAULT 0,
  adjusted_network_ns INTEGER NOT NULL DEFAULT 0,
  speed_bump_ns INTEGER NOT NULL DEFAULT 0,
  speed_bump_source TEXT NOT NULL DEFAULT '',
  submission_ns INTEGER NOT NULL DEFAULT 0,
  start_delay_ns INTEGER NOT NULL DEFAULT 0,
  write_delay_ns INTEGER NOT NULL DEFAULT 0,
  ok INTEGER NOT NULL,
  classification TEXT NOT NULL,
  classification_reason TEXT NOT NULL,
  cleanup_attempted INTEGER NOT NULL,
  cleanup_ok INTEGER NOT NULL,
  cleanup_status_code INTEGER NOT NULL DEFAULT 0,
  cleanup_duration_ns INTEGER NOT NULL DEFAULT 0,
  cleanup_prepared_ns INTEGER NOT NULL DEFAULT 0,
  cleanup_scheduled_at TEXT NOT NULL DEFAULT '',
  cleanup_sent_at TEXT NOT NULL DEFAULT '',
  cleanup_start_delay_ns INTEGER NOT NULL DEFAULT 0,
  cleanup_write_delay_ns INTEGER NOT NULL DEFAULT 0,
  cleanup_bytes_read INTEGER NOT NULL DEFAULT 0,
  cleanup_error TEXT NOT NULL DEFAULT '',
  cleanup_description TEXT NOT NULL DEFAULT '',
  cleanup_metadata_json TEXT NOT NULL DEFAULT '',
  order_refs_json TEXT NOT NULL DEFAULT '',
  closeout_order_refs_json TEXT NOT NULL DEFAULT '',
  expected_entry_fill_json TEXT NOT NULL DEFAULT '',
  expected_exit_fill_json TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS samples_completed_at_idx ON samples(completed_at);
CREATE INDEX IF NOT EXISTS samples_group_idx ON samples(venue, transport, scenario, latency_mode, completed_at);
CREATE TABLE IF NOT EXISTS sample_costs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  completed_at TEXT NOT NULL,
  venue TEXT NOT NULL,
  run_id TEXT NOT NULL,
  entry_order_id TEXT NOT NULL DEFAULT '',
  exit_order_id TEXT NOT NULL DEFAULT '',
  entry_qty REAL NOT NULL DEFAULT 0,
  exit_qty REAL NOT NULL DEFAULT 0,
  entry_value_usd REAL NOT NULL DEFAULT 0,
  exit_value_usd REAL NOT NULL DEFAULT 0,
  entry_fee_usd REAL NOT NULL DEFAULT 0,
  exit_fee_usd REAL NOT NULL DEFAULT 0,
  price_move_cost_usd REAL NOT NULL DEFAULT 0,
  trade_cost_usd REAL NOT NULL DEFAULT 0,
  balance_before_usd REAL NOT NULL DEFAULT 0,
  balance_after_usd REAL NOT NULL DEFAULT 0,
  balance_delta_usd REAL NOT NULL DEFAULT 0,
  reconciliation_diff_usd REAL NOT NULL DEFAULT 0,
  clean INTEGER NOT NULL DEFAULT 0,
  quality_reason TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  balance_before_json TEXT NOT NULL DEFAULT '',
  balance_after_json TEXT NOT NULL DEFAULT '',
  UNIQUE(completed_at, venue, run_id)
);
CREATE INDEX IF NOT EXISTS sample_costs_completed_at_idx ON sample_costs(completed_at);
`)
	if err != nil {
		return err
	}
	if err := s.ensureSamplesOrderType(ctx); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "measurement_mode", `ALTER TABLE samples ADD COLUMN measurement_mode TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "submission_ns", `ALTER TABLE samples ADD COLUMN submission_ns INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "scheduled_at", `ALTER TABLE samples ADD COLUMN scheduled_at TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "sent_at", `ALTER TABLE samples ADD COLUMN sent_at TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "start_delay_ns", `ALTER TABLE samples ADD COLUMN start_delay_ns INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "raw_network_ns", `ALTER TABLE samples ADD COLUMN raw_network_ns INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "adjusted_network_ns", `ALTER TABLE samples ADD COLUMN adjusted_network_ns INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "speed_bump_ns", `ALTER TABLE samples ADD COLUMN speed_bump_ns INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "speed_bump_source", `ALTER TABLE samples ADD COLUMN speed_bump_source TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "write_delay_ns", `ALTER TABLE samples ADD COLUMN write_delay_ns INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "cleanup_prepared_ns", `ALTER TABLE samples ADD COLUMN cleanup_prepared_ns INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "cleanup_status_code", `ALTER TABLE samples ADD COLUMN cleanup_status_code INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "cleanup_duration_ns", `ALTER TABLE samples ADD COLUMN cleanup_duration_ns INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "cleanup_scheduled_at", `ALTER TABLE samples ADD COLUMN cleanup_scheduled_at TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "cleanup_sent_at", `ALTER TABLE samples ADD COLUMN cleanup_sent_at TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "cleanup_start_delay_ns", `ALTER TABLE samples ADD COLUMN cleanup_start_delay_ns INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "cleanup_write_delay_ns", `ALTER TABLE samples ADD COLUMN cleanup_write_delay_ns INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "cleanup_bytes_read", `ALTER TABLE samples ADD COLUMN cleanup_bytes_read INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "cleanup_error", `ALTER TABLE samples ADD COLUMN cleanup_error TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "cleanup_description", `ALTER TABLE samples ADD COLUMN cleanup_description TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "cleanup_metadata_json", `ALTER TABLE samples ADD COLUMN cleanup_metadata_json TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "order_refs_json", `ALTER TABLE samples ADD COLUMN order_refs_json TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "closeout_order_refs_json", `ALTER TABLE samples ADD COLUMN closeout_order_refs_json TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "expected_entry_fill_json", `ALTER TABLE samples ADD COLUMN expected_entry_fill_json TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "expected_exit_fill_json", `ALTER TABLE samples ADD COLUMN expected_exit_fill_json TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "metadata_json", `ALTER TABLE samples ADD COLUMN metadata_json TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS samples_order_type_idx ON samples(order_type, completed_at);`)
	return err
}

func (s *SQLite) WriteSamples(ctx context.Context, records []SampleRecord) error {
	if len(records) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO samples (
  completed_at, scheduled_at, sent_at, venue, run_id, scenario, transport, order_type, latency_mode, measurement_mode, iteration,
  batch_size, network_ns, raw_network_ns, adjusted_network_ns, speed_bump_ns, speed_bump_source, submission_ns, start_delay_ns, write_delay_ns, ok, classification, classification_reason,
  cleanup_attempted, cleanup_ok, cleanup_status_code, cleanup_duration_ns, cleanup_prepared_ns, cleanup_scheduled_at, cleanup_sent_at, cleanup_start_delay_ns, cleanup_write_delay_ns, cleanup_bytes_read, cleanup_error, cleanup_description,
  cleanup_metadata_json, order_refs_json, closeout_order_refs_json, expected_entry_fill_json, expected_exit_fill_json, metadata_json, error
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, record := range records {
		sample := record.Sample
		if sample.Warmup {
			continue
		}
		cleanupAttempted, cleanupOK := cleanupFields(sample.Cleanup)
		cleanupStatusCode, cleanupDurationNS, cleanupPreparedNS, cleanupScheduledAt, cleanupSentAt, cleanupStartDelayNS, cleanupWriteDelayNS, cleanupBytesRead, cleanupError, cleanupDescription := cleanupStorageFields(sample.Cleanup)
		cleanupMetadataJSON, err := cleanupMetadataJSON(sample.Cleanup)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		metadataJSON, err := metadataJSON(sample.Metadata)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		orderRefsJSON, err := marshalJSON(sample.OrderRefs)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		closeoutRefsJSON, err := marshalJSON(sample.CloseoutOrderRefs)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		expectedEntryJSON, err := marshalOptionalJSON(sample.ExpectedEntryFill)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		expectedExitJSON, err := marshalOptionalJSON(sample.ExpectedExitFill)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if _, err := stmt.ExecContext(ctx,
			sample.CompletedAt.UTC().Format(time.RFC3339Nano),
			formatOptionalTime(sample.ScheduledAt),
			formatOptionalTime(sample.SentAt),
			sample.Venue,
			sample.RunID,
			string(sample.Scenario),
			sample.Transport,
			sample.OrderType,
			string(record.LatencyMode),
			string(sample.MeasurementMode),
			sample.Iteration,
			sample.BatchSize,
			sample.NetworkNS,
			bench.RawNetworkNS(sample),
			bench.AdjustedNetworkNS(sample),
			sample.SpeedBumpNS,
			sample.SpeedBumpSource,
			sample.SubmissionNS,
			sample.StartDelayNS,
			sample.WriteDelayNS,
			boolInt(sample.OK),
			string(sample.Classification.Status),
			sample.Classification.Reason,
			cleanupAttempted,
			cleanupOK,
			cleanupStatusCode,
			cleanupDurationNS,
			cleanupPreparedNS,
			cleanupScheduledAt,
			cleanupSentAt,
			cleanupStartDelayNS,
			cleanupWriteDelayNS,
			cleanupBytesRead,
			cleanupError,
			cleanupDescription,
			cleanupMetadataJSON,
			orderRefsJSON,
			closeoutRefsJSON,
			expectedEntryJSON,
			expectedExitJSON,
			metadataJSON,
			sample.Error,
		); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLite) WriteSampleCosts(ctx context.Context, costs []bench.SampleCost) error {
	if len(costs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO sample_costs (
  completed_at, venue, run_id, entry_order_id, exit_order_id, entry_qty, exit_qty,
  entry_value_usd, exit_value_usd, entry_fee_usd, exit_fee_usd, price_move_cost_usd, trade_cost_usd,
  balance_before_usd, balance_after_usd, balance_delta_usd, reconciliation_diff_usd, clean,
  quality_reason, metadata_json, balance_before_json, balance_after_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(completed_at, venue, run_id) DO UPDATE SET
  entry_order_id=excluded.entry_order_id,
  exit_order_id=excluded.exit_order_id,
  entry_qty=excluded.entry_qty,
  exit_qty=excluded.exit_qty,
  entry_value_usd=excluded.entry_value_usd,
  exit_value_usd=excluded.exit_value_usd,
  entry_fee_usd=excluded.entry_fee_usd,
  exit_fee_usd=excluded.exit_fee_usd,
  price_move_cost_usd=excluded.price_move_cost_usd,
  trade_cost_usd=excluded.trade_cost_usd,
  balance_before_usd=excluded.balance_before_usd,
  balance_after_usd=excluded.balance_after_usd,
  balance_delta_usd=excluded.balance_delta_usd,
  reconciliation_diff_usd=excluded.reconciliation_diff_usd,
  clean=excluded.clean,
  quality_reason=excluded.quality_reason,
  metadata_json=excluded.metadata_json,
  balance_before_json=excluded.balance_before_json,
  balance_after_json=excluded.balance_after_json
`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, cost := range costs {
		metadataJSON, err := metadataJSON(cost.Metadata)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		beforeJSON, err := marshalJSON(cost.BalanceBefore)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		afterJSON, err := marshalJSON(cost.BalanceAfter)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if _, err := stmt.ExecContext(ctx,
			cost.CompletedAt.UTC().Format(time.RFC3339Nano),
			cost.Venue,
			cost.RunID,
			cost.EntryOrderID,
			cost.ExitOrderID,
			cost.EntryQty,
			cost.ExitQty,
			cost.EntryValueUSD,
			cost.ExitValueUSD,
			cost.EntryFeeUSD,
			cost.ExitFeeUSD,
			cost.PriceMoveCostUSD,
			cost.TradeCostUSD,
			cost.BalanceBeforeUSD,
			cost.BalanceAfterUSD,
			cost.BalanceDeltaUSD,
			cost.ReconciliationDiffUSD,
			boolInt(cost.Clean),
			cost.QualityReason,
			metadataJSON,
			beforeJSON,
			afterJSON,
		); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseOptionalTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func (s *SQLite) RecentSamples(ctx context.Context, since time.Time, limit int) ([]bench.Sample, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT completed_at, scheduled_at, sent_at, venue, run_id, scenario, transport, order_type, measurement_mode, iteration, batch_size,
       network_ns, raw_network_ns, adjusted_network_ns, speed_bump_ns, speed_bump_source, submission_ns, start_delay_ns, write_delay_ns, ok, classification, classification_reason,
       cleanup_attempted, cleanup_ok, cleanup_status_code, cleanup_duration_ns, cleanup_prepared_ns, cleanup_scheduled_at, cleanup_sent_at, cleanup_start_delay_ns, cleanup_write_delay_ns, cleanup_bytes_read, cleanup_error, cleanup_description,
       cleanup_metadata_json, order_refs_json, closeout_order_refs_json, expected_entry_fill_json, expected_exit_fill_json, metadata_json, error
FROM samples
WHERE completed_at >= ?
  AND transport != ''
  AND order_type != ''
  AND measurement_mode != ''
ORDER BY completed_at DESC
LIMIT ?
`, since.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var samples []bench.Sample
	for rows.Next() {
		var sample bench.Sample
		var completedAt string
		var scheduledAt string
		var sentAt string
		var scenario string
		var ok int
		var status string
		var cleanupAttempted int
		var cleanupOK int
		var cleanupStatusCode int
		var cleanupDurationNS int64
		var cleanupPreparedNS int64
		var cleanupScheduledAt string
		var cleanupSentAt string
		var cleanupStartDelayNS int64
		var cleanupWriteDelayNS int64
		var cleanupBytesRead int64
		var cleanupError string
		var cleanupDescription string
		var cleanupMetadataJSON string
		var orderRefsJSON string
		var closeoutRefsJSON string
		var expectedEntryJSON string
		var expectedExitJSON string
		var metadataJSON string
		if err := rows.Scan(
			&completedAt,
			&scheduledAt,
			&sentAt,
			&sample.Venue,
			&sample.RunID,
			&scenario,
			&sample.Transport,
			&sample.OrderType,
			&sample.MeasurementMode,
			&sample.Iteration,
			&sample.BatchSize,
			&sample.NetworkNS,
			&sample.RawNetworkNS,
			&sample.AdjustedNetworkNS,
			&sample.SpeedBumpNS,
			&sample.SpeedBumpSource,
			&sample.SubmissionNS,
			&sample.StartDelayNS,
			&sample.WriteDelayNS,
			&ok,
			&status,
			&sample.Classification.Reason,
			&cleanupAttempted,
			&cleanupOK,
			&cleanupStatusCode,
			&cleanupDurationNS,
			&cleanupPreparedNS,
			&cleanupScheduledAt,
			&cleanupSentAt,
			&cleanupStartDelayNS,
			&cleanupWriteDelayNS,
			&cleanupBytesRead,
			&cleanupError,
			&cleanupDescription,
			&cleanupMetadataJSON,
			&orderRefsJSON,
			&closeoutRefsJSON,
			&expectedEntryJSON,
			&expectedExitJSON,
			&metadataJSON,
			&sample.Error,
		); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, completedAt)
		if err != nil {
			return nil, fmt.Errorf("parse completed_at: %w", err)
		}
		sample.CompletedAt = parsed
		sample.ScheduledAt = parseOptionalTime(scheduledAt)
		sample.SentAt = parseOptionalTime(sentAt)
		if sample.RawNetworkNS == 0 {
			sample.RawNetworkNS = sample.NetworkNS
		}
		if sample.AdjustedNetworkNS == 0 && sample.NetworkNS > 0 {
			sample.AdjustedNetworkNS = sample.NetworkNS - sample.SpeedBumpNS
			if sample.AdjustedNetworkNS < 0 {
				sample.AdjustedNetworkNS = 0
			}
		}
		sample.Scenario = bench.Scenario(scenario)
		sample.OK = ok == 1
		sample.Classification.Status = lifecycle.ClassificationStatus(status)
		sample.Cleanup = &bench.CleanupResult{
			Attempted:    cleanupAttempted == 1,
			OK:           cleanupOK == 1,
			StatusCode:   cleanupStatusCode,
			Error:        cleanupError,
			DurationNS:   cleanupDurationNS,
			PreparedNS:   cleanupPreparedNS,
			ScheduledAt:  parseOptionalTime(cleanupScheduledAt),
			SentAt:       parseOptionalTime(cleanupSentAt),
			StartDelayNS: cleanupStartDelayNS,
			WriteDelayNS: cleanupWriteDelayNS,
			BytesRead:    cleanupBytesRead,
			Description:  cleanupDescription,
		}
		if cleanupMetadataJSON != "" {
			_ = json.Unmarshal([]byte(cleanupMetadataJSON), &sample.Cleanup.Metadata)
		}
		if metadataJSON != "" {
			_ = json.Unmarshal([]byte(metadataJSON), &sample.Metadata)
		}
		if orderRefsJSON != "" {
			_ = json.Unmarshal([]byte(orderRefsJSON), &sample.OrderRefs)
		}
		if closeoutRefsJSON != "" {
			_ = json.Unmarshal([]byte(closeoutRefsJSON), &sample.CloseoutOrderRefs)
		}
		if expectedEntryJSON != "" {
			var fill bench.ExpectedFill
			if err := json.Unmarshal([]byte(expectedEntryJSON), &fill); err == nil {
				sample.ExpectedEntryFill = &fill
			}
		}
		if expectedExitJSON != "" {
			var fill bench.ExpectedFill
			if err := json.Unmarshal([]byte(expectedExitJSON), &fill); err == nil {
				sample.ExpectedExitFill = &fill
			}
		}
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	costs, err := s.recentSampleCosts(ctx, since)
	if err != nil {
		return nil, err
	}
	for index := range samples {
		if cost, ok := costs[sampleCostKey(samples[index])]; ok {
			copy := cost
			samples[index].Cost = &copy
		}
	}
	return samples, nil
}

func (s *SQLite) recentSampleCosts(ctx context.Context, since time.Time) (map[string]bench.SampleCost, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT completed_at, venue, run_id, entry_order_id, exit_order_id, entry_qty, exit_qty,
       entry_value_usd, exit_value_usd, entry_fee_usd, exit_fee_usd, price_move_cost_usd, trade_cost_usd,
       balance_before_usd, balance_after_usd, balance_delta_usd, reconciliation_diff_usd, clean,
       quality_reason, metadata_json, balance_before_json, balance_after_json
FROM sample_costs
WHERE completed_at >= ?
`, since.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	costs := make(map[string]bench.SampleCost)
	for rows.Next() {
		var cost bench.SampleCost
		var completedAt string
		var clean int
		var metadataJSON string
		var beforeJSON string
		var afterJSON string
		if err := rows.Scan(
			&completedAt,
			&cost.Venue,
			&cost.RunID,
			&cost.EntryOrderID,
			&cost.ExitOrderID,
			&cost.EntryQty,
			&cost.ExitQty,
			&cost.EntryValueUSD,
			&cost.ExitValueUSD,
			&cost.EntryFeeUSD,
			&cost.ExitFeeUSD,
			&cost.PriceMoveCostUSD,
			&cost.TradeCostUSD,
			&cost.BalanceBeforeUSD,
			&cost.BalanceAfterUSD,
			&cost.BalanceDeltaUSD,
			&cost.ReconciliationDiffUSD,
			&clean,
			&cost.QualityReason,
			&metadataJSON,
			&beforeJSON,
			&afterJSON,
		); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, completedAt)
		if err != nil {
			return nil, err
		}
		cost.CompletedAt = parsed
		cost.Clean = clean == 1
		if metadataJSON != "" {
			_ = json.Unmarshal([]byte(metadataJSON), &cost.Metadata)
		}
		if beforeJSON != "" {
			_ = json.Unmarshal([]byte(beforeJSON), &cost.BalanceBefore)
		}
		if afterJSON != "" {
			_ = json.Unmarshal([]byte(afterJSON), &cost.BalanceAfter)
		}
		costs[sampleCostKeyFrom(cost.CompletedAt, cost.Venue, cost.RunID)] = cost
	}
	return costs, rows.Err()
}

func (s *SQLite) ensureSamplesOrderType(ctx context.Context) error {
	return s.ensureColumn(ctx, "order_type", `ALTER TABLE samples ADD COLUMN order_type TEXT NOT NULL DEFAULT ''`)
}

func (s *SQLite) ensureColumn(ctx context.Context, column string, statement string) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(samples)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, statement)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return nil
	}
	return err
}

func (s *SQLite) DeleteBefore(ctx context.Context, before time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM samples WHERE completed_at < ?`, before.UTC().Format(time.RFC3339Nano))
	return err
}

func cleanupFields(cleanup *bench.CleanupResult) (int, int) {
	if cleanup == nil {
		return 0, 0
	}
	return boolInt(cleanup.Attempted), boolInt(cleanup.OK)
}

func cleanupStorageFields(cleanup *bench.CleanupResult) (int, int64, int64, string, string, int64, int64, int64, string, string) {
	if cleanup == nil {
		return 0, 0, 0, "", "", 0, 0, 0, "", ""
	}
	return cleanup.StatusCode,
		cleanup.DurationNS,
		cleanup.PreparedNS,
		formatOptionalTime(cleanup.ScheduledAt),
		formatOptionalTime(cleanup.SentAt),
		cleanup.StartDelayNS,
		cleanup.WriteDelayNS,
		cleanup.BytesRead,
		cleanup.Error,
		cleanup.Description
}

func metadataJSON(metadata map[string]any) (string, error) {
	if len(metadata) == 0 {
		return "", nil
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func cleanupMetadataJSON(cleanup *bench.CleanupResult) (string, error) {
	if cleanup == nil {
		return "", nil
	}
	return metadataJSON(cleanup.Metadata)
}

func marshalJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func marshalOptionalJSON(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	return marshalJSON(value)
}

func sampleCostKey(sample bench.Sample) string {
	return sampleCostKeyFrom(sample.CompletedAt, sample.Venue, sample.RunID)
}

func sampleCostKeyFrom(completedAt time.Time, venue string, runID string) string {
	return completedAt.UTC().Format(time.RFC3339Nano) + "\x00" + venue + "\x00" + runID
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
