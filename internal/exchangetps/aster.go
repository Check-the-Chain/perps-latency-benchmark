package exchangetps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const DefaultAsterWSURL = "wss://chainstream.asterdex.com/plain/ws"

type AsterCollector struct {
	WSURL         string
	Aggregator    *Aggregator
	FlushInterval time.Duration
}

func (c *AsterCollector) Run(ctx context.Context, store *Store) error {
	if c.Aggregator == nil {
		return errors.New("aggregator is required")
	}
	if err := store.SetSourceMetadata(ctx, SourceMetadata{
		Venue:         "aster",
		Quality:       SourceQualityBlockDerived,
		BucketSeconds: c.Aggregator.BucketSeconds(),
		Description:   "AsterScan explorer block txCount plus explorer transaction action stream",
	}); err != nil {
		return err
	}
	wsURL := c.WSURL
	if wsURL == "" {
		wsURL = DefaultAsterWSURL
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
					log.Printf("aster tps flush failed: %v", err)
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
		log.Printf("aster tps websocket disconnected: %v", err)
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

func (c *AsterCollector) runSession(ctx context.Context, wsURL string) error {
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
		"method": "SUBSCRIBE",
		"params": []string{"block@explorer", "transaction@explorer"},
		"id":     1,
	}); err != nil {
		return err
	}
	log.Printf("aster tps collector connected to %s", wsURL)
	for ctx.Err() == nil {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if err := c.handleMessage(data); err != nil {
			log.Printf("aster tps ignored message: %v", err)
		}
	}
	return ctx.Err()
}

func (c *AsterCollector) handleMessage(data []byte) error {
	var msg struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return err
	}
	switch msg.Type {
	case "block":
		var block struct {
			Timestamp int64 `json:"timestamp"`
			TxCount   int64 `json:"txCount"`
		}
		if err := json.Unmarshal(msg.Data, &block); err != nil {
			return err
		}
		c.Aggregator.AddBlock(block.Timestamp, block.TxCount)
	case "transaction":
		var tx struct {
			Timestamp int64  `json:"timestamp"`
			Action    string `json:"action"`
			Error     string `json:"error"`
		}
		if err := json.Unmarshal(msg.Data, &tx); err != nil {
			return err
		}
		c.Aggregator.AddTransaction(tx.Timestamp, tx.Action, tx.Error)
	}
	return nil
}
