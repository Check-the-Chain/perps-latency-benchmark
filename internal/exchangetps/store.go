package exchangetps

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type SourceQuality int64

const (
	SourceQualityUnknown          SourceQuality = 0
	SourceQualityBlockDerived     SourceQuality = 1
	SourceQualityProviderReported SourceQuality = 2
)

type BucketDelta struct {
	Venue       string
	BucketStart time.Time
	TxCount     int64
	BlockCount  int64
	OrderCount  int64
	PlaceCount  int64
	CancelCount int64
	ErrorCount  int64
}

type SourceMetadata struct {
	Venue         string
	Quality       SourceQuality
	BucketSeconds int64
	Description   string
}

func OpenStore(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("exchange TPS store path is required")
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", exchangeTPSDSN(path))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	if err := store.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func exchangeTPSDSN(path string) string {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(10000)")
	q.Add("_pragma", "journal_mode(WAL)")
	return path + "?" + q.Encode()
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
PRAGMA journal_mode=WAL;
PRAGMA busy_timeout=10000;
PRAGMA auto_vacuum=INCREMENTAL;
CREATE TABLE IF NOT EXISTS venues (
  id INTEGER PRIMARY KEY,
  code TEXT NOT NULL UNIQUE
);
CREATE TABLE IF NOT EXISTS venue_sources (
  v INTEGER PRIMARY KEY,
  q INTEGER NOT NULL,
  bucket_s INTEGER NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  upd INTEGER NOT NULL
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS tps_1m (
  v INTEGER NOT NULL,
  t INTEGER NOT NULL,
  tx INTEGER NOT NULL DEFAULT 0,
  blk INTEGER NOT NULL DEFAULT 0,
  ord INTEGER NOT NULL DEFAULT 0,
  plc INTEGER NOT NULL DEFAULT 0,
  cxl INTEGER NOT NULL DEFAULT 0,
  err INTEGER NOT NULL DEFAULT 0,
  upd INTEGER NOT NULL,
  PRIMARY KEY (v, t)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS tps_1h (
  v INTEGER NOT NULL,
  t INTEGER NOT NULL,
  tx INTEGER NOT NULL DEFAULT 0,
  blk INTEGER NOT NULL DEFAULT 0,
  ord INTEGER NOT NULL DEFAULT 0,
  plc INTEGER NOT NULL DEFAULT 0,
  cxl INTEGER NOT NULL DEFAULT 0,
  err INTEGER NOT NULL DEFAULT 0,
  upd INTEGER NOT NULL,
  PRIMARY KEY (v, t)
) WITHOUT ROWID;
DROP INDEX IF EXISTS tps_1m_time_idx;
DROP INDEX IF EXISTS tps_1h_time_idx;
`)
	if err != nil {
		return err
	}
	return s.dropUnsupportedTPSTables(ctx)
}

func (s *Store) RecordLiveDelta1m(ctx context.Context, delta BucketDelta) error {
	return s.write1m(ctx, delta, true)
}

func (s *Store) RecordObservedBucket1m(ctx context.Context, delta BucketDelta) error {
	return s.write1m(ctx, delta, false)
}

func (s *Store) SetSourceMetadata(ctx context.Context, metadata SourceMetadata) error {
	if metadata.Venue == "" {
		return errors.New("venue is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	venueID, err := ensureVenue(ctx, tx, metadata.Venue)
	if err != nil {
		return err
	}
	if metadata.BucketSeconds <= 0 {
		metadata.BucketSeconds = 60
	}
	if metadata.Quality == SourceQualityUnknown {
		metadata.Quality = SourceQualityProviderReported
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO venue_sources (v, q, bucket_s, description, upd)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(v) DO UPDATE SET
  q = excluded.q,
  bucket_s = excluded.bucket_s,
  description = excluded.description,
  upd = excluded.upd
`, venueID, metadata.Quality, metadata.BucketSeconds, metadata.Description, time.Now().Unix())
	if err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

func (s *Store) write1m(ctx context.Context, delta BucketDelta, additive bool) error {
	if delta.Venue == "" {
		return errors.New("venue is required")
	}
	if delta.BucketStart.IsZero() {
		return errors.New("bucket start is required")
	}
	delta.BucketStart = time.Unix(floorUnix(delta.BucketStart.UTC().Unix(), 60), 0).UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	venueID, err := ensureVenue(ctx, tx, delta.Venue)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	query := `
INSERT INTO tps_1m (v, t, tx, blk, ord, plc, cxl, err, upd)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(v, t) DO UPDATE SET
  tx = excluded.tx,
  blk = excluded.blk,
  ord = excluded.ord,
  plc = excluded.plc,
  cxl = excluded.cxl,
  err = excluded.err,
  upd = excluded.upd
`
	if additive {
		query = `
INSERT INTO tps_1m (v, t, tx, blk, ord, plc, cxl, err, upd)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(v, t) DO UPDATE SET
  tx = tx + excluded.tx,
  blk = blk + excluded.blk,
  ord = ord + excluded.ord,
  plc = plc + excluded.plc,
  cxl = cxl + excluded.cxl,
  err = err + excluded.err,
  upd = excluded.upd
`
	}
	_, err = tx.ExecContext(ctx, query, venueID, delta.BucketStart.Unix(), delta.TxCount, delta.BlockCount, delta.OrderCount, delta.PlaceCount, delta.CancelCount, delta.ErrorCount, now)
	if err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

func ensureVenue(ctx context.Context, tx *sql.Tx, code string) (int64, error) {
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO venues (code) VALUES (?)`, code); err != nil {
		return 0, err
	}
	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM venues WHERE code = ?`, code).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) RefreshRollups(ctx context.Context, venue string, from, to time.Time) error {
	if venue == "" {
		return errors.New("venue is required")
	}
	if to.Before(from) {
		return nil
	}
	fromHour := floorUnix(from.Unix(), 3600)
	toHour := floorUnix(to.Unix(), 3600)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	venueID, err := ensureVenue(ctx, tx, venue)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	if _, err = tx.ExecContext(ctx, `DELETE FROM tps_1h WHERE v = ? AND t BETWEEN ? AND ?`, venueID, fromHour, toHour); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO tps_1h (v, t, tx, blk, ord, plc, cxl, err, upd)
SELECT v, (t / 3600) * 3600, SUM(tx), SUM(blk), SUM(ord), SUM(plc), SUM(cxl), SUM(err), ?
FROM tps_1m
WHERE v = ? AND t BETWEEN ? AND ?
GROUP BY v, (t / 3600) * 3600
`, now, venueID, fromHour, toHour+3599); err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

func (s *Store) ApplyRetention(ctx context.Context, minuteRetention time.Duration) error {
	now := time.Now()
	if minuteRetention > 0 {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM tps_1m WHERE t < ?`, now.Add(-minuteRetention).Unix()); err != nil {
			return err
		}
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA incremental_vacuum`); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	return err
}

func (s *Store) dropUnsupportedTPSTables(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name GLOB 'tps_*'`)
	if err != nil {
		return err
	}
	defer rows.Close()

	allowed := map[string]struct{}{
		"tps_1m": {},
		"tps_1h": {},
	}
	var unsupported []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		if _, ok := allowed[name]; ok {
			continue
		}
		unsupported = append(unsupported, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, name := range unsupported {
		if _, err := s.db.ExecContext(ctx, `DROP TABLE IF EXISTS `+quoteSQLiteIdentifier(name)); err != nil {
			return err
		}
	}
	return nil
}

func quoteSQLiteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func floorUnix(ts, bucketSeconds int64) int64 {
	if bucketSeconds <= 0 {
		return ts
	}
	return ts - ts%bucketSeconds
}
