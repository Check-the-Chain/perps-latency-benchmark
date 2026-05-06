package confirmws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"perps-latency-benchmark/internal/netlatency"
)

type Client struct {
	conn      *websocket.Conn
	readLimit int64
}

func Dial(ctx context.Context, url string, headers http.Header, readInitial bool) (*Client, error) {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, headers)
	if err != nil {
		return nil, err
	}
	client := &Client{conn: conn, readLimit: 4 << 20}
	if readInitial {
		if _, _, err := client.readRaw(ctx); err != nil {
			_ = client.Close()
			return nil, err
		}
	}
	return client, nil
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *Client) WriteJSON(ctx context.Context, value any) error {
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.conn.SetWriteDeadline(deadline)
	} else {
		_ = c.conn.SetWriteDeadline(time.Time{})
	}
	return c.conn.WriteJSON(value)
}

func (c *Client) Wait(ctx context.Context, start time.Time, match func(map[string]any) (bool, error)) (netlatency.Result, error) {
	for {
		data, receivedAt, err := c.readRaw(ctx)
		if err != nil {
			return result(start, time.Now(), 0, nil), err
		}
		if int64(len(data)) > c.readLimit {
			return result(start, receivedAt, int64(len(data)), data), fmt.Errorf("websocket confirmation exceeded read limit: %d", len(data))
		}
		var decoded map[string]any
		if err := json.Unmarshal(data, &decoded); err != nil {
			continue
		}
		ok, err := match(decoded)
		if err != nil {
			return result(start, receivedAt, int64(len(data)), data), err
		}
		if ok {
			return result(start, receivedAt, int64(len(data)), data), nil
		}
	}
}

func (c *Client) DrainUntil(ctx context.Context, match func(map[string]any) bool) error {
	_, err := c.Wait(ctx, time.Now(), func(value map[string]any) (bool, error) {
		return match(value), nil
	})
	return err
}

func (c *Client) readRaw(ctx context.Context) ([]byte, time.Time, error) {
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.conn.SetReadDeadline(deadline)
	} else {
		_ = c.conn.SetReadDeadline(time.Time{})
	}
	_, data, err := c.conn.ReadMessage()
	return data, time.Now(), err
}

func result(start time.Time, finish time.Time, bytesRead int64, body []byte) netlatency.Result {
	if start.IsZero() {
		start = finish
	}
	if finish.Before(start) {
		finish = start
	}
	totalNS := finish.Sub(start).Nanoseconds()
	return netlatency.Result{
		BytesRead: bytesRead,
		Body:      body,
		Trace: netlatency.Trace{
			StartedAt:      start.UTC(),
			TotalNS:        totalNS,
			Transport:      "websocket",
			TTFBNS:         totalNS,
			ResponseWaitNS: totalNS,
		},
	}
}
