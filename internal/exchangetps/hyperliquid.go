package exchangetps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const DefaultHyperliquidWSURL = "wss://rpc.hyperliquid.xyz/ws"
const DefaultHyperliquidExplorerURL = "https://rpc.hyperliquid.xyz/explorer"
const DefaultHyperliquidActionSamplesPerMinute = 120

type HyperliquidCollector struct {
	WSURL                  string
	ExplorerURL            string
	ActionSamplesPerMinute int
	FlushInterval          time.Duration
	Aggregator             *Aggregator

	mu                  sync.Mutex
	seen                map[int64]struct{}
	lastHeight          int64
	ignoreThroughBucket int64
	sampledActionSlots  map[int64]map[int]struct{}
	detailJobs          chan hyperliquidBlock
}

type hyperliquidBlock struct {
	BlockTime int64  `json:"blockTime"`
	Hash      string `json:"hash"`
	Height    int64  `json:"height"`
	NumTxs    int64  `json:"numTxs"`
	Proposer  string `json:"proposer"`
}

func (c *HyperliquidCollector) Run(ctx context.Context, store *Store) error {
	if c.Aggregator == nil {
		return errors.New("aggregator is required")
	}
	description := "Hyperliquid explorerBlock numTxs stream bucketed by blockTime"
	if c.ActionSamplesPerMinute > 0 {
		description = fmt.Sprintf("%s; category split sampled from blockDetails at %d blocks/minute", description, c.ActionSamplesPerMinute)
	}
	if err := store.SetSourceMetadata(ctx, SourceMetadata{
		Venue:         "hyperliquid",
		Quality:       SourceQualityBlockDerived,
		BucketSeconds: c.Aggregator.BucketSeconds(),
		Description:   description,
	}); err != nil {
		return err
	}
	if c.seen == nil {
		c.seen = make(map[int64]struct{})
	}
	if c.sampledActionSlots == nil {
		c.sampledActionSlots = make(map[int64]map[int]struct{})
	}
	wsURL := c.WSURL
	if wsURL == "" {
		wsURL = DefaultHyperliquidWSURL
	}
	explorerURL := c.ExplorerURL
	if explorerURL == "" {
		explorerURL = DefaultHyperliquidExplorerURL
	}
	flushInterval := c.FlushInterval
	if flushInterval <= 0 {
		flushInterval = time.Second
	}
	if c.ActionSamplesPerMinute > 0 {
		c.detailJobs = make(chan hyperliquidBlock, max(2*c.ActionSamplesPerMinute, 10))
		for range hyperliquidActionSamplerWorkers(c.ActionSamplesPerMinute) {
			go c.runActionSampler(ctx, store, explorerURL)
		}
	}

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-ticker.C:
				if err := c.Aggregator.Flush(ctx, store, false); err != nil {
					log.Printf("hyperliquid tps flush failed: %v", err)
				}
			case <-done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	backoff := time.Second
	for ctx.Err() == nil {
		err := c.runSession(ctx, wsURL)
		if ctx.Err() != nil {
			break
		}
		log.Printf("hyperliquid tps websocket disconnected: %v", err)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			break
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
	if err := c.Aggregator.Flush(context.Background(), store, false); err != nil {
		return err
	}
	return ctx.Err()
}

func (c *HyperliquidCollector) runSession(ctx context.Context, wsURL string) error {
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, resp, err := dialer.DialContext(ctx, wsURL, http.Header{})
	if err != nil {
		if resp != nil {
			return fmt.Errorf("dial %s: %w (status %s)", wsURL, err, resp.Status)
		}
		return fmt.Errorf("dial %s: %w", wsURL, err)
	}
	defer conn.Close()
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
	conn.SetReadLimit(8 << 20)
	if err := conn.WriteJSON(map[string]any{
		"method":       "subscribe",
		"subscription": map[string]string{"type": "explorerBlock"},
	}); err != nil {
		return err
	}
	log.Printf("hyperliquid tps collector connected to %s", wsURL)
	c.ignoreCurrentSessionStartBucket()
	defer c.Aggregator.DropBucketAt(time.Now().UTC())
	for ctx.Err() == nil {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if err := c.handleMessage(data); err != nil {
			log.Printf("hyperliquid tps ignored message: %v", err)
		}
	}
	return ctx.Err()
}

func (c *HyperliquidCollector) acceptHeight(height int64) (int64, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if height <= 0 {
		return c.lastHeight, false, false
	}
	if c.seen == nil {
		c.seen = make(map[int64]struct{})
	}
	if _, ok := c.seen[height]; ok {
		return c.lastHeight, false, false
	}
	last := c.lastHeight
	gap := last > 0 && height > last+1
	c.seen[height] = struct{}{}
	if height > c.lastHeight {
		c.lastHeight = height
		c.pruneSeenLocked()
	}
	return last, gap, true
}

func (c *HyperliquidCollector) ignoreCurrentSessionStartBucket() {
	bucketUnix := time.Now().UTC().Truncate(time.Minute).Unix()
	c.mu.Lock()
	defer c.mu.Unlock()
	if bucketUnix > c.ignoreThroughBucket {
		c.ignoreThroughBucket = bucketUnix
	}
}

func (c *HyperliquidCollector) shouldIgnoreBlock(timestampMs int64) bool {
	if timestampMs <= 0 {
		return true
	}
	bucketUnix := time.Unix(floorUnix(time.UnixMilli(timestampMs).UTC().Unix(), 60), 0).UTC().Unix()
	c.mu.Lock()
	defer c.mu.Unlock()
	return bucketUnix <= c.ignoreThroughBucket
}

func (c *HyperliquidCollector) pruneSeenLocked() {
	const keepRecentHeights = 10000
	pruneBelow := c.lastHeight - keepRecentHeights
	if pruneBelow <= 0 {
		return
	}
	for height := range c.seen {
		if height < pruneBelow {
			delete(c.seen, height)
		}
	}
}

func (c *HyperliquidCollector) shouldSampleActionBreakdown(block hyperliquidBlock) bool {
	if c.ActionSamplesPerMinute <= 0 || block.BlockTime <= 0 {
		return false
	}
	ts := time.UnixMilli(block.BlockTime).UTC()
	bucketStart := ts.Truncate(time.Minute)
	bucketUnix := bucketStart.Unix()
	elapsedMs := int(ts.Sub(bucketStart) / time.Millisecond)
	slot := (elapsedMs * c.ActionSamplesPerMinute) / int(time.Minute/time.Millisecond)
	if slot < 0 {
		slot = 0
	}
	if slot >= c.ActionSamplesPerMinute {
		slot = c.ActionSamplesPerMinute - 1
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sampledActionSlots == nil {
		c.sampledActionSlots = make(map[int64]map[int]struct{})
	}
	slots := c.sampledActionSlots[bucketUnix]
	if slots == nil {
		slots = make(map[int]struct{})
		c.sampledActionSlots[bucketUnix] = slots
	}
	if _, ok := slots[slot]; ok {
		return false
	}
	slots[slot] = struct{}{}
	for oldBucket := range c.sampledActionSlots {
		if oldBucket < bucketUnix-120 {
			delete(c.sampledActionSlots, oldBucket)
		}
	}
	return true
}

func (c *HyperliquidCollector) enqueueActionSample(block hyperliquidBlock) {
	if c.detailJobs == nil {
		return
	}
	select {
	case c.detailJobs <- block:
	default:
		log.Printf("hyperliquid action sampler queue full; skipped height=%d", block.Height)
	}
}

func hyperliquidActionSamplerWorkers(samplesPerMinute int) int {
	if samplesPerMinute <= 60 {
		return 2
	}
	if samplesPerMinute <= 180 {
		return 4
	}
	return 8
}

func (c *HyperliquidCollector) runActionSampler(ctx context.Context, store *Store, explorerURL string) {
	client := &http.Client{Timeout: 8 * time.Second}
	for {
		select {
		case <-ctx.Done():
			return
		case block := <-c.detailJobs:
			delta, err := fetchHyperliquidActionSample(ctx, client, explorerURL, block)
			if err != nil {
				if ctx.Err() == nil {
					log.Printf("hyperliquid action sample failed height=%d: %v", block.Height, err)
				}
				continue
			}
			if err := store.RecordCategorySplitDelta1m(ctx, delta); err != nil {
				log.Printf("hyperliquid action sample store failed height=%d: %v", block.Height, err)
				continue
			}
			bucketStart := time.UnixMilli(block.BlockTime).UTC().Truncate(time.Minute)
			if err := store.RefreshRollups(ctx, "hyperliquid", bucketStart, bucketStart); err != nil {
				log.Printf("hyperliquid action sample rollup failed height=%d: %v", block.Height, err)
			}
		}
	}
}

func fetchHyperliquidActionSample(ctx context.Context, client *http.Client, explorerURL string, block hyperliquidBlock) (CategorySplitDelta, error) {
	body, err := json.Marshal(map[string]any{
		"type":   "blockDetails",
		"height": block.Height,
	})
	if err != nil {
		return CategorySplitDelta{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, explorerURL, bytes.NewReader(body))
	if err != nil {
		return CategorySplitDelta{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return CategorySplitDelta{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return CategorySplitDelta{}, fmt.Errorf("explorer status %d: %s", resp.StatusCode, string(limited))
	}
	limited := io.LimitReader(resp.Body, 16<<20)
	var payload hyperliquidBlockDetailsResponse
	if err := json.NewDecoder(limited).Decode(&payload); err != nil {
		return CategorySplitDelta{}, err
	}
	details := payload.BlockDetails
	if details.Height == 0 {
		return CategorySplitDelta{}, errors.New("missing blockDetails in explorer response")
	}
	categoryCounts := make(map[string]int64)
	for _, tx := range details.Txs {
		action := tx.actionType()
		if action == "" {
			action = "unknown"
		}
		categoryCounts[hyperliquidActionCategory(action)]++
	}
	sampledTxs := details.NumTxs
	if sampledTxs == 0 {
		sampledTxs = int64(len(details.Txs))
	}
	if details.NumTxs > 0 && int64(len(details.Txs)) != details.NumTxs {
		log.Printf("hyperliquid blockDetails tx length mismatch height=%d numTxs=%d txs=%d", details.Height, details.NumTxs, len(details.Txs))
	}
	if block.NumTxs > 0 && details.NumTxs > 0 && block.NumTxs != details.NumTxs {
		log.Printf("hyperliquid blockDetails numTxs mismatch height=%d stream=%d details=%d", block.Height, block.NumTxs, details.NumTxs)
	}
	categoryShares := make(map[string]int64)
	for _, category := range hyperliquidActionCategories() {
		categoryShares[category] = 0
	}
	if sampledTxs > 0 {
		for category, count := range categoryCounts {
			categoryShares[category] = (count * 1_000_000) / sampledTxs
		}
	}
	return CategorySplitDelta{
		Venue:          "hyperliquid",
		BucketStart:    time.UnixMilli(block.BlockTime).UTC().Truncate(time.Minute),
		SampledTxCount: sampledTxs,
		SampledBlocks:  1,
		CategoryShares: categoryShares,
	}, nil
}

func hyperliquidActionCategories() []string {
	return []string{"core_order", "core_cancel", "core_modify", "evm", "core_other"}
}

func hyperliquidActionCategory(action string) string {
	switch action {
	case "evmRawTx":
		return "evm"
	case "order":
		return "core_order"
	case "cancel", "cancelByCloid", "scheduleCancel":
		return "core_cancel"
	case "modify", "batchModify", "updateLeverage":
		return "core_modify"
	default:
		return "core_other"
	}
}

type hyperliquidBlockDetailsResponse struct {
	Type         string                  `json:"type"`
	BlockDetails hyperliquidBlockDetails `json:"blockDetails"`
}

type hyperliquidBlockDetails struct {
	Height int64                   `json:"height"`
	NumTxs int64                   `json:"numTxs"`
	Txs    []hyperliquidExplorerTx `json:"txs"`
}

type hyperliquidExplorerTx struct {
	Action json.RawMessage `json:"action"`
}

func (tx hyperliquidExplorerTx) actionType() string {
	if len(tx.Action) == 0 {
		return ""
	}
	var object struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(tx.Action, &object); err == nil && object.Type != "" {
		return object.Type
	}
	var value string
	if err := json.Unmarshal(tx.Action, &value); err == nil {
		return value
	}
	return ""
}

func (c *HyperliquidCollector) handleMessage(data []byte) error {
	var msg struct {
		Channel string          `json:"channel"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return c.handleBlocks(data)
	}
	if msg.Channel == "subscriptionResponse" || msg.Channel == "pong" {
		return nil
	}
	if msg.Channel == "explorerBlock" {
		return c.handleBlocks(msg.Data)
	}
	if len(msg.Data) > 0 {
		return nil
	}
	return c.handleBlocks(data)
}

func (c *HyperliquidCollector) handleBlocks(data []byte) error {
	var blocks []hyperliquidBlock
	if err := json.Unmarshal(data, &blocks); err != nil {
		var block hyperliquidBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return err
		}
		blocks = []hyperliquidBlock{block}
	}
	for _, block := range blocks {
		if c.shouldIgnoreBlock(block.BlockTime) {
			continue
		}
		lastHeight, gap, accepted := c.acceptHeight(block.Height)
		if !accepted {
			continue
		}
		if gap {
			log.Printf("hyperliquid tps height gap detected: last=%d current=%d", lastHeight, block.Height)
		}
		c.Aggregator.AddBlock(block.BlockTime, block.NumTxs)
		if c.shouldSampleActionBreakdown(block) {
			c.enqueueActionSample(block)
		}
	}
	return nil
}
