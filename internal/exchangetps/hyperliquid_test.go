package exchangetps

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
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

func TestHyperliquidActionSamplerFetchesBlockDetailsIncludingEVM(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		_, _ = w.Write([]byte(`{
			"type":"blockDetails",
			"blockDetails":{
				"height":123,
				"numTxs":4,
				"txs":[
					{"action":{"type":"order"}},
					{"action":{"type":"cancel"}},
					{"action":{"type":"evmRawTx"}},
					{"action":{"type":"evmRawTx"}}
				]
			}
		}`))
	}))
	defer server.Close()

	delta, err := fetchHyperliquidActionSample(ctx, server.Client(), server.URL, hyperliquidBlock{
		Height:    123,
		BlockTime: time.Unix(1_700_000_000, 0).UTC().UnixMilli(),
		NumTxs:    4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if delta.SampledTxCount != 4 || delta.SampledBlocks != 1 {
		t.Fatalf("unexpected sample denominator: %+v", delta)
	}
	if delta.CategoryShares["core_order"] != 250_000 || delta.CategoryShares["core_cancel"] != 250_000 || delta.CategoryShares["evm"] != 500_000 {
		t.Fatalf("unexpected category shares: %+v", delta.CategoryShares)
	}
}

func TestHyperliquidSamplerSelectsConfiguredSlotsPerMinute(t *testing.T) {
	collector := &HyperliquidCollector{ActionSamplesPerMinute: 10}
	start := time.Unix(1_700_000_000, 0).UTC().Truncate(time.Minute)

	if !collector.shouldSampleActionBreakdown(hyperliquidBlock{BlockTime: start.Add(time.Second).UnixMilli()}) {
		t.Fatal("first slot should be sampled")
	}
	if collector.shouldSampleActionBreakdown(hyperliquidBlock{BlockTime: start.Add(2 * time.Second).UnixMilli()}) {
		t.Fatal("same slot should not be sampled twice")
	}
	if !collector.shouldSampleActionBreakdown(hyperliquidBlock{BlockTime: start.Add(7 * time.Second).UnixMilli()}) {
		t.Fatal("next slot should be sampled")
	}
}
