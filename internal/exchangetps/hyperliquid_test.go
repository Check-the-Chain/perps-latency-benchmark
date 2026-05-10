package exchangetps

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestHyperliquidCollectorParsesExplorerBlockEnvelopeAndDedupesHeights(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "exchange_tps.db")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	collector := &HyperliquidCollector{
		Aggregator: NewAggregator("hyperliquid", time.Minute),
	}
	if err := collector.handleMessage([]byte(`{
		"channel":"explorerBlock",
		"data":[
			{"blockTime":1778391960000,"hash":"0x0000000000000000000000000000000000000000000000000000000000000001","height":10,"numTxs":3,"proposer":"0x0000000000000000000000000000000000000001"},
			{"blockTime":1778391960000,"hash":"0x0000000000000000000000000000000000000000000000000000000000000001","height":10,"numTxs":3,"proposer":"0x0000000000000000000000000000000000000001"},
			{"blockTime":1778391961000,"hash":"0x0000000000000000000000000000000000000000000000000000000000000002","height":11,"numTxs":7,"proposer":"0x0000000000000000000000000000000000000001"}
		]
	}`)); err != nil {
		t.Fatal(err)
	}
	if err := collector.Aggregator.Flush(ctx, store, true); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", exchangeTPSDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var txCount, blockCount int64
	if err := db.QueryRowContext(ctx, `
SELECT tx, blk
FROM tps_1m
JOIN venues ON venues.id = tps_1m.v
WHERE venues.code = 'hyperliquid' AND tps_1m.t = ?
`, time.UnixMilli(1778391960000).UTC().Truncate(time.Minute).Unix()).Scan(&txCount, &blockCount); err != nil {
		t.Fatal(err)
	}
	if txCount != 10 || blockCount != 2 {
		t.Fatalf("unexpected bucket counts: tx=%d blocks=%d", txCount, blockCount)
	}
}

func TestHyperliquidCollectorSkipsSessionStartBucket(t *testing.T) {
	collector := &HyperliquidCollector{
		Aggregator: NewAggregator("hyperliquid", time.Minute),
	}
	now := time.Now().UTC()
	collector.ignoreThroughBucket = now.Truncate(time.Minute).Unix()

	if !collector.shouldIgnoreBlock(now.UnixMilli()) {
		t.Fatal("current session bucket should be ignored")
	}
	if collector.shouldIgnoreBlock(now.Add(time.Minute).UnixMilli()) {
		t.Fatal("next full bucket should not be ignored")
	}
}
