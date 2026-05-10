package exchangetps

import (
	"context"
	"fmt"
	"time"
)

type SeriesBucket string

const (
	SeriesBucket1m SeriesBucket = "1m"
	SeriesBucket1h SeriesBucket = "1h"
)

type SeriesReadModel struct {
	UpdatedAt time.Time   `json:"updated_at"`
	Bucket    string      `json:"bucket"`
	Window    string      `json:"window"`
	Series    []SeriesRow `json:"series"`
	Sources   []SourceRow `json:"sources"`
}

type SeriesRow struct {
	Venue           string    `json:"venue"`
	BucketStart     time.Time `json:"bucket_start"`
	BucketSeconds   int64     `json:"bucket_seconds"`
	Complete        bool      `json:"complete"`
	TxCount         int64     `json:"tx_count"`
	BlockCount      int64     `json:"block_count,omitempty"`
	OrderCount      int64     `json:"order_count,omitempty"`
	PlaceCount      int64     `json:"place_count,omitempty"`
	CancelCount     int64     `json:"cancel_count,omitempty"`
	ErrorCount      int64     `json:"error_count,omitempty"`
	TPS             float64   `json:"tps"`
	OrdersPerSecond float64   `json:"orders_per_second,omitempty"`
	SourceQuality   string    `json:"source_quality"`
}

type SourceRow struct {
	Venue         string `json:"venue"`
	Quality       string `json:"quality"`
	BucketSeconds int64  `json:"bucket_seconds"`
	Description   string `json:"description"`
	UpdatedAt     int64  `json:"updated_at"`
}

func (s *Store) RecentSeries(ctx context.Context, bucket SeriesBucket, since time.Time, limit int) (SeriesReadModel, error) {
	parsedBucket, err := ParseSeriesBucket(string(bucket))
	if err != nil {
		return SeriesReadModel{}, err
	}
	table, bucketSeconds, err := seriesTable(parsedBucket)
	if err != nil {
		return SeriesReadModel{}, err
	}
	if limit <= 0 {
		limit = 5000
	}
	now := time.Now().UTC()
	query := fmt.Sprintf(`
SELECT venues.code, %s.t, %s.tx, %s.blk, %s.ord, %s.plc, %s.cxl, %s.err,
       COALESCE(venue_sources.q, 0)
FROM %s
JOIN venues ON venues.id = %s.v
LEFT JOIN venue_sources ON venue_sources.v = venues.id
WHERE %s.t >= ?
ORDER BY %s.t ASC, venues.code ASC
LIMIT ?
`, table, table, table, table, table, table, table, table, table, table, table)
	rows, err := s.db.QueryContext(ctx, query, since.UTC().Unix(), limit)
	if err != nil {
		return SeriesReadModel{}, err
	}
	defer rows.Close()

	series := make([]SeriesRow, 0)
	for rows.Next() {
		var row SeriesRow
		var bucketUnix int64
		var sourceQuality SourceQuality
		if err := rows.Scan(
			&row.Venue,
			&bucketUnix,
			&row.TxCount,
			&row.BlockCount,
			&row.OrderCount,
			&row.PlaceCount,
			&row.CancelCount,
			&row.ErrorCount,
			&sourceQuality,
		); err != nil {
			return SeriesReadModel{}, err
		}
		row.BucketStart = time.Unix(bucketUnix, 0).UTC()
		row.BucketSeconds = bucketSeconds
		row.Complete = !row.BucketStart.Add(time.Duration(bucketSeconds) * time.Second).After(now)
		row.TPS = float64(row.TxCount) / float64(bucketSeconds)
		if row.OrderCount > 0 {
			row.OrdersPerSecond = float64(row.OrderCount) / float64(bucketSeconds)
		}
		row.SourceQuality = sourceQuality.String()
		series = append(series, row)
	}
	if err := rows.Err(); err != nil {
		return SeriesReadModel{}, err
	}

	sources, err := s.SourceRows(ctx)
	if err != nil {
		return SeriesReadModel{}, err
	}
	return SeriesReadModel{
		UpdatedAt: now,
		Bucket:    string(parsedBucket),
		Window:    time.Since(since).Round(time.Second).String(),
		Series:    series,
		Sources:   sources,
	}, nil
}

func (s *Store) SourceRows(ctx context.Context) ([]SourceRow, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT venues.code, venue_sources.q, venue_sources.bucket_s, venue_sources.description, venue_sources.upd
FROM venue_sources
JOIN venues ON venues.id = venue_sources.v
ORDER BY venues.code ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sources := make([]SourceRow, 0)
	for rows.Next() {
		var source SourceRow
		var quality SourceQuality
		if err := rows.Scan(&source.Venue, &quality, &source.BucketSeconds, &source.Description, &source.UpdatedAt); err != nil {
			return nil, err
		}
		source.Quality = quality.String()
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sources, nil
}

func (q SourceQuality) String() string {
	switch q {
	case SourceQualityBlockDerived:
		return "block-derived"
	case SourceQualityProviderReported:
		return "provider-reported"
	default:
		return "unknown"
	}
}

func seriesTable(bucket SeriesBucket) (string, int64, error) {
	switch bucket {
	case SeriesBucket1m:
		return "tps_1m", 60, nil
	case SeriesBucket1h:
		return "tps_1h", 3600, nil
	}
	return "", 0, fmt.Errorf("unsupported exchange TPS bucket %q", bucket)
}

func ParseSeriesBucket(value string) (SeriesBucket, error) {
	switch SeriesBucket(value) {
	case "":
		return SeriesBucket1m, nil
	case SeriesBucket1m:
		return SeriesBucket1m, nil
	case SeriesBucket1h:
		return SeriesBucket1h, nil
	default:
		return "", fmt.Errorf("unsupported exchange TPS bucket %q", value)
	}
}
