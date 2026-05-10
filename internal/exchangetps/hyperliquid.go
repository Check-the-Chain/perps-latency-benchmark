package exchangetps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const DefaultHyperliquidWSURL = "wss://rpc.hyperliquid.xyz/ws"

type HyperliquidCollector struct {
	WSURL         string
	FlushInterval time.Duration
	Aggregator    *Aggregator

	mu         sync.Mutex
	seen       map[int64]struct{}
	lastHeight int64
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
	if err := store.SetSourceMetadata(ctx, SourceMetadata{
		Venue:         "hyperliquid",
		Quality:       SourceQualityBlockDerived,
		BucketSeconds: c.Aggregator.BucketSeconds(),
		Description:   "Hyperliquid explorerBlock numTxs stream bucketed by blockTime",
	}); err != nil {
		return err
	}
	if c.seen == nil {
		c.seen = make(map[int64]struct{})
	}
	wsURL := c.WSURL
	if wsURL == "" {
		wsURL = DefaultHyperliquidWSURL
	}
	flushInterval := c.FlushInterval
	if flushInterval <= 0 {
		flushInterval = time.Second
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
	if err := c.Aggregator.Flush(context.Background(), store, true); err != nil {
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
		lastHeight, gap, accepted := c.acceptHeight(block.Height)
		if !accepted {
			continue
		}
		if gap {
			log.Printf("hyperliquid tps height gap detected: last=%d current=%d", lastHeight, block.Height)
		}
		c.Aggregator.AddBlock(block.BlockTime, block.NumTxs)
	}
	return nil
}
