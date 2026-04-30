package netlatency

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type WebSocketClient struct {
	url         string
	headers     http.Header
	dialer      *websocket.Dialer
	mu          sync.Mutex
	conn        *websocket.Conn
	readLimit   int64
	readInitial bool
}

func NewWebSocketClient(url string, headers http.Header, readInitial bool) *WebSocketClient {
	return &WebSocketClient{
		url:         url,
		headers:     headers.Clone(),
		dialer:      websocket.DefaultDialer,
		readLimit:   4 << 20,
		readInitial: readInitial,
	}
}

func (c *WebSocketClient) EnsureConnected(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ensureConnectedLocked(ctx)
}

func (c *WebSocketClient) Do(ctx context.Context, message []byte) (Result, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensureConnectedLocked(ctx); err != nil {
		return Result{}, err
	}

	start := time.Now()
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.conn.SetWriteDeadline(deadline)
		_ = c.conn.SetReadDeadline(deadline)
	} else {
		_ = c.conn.SetWriteDeadline(time.Time{})
		_ = c.conn.SetReadDeadline(time.Time{})
	}

	if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
		c.closeLocked()
		finish := time.Now()
		return websocketResult(start, finish, 0, 0), err
	}
	writeDone := time.Now()

	_, data, err := c.conn.ReadMessage()
	finish := time.Now()
	if err != nil {
		c.closeLocked()
		return websocketResult(start, finish, writeDone.Sub(start).Nanoseconds(), 0), err
	}
	if int64(len(data)) > c.readLimit {
		return websocketResult(start, finish, writeDone.Sub(start).Nanoseconds(), int64(len(data)), data),
			fmt.Errorf("websocket response exceeded read limit: %d", len(data))
	}
	return websocketResult(start, finish, writeDone.Sub(start).Nanoseconds(), int64(len(data)), data), nil
}

func (c *WebSocketClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeLocked()
}

func (c *WebSocketClient) ensureConnectedLocked(ctx context.Context) error {
	if c.conn != nil {
		return nil
	}
	conn, _, err := c.dialer.DialContext(ctx, c.url, c.headers)
	if err != nil {
		return err
	}
	if c.readInitial {
		if deadline, ok := ctx.Deadline(); ok {
			_ = conn.SetReadDeadline(deadline)
		}
		if _, _, err := conn.ReadMessage(); err != nil {
			_ = conn.Close()
			return err
		}
		_ = conn.SetReadDeadline(time.Time{})
	}
	c.conn = conn
	return nil
}

func (c *WebSocketClient) closeLocked() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func websocketResult(start time.Time, finish time.Time, writeNS int64, bytesRead int64, body ...[]byte) Result {
	totalNS := finish.Sub(start).Nanoseconds()
	result := Result{
		StatusCode: 0,
		BytesRead:  bytesRead,
		Trace: Trace{
			StartedAt:      start.UTC(),
			TotalNS:        totalNS,
			Transport:      "websocket",
			RequestWriteNS: writeNS,
			TTFBNS:         totalNS,
			ResponseWaitNS: totalNS - writeNS,
		},
	}
	if len(body) > 0 {
		result.Body = body[0]
	}
	return result
}
