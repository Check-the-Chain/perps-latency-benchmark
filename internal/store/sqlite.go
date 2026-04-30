package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	db, err := sql.Open("sqlite", path)
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
);
CREATE INDEX IF NOT EXISTS samples_completed_at_idx ON samples(completed_at);
CREATE INDEX IF NOT EXISTS samples_group_idx ON samples(venue, transport, scenario, latency_mode, completed_at);
`)
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
  completed_at, venue, run_id, scenario, transport, latency_mode, iteration,
  batch_size, network_ns, ok, classification, classification_reason,
  cleanup_attempted, cleanup_ok, error
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		if _, err := stmt.ExecContext(ctx,
			sample.CompletedAt.UTC().Format(time.RFC3339Nano),
			sample.Venue,
			sample.RunID,
			string(sample.Scenario),
			sample.Transport,
			string(record.LatencyMode),
			sample.Iteration,
			sample.BatchSize,
			sample.NetworkNS,
			boolInt(sample.OK),
			string(sample.Classification.Status),
			sample.Classification.Reason,
			cleanupAttempted,
			cleanupOK,
			sample.Error,
		); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLite) RecentSamples(ctx context.Context, since time.Time, limit int) ([]bench.Sample, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT completed_at, venue, run_id, scenario, transport, iteration, batch_size,
       network_ns, ok, classification, classification_reason,
       cleanup_attempted, cleanup_ok, error
FROM samples
WHERE completed_at >= ?
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
		var scenario string
		var ok int
		var status string
		var cleanupAttempted int
		var cleanupOK int
		if err := rows.Scan(
			&completedAt,
			&sample.Venue,
			&sample.RunID,
			&scenario,
			&sample.Transport,
			&sample.Iteration,
			&sample.BatchSize,
			&sample.NetworkNS,
			&ok,
			&status,
			&sample.Classification.Reason,
			&cleanupAttempted,
			&cleanupOK,
			&sample.Error,
		); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, completedAt)
		if err != nil {
			return nil, fmt.Errorf("parse completed_at: %w", err)
		}
		sample.CompletedAt = parsed
		sample.Scenario = bench.Scenario(scenario)
		sample.OK = ok == 1
		sample.Classification.Status = lifecycle.ClassificationStatus(status)
		sample.Cleanup = &bench.CleanupResult{Attempted: cleanupAttempted == 1, OK: cleanupOK == 1}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
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

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
