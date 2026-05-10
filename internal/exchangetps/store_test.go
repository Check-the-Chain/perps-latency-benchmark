package exchangetps

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestStoreRecordsLiveDeltasAndRollups(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "exchange_tps.db")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	start := time.Unix(1_778_391_960, 0).UTC().Truncate(time.Minute)
	if err := store.RecordLiveDelta1m(ctx, BucketDelta{
		Venue:       "aster",
		BucketStart: start,
		TxCount:     120,
		BlockCount:  2,
		OrderCount:  30,
		PlaceCount:  20,
		CancelCount: 10,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordLiveDelta1m(ctx, BucketDelta{
		Venue:       "aster",
		BucketStart: start,
		TxCount:     80,
		BlockCount:  1,
		OrderCount:  12,
		PlaceCount:  8,
		CancelCount: 4,
		ErrorCount:  1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RefreshRollups(ctx, "aster", start, start); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", exchangeTPSDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var txCount, blockCount, orderCount, errorCount int64
	if err := db.QueryRowContext(ctx, `
SELECT tx, blk, ord, err
FROM tps_1m
JOIN venues ON venues.id = tps_1m.v
WHERE venues.code = 'aster' AND tps_1m.t = ?
`, floorUnix(start.Unix(), 60)).Scan(&txCount, &blockCount, &orderCount, &errorCount); err != nil {
		t.Fatal(err)
	}
	if txCount != 200 || blockCount != 3 || orderCount != 42 || errorCount != 1 {
		t.Fatalf("unexpected rollup counts: tx=%d blk=%d ord=%d err=%d", txCount, blockCount, orderCount, errorCount)
	}
}

func TestStoreRecordsObservedBucketByReplacingOverlap(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "exchange_tps.db")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	start := time.Unix(1_778_391_960, 0).UTC().Truncate(time.Minute)
	if err := store.RecordObservedBucket1m(ctx, BucketDelta{Venue: "lighter", BucketStart: start, TxCount: 100}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordObservedBucket1m(ctx, BucketDelta{Venue: "lighter", BucketStart: start, TxCount: 125}); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", exchangeTPSDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var txCount int64
	if err := db.QueryRowContext(ctx, `
SELECT tx
FROM tps_1m
JOIN venues ON venues.id = tps_1m.v
WHERE venues.code = 'lighter' AND tps_1m.t = ?
`, start.Unix()).Scan(&txCount); err != nil {
		t.Fatal(err)
	}
	if txCount != 125 {
		t.Fatalf("expected replacement count 125, got %d", txCount)
	}
}

func TestOpenStoreDropsUnsupportedTPSTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exchange_tps.db")
	db, err := sql.Open("sqlite", exchangeTPSDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE tps_legacy_raw (v INTEGER NOT NULL, t INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	db, err = sql.Open("sqlite", exchangeTPSDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'tps_legacy_raw'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("unsupported TPS table still exists")
	}
}

func TestLighterCollectOnceWritesCompactBuckets(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "exchange_tps.db")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	base := time.Unix(1_778_391_960, 0).UTC().Truncate(time.Minute)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("period") != "h" || r.URL.Query().Get("kind") != "tps" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		_, _ = fmt.Fprintf(w, `{"code":200,"metrics":[
			{"timestamp":%d,"data":12},
			{"timestamp":%d,"data":18},
			{"timestamp":%d,"data":3},
			{"timestamp":%d,"data":6},
			{"timestamp":%d,"data":7},
			{"timestamp":%d,"data":8}
		]}`,
			base.UnixMilli(),
			base.Add(15*time.Second).UnixMilli(),
			base.Add(30*time.Second).UnixMilli(),
			base.Add(time.Minute).UnixMilli(),
			base.Add(time.Minute+15*time.Second).UnixMilli(),
			base.Add(time.Minute+30*time.Second).UnixMilli(),
		)
	}))
	defer server.Close()

	collector := &LighterCollector{MetricsURL: server.URL}
	if err := collector.CollectOnce(ctx, store); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", exchangeTPSDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `
SELECT tps_1m.t, tx
FROM tps_1m
JOIN venues ON venues.id = tps_1m.v
WHERE venues.code = 'lighter'
ORDER BY tps_1m.t
`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []int64
	for rows.Next() {
		var ts, txCount int64
		if err := rows.Scan(&ts, &txCount); err != nil {
			t.Fatal(err)
		}
		got = append(got, txCount)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != 495 || got[1] != 315 {
		t.Fatalf("unexpected lighter tx counts: %v", got)
	}
}

func TestSourceMetadataIsStoredPerVenue(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "exchange_tps.db")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.SetSourceMetadata(ctx, SourceMetadata{
		Venue:         "aster",
		Quality:       SourceQualityBlockDerived,
		BucketSeconds: 60,
		Description:   "block stream",
	}); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", exchangeTPSDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var quality, bucketSeconds int64
	var description string
	if err := db.QueryRowContext(ctx, `
SELECT q, bucket_s, description
FROM venue_sources
JOIN venues ON venues.id = venue_sources.v
WHERE venues.code = 'aster'
`).Scan(&quality, &bucketSeconds, &description); err != nil {
		t.Fatal(err)
	}
	if SourceQuality(quality) != SourceQualityBlockDerived || bucketSeconds != 60 || description != "block stream" {
		t.Fatalf("unexpected metadata: quality=%d bucket=%d description=%q", quality, bucketSeconds, description)
	}
}

func TestRecentSeriesReads1mAnd1hLevels(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "exchange_tps.db")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	start := time.Unix(floorUnix(1_700_000_000, 3600), 0).UTC()
	if err := store.SetSourceMetadata(ctx, SourceMetadata{
		Venue:         "lighter",
		Quality:       SourceQualityProviderReported,
		BucketSeconds: 60,
		Description:   "test source",
	}); err != nil {
		t.Fatal(err)
	}
	for _, delta := range []BucketDelta{
		{Venue: "lighter", BucketStart: start, TxCount: 100},
		{Venue: "lighter", BucketStart: start.Add(time.Minute), TxCount: 200},
		{Venue: "lighter", BucketStart: start.Add(2 * time.Minute), TxCount: 300},
	} {
		if err := store.RecordObservedBucket1m(ctx, delta); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.RefreshRollups(ctx, "lighter", start, start.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}

	oneMinute, err := store.RecentSeries(ctx, SeriesBucket1m, start.Add(-time.Second), 10)
	if err != nil {
		t.Fatal(err)
	}
	if oneMinute.Bucket != "1m" {
		t.Fatalf("bucket = %q, want 1m", oneMinute.Bucket)
	}
	if len(oneMinute.Series) != 3 {
		t.Fatalf("1m rows = %d, want 3", len(oneMinute.Series))
	}
	if oneMinute.Series[0].TxCount != 100 || oneMinute.Series[0].TPS != float64(100)/60 {
		t.Fatalf("unexpected 1m first row: %+v", oneMinute.Series[0])
	}
	if oneMinute.Series[2].TxCount != 300 || oneMinute.Series[2].TPS != 5 {
		t.Fatalf("unexpected 1m third row: %+v", oneMinute.Series[2])
	}

	oneHour, err := store.RecentSeries(ctx, SeriesBucket1h, start.Add(-time.Second), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(oneHour.Series) != 1 {
		t.Fatalf("1h rows = %d, want 1", len(oneHour.Series))
	}
	if oneHour.Series[0].TxCount != 600 || oneHour.Series[0].TPS != float64(600)/3600 {
		t.Fatalf("unexpected 1h row: %+v", oneHour.Series[0])
	}
	if len(oneHour.Sources) != 1 || oneHour.Sources[0].Quality != "provider-reported" {
		t.Fatalf("unexpected sources: %+v", oneHour.Sources)
	}
}

func TestActionClassification(t *testing.T) {
	for _, action := range []string{"PlaceOrder", "PlaceStrategy", "CancelOrder", "CancelOrders", "CountdownCancelAll"} {
		if !IsOrderAction(action) {
			t.Fatalf("%s should be an order action", action)
		}
	}
	if IsOrderAction("Deposit") {
		t.Fatal("Deposit should not be an order action")
	}
}
