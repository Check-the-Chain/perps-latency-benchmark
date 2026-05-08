package netlatency

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gorilla/websocket"
)

type WebSocketClient struct {
	url             string
	headers         http.Header
	dialer          *websocket.Dialer
	mu              sync.Mutex
	conn            *websocket.Conn
	coderConn       *coderws.Conn
	readLimit       int64
	readInitial     bool
	heartbeat       WebSocketHeartbeat
	heartbeatCancel context.CancelFunc
	lastIO          time.Time
}

type WebSocketHeartbeat struct {
	Message      []byte
	ControlFrame string
	IdleAfter    time.Duration
	Timeout      time.Duration
	ObserveRTT   func(valueNS int64, source string)
}

func NewWebSocketClient(url string, headers http.Header, readInitial bool) *WebSocketClient {
	return NewWebSocketClientWithHeartbeat(url, headers, readInitial, WebSocketHeartbeat{})
}

func NewWebSocketClientWithHeartbeat(url string, headers http.Header, readInitial bool, heartbeat WebSocketHeartbeat) *WebSocketClient {
	return &WebSocketClient{
		url:         url,
		headers:     headers.Clone(),
		dialer:      websocket.DefaultDialer,
		readLimit:   4 << 20,
		readInitial: readInitial,
		heartbeat:   heartbeat.normalized(),
	}
}

func (c *WebSocketClient) EnsureConnected(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ensureReadyLocked(ctx)
}

func (c *WebSocketClient) Do(ctx context.Context, message []byte) (Result, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensureReadyLocked(ctx); err != nil {
		return Result{}, err
	}

	start := time.Now()
	if c.coderConn != nil {
		if err := c.coderConn.Write(ctx, coderws.MessageText, message); err != nil {
			c.closeLocked()
			finish := time.Now()
			return websocketResult(start, finish, 0, 0), err
		}
	} else {
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
	}
	writeDone := time.Now()
	c.lastIO = writeDone

	data, err := c.readMessage(ctx)
	finish := time.Now()
	if err != nil {
		c.closeLocked()
		return websocketResult(start, finish, writeDone.Sub(start).Nanoseconds(), 0), err
	}
	if int64(len(data)) > c.readLimit {
		return websocketResult(start, finish, writeDone.Sub(start).Nanoseconds(), int64(len(data)), data),
			fmt.Errorf("websocket response exceeded read limit: %d", len(data))
	}
	c.lastIO = finish
	return websocketResult(start, finish, writeDone.Sub(start).Nanoseconds(), int64(len(data)), data), nil
}

func (c *WebSocketClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeLocked()
}

func (c *WebSocketClient) ensureConnectedLocked(ctx context.Context) error {
	if c.conn != nil || c.coderConn != nil {
		return nil
	}
	conn, _, err := c.dialer.DialContext(ctx, c.url, c.headers)
	if err != nil {
		if !isCompressionNegotiationError(err) {
			return err
		}
		return c.ensureCoderConnectedLocked(ctx)
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
	c.lastIO = time.Now()
	c.startControlHeartbeatLocked()
	return nil
}

func (c *WebSocketClient) ensureCoderConnectedLocked(ctx context.Context) error {
	if c.coderConn != nil {
		return nil
	}
	conn, _, err := coderws.Dial(ctx, c.url, &coderws.DialOptions{
		HTTPHeader:      c.headers.Clone(),
		CompressionMode: coderws.CompressionContextTakeover,
	})
	if err != nil {
		return err
	}
	conn.SetReadLimit(c.readLimit)
	if c.readInitial {
		if _, _, err := conn.Read(ctx); err != nil {
			_ = conn.Close(coderws.StatusNormalClosure, "")
			return err
		}
	}
	c.coderConn = conn
	c.lastIO = time.Now()
	c.startControlHeartbeatLocked()
	return nil
}

func (c *WebSocketClient) ensureReadyLocked(ctx context.Context) error {
	if err := c.ensureConnectedLocked(ctx); err != nil {
		return err
	}
	if err := c.heartbeatIfIdleLocked(ctx); err != nil {
		c.closeLocked()
		return c.ensureConnectedLocked(ctx)
	}
	return nil
}

func (c *WebSocketClient) heartbeatIfIdleLocked(ctx context.Context) error {
	if !c.heartbeat.enabled() || c.heartbeat.IdleAfter <= 0 || time.Since(c.lastIO) < c.heartbeat.IdleAfter {
		return nil
	}
	deadline := time.Now().Add(c.heartbeat.Timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if c.heartbeat.controlFrameCode() != 0 {
		return c.sendControlHeartbeatLocked(ctx, deadline)
	}
	start := time.Now()
	if c.coderConn != nil {
		heartbeatCtx, cancel := context.WithDeadline(ctx, deadline)
		defer cancel()
		if err := c.coderConn.Write(heartbeatCtx, coderws.MessageText, c.heartbeat.Message); err != nil {
			return err
		}
		if _, _, err := c.coderConn.Read(heartbeatCtx); err != nil {
			return err
		}
	} else {
		if err := c.conn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		if err := c.conn.SetReadDeadline(deadline); err != nil {
			return err
		}
		if err := c.conn.WriteMessage(websocket.TextMessage, c.heartbeat.Message); err != nil {
			return err
		}
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return err
		}
		_ = c.conn.SetWriteDeadline(time.Time{})
		_ = c.conn.SetReadDeadline(time.Time{})
	}
	finish := time.Now()
	c.lastIO = finish
	if c.heartbeat.ObserveRTT != nil {
		c.heartbeat.ObserveRTT(finish.Sub(start).Nanoseconds(), "ws_heartbeat")
	}
	return nil
}

func (c *WebSocketClient) closeLocked() error {
	var err error
	if c.heartbeatCancel != nil {
		c.heartbeatCancel()
		c.heartbeatCancel = nil
	}
	if c.conn != nil {
		err = c.conn.Close()
		c.conn = nil
	}
	if c.coderConn != nil {
		err = c.coderConn.Close(coderws.StatusNormalClosure, "")
		c.coderConn = nil
	}
	c.lastIO = time.Time{}
	return err
}

func (c *WebSocketClient) startControlHeartbeatLocked() {
	if c.heartbeat.controlFrameCode() == 0 || c.heartbeat.IdleAfter <= 0 || c.heartbeatCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.heartbeatCancel = cancel
	interval := c.heartbeat.IdleAfter
	go c.controlHeartbeatLoop(ctx, interval)
}

func (c *WebSocketClient) controlHeartbeatLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			if ctx.Err() != nil {
				c.mu.Unlock()
				return
			}
			if err := c.controlHeartbeatIfIdleLocked(ctx); err != nil {
				c.closeLocked()
			}
			c.mu.Unlock()
		}
	}
}

func (c *WebSocketClient) controlHeartbeatIfIdleLocked(ctx context.Context) error {
	if c.heartbeat.controlFrameCode() == 0 || c.heartbeat.IdleAfter <= 0 || time.Since(c.lastIO) < c.heartbeat.IdleAfter {
		return nil
	}
	deadline := time.Now().Add(c.heartbeat.Timeout)
	return c.sendControlHeartbeatLocked(ctx, deadline)
}

func (c *WebSocketClient) sendControlHeartbeatLocked(ctx context.Context, deadline time.Time) error {
	if c.conn == nil && c.coderConn == nil {
		return nil
	}
	start := time.Now()
	if c.coderConn != nil {
		pingCtx, cancel := context.WithDeadline(ctx, deadline)
		err := c.coderConn.Ping(pingCtx)
		cancel()
		if err != nil {
			return err
		}
	} else if err := c.conn.WriteControl(c.heartbeat.controlFrameCode(), nil, deadline); err != nil {
		return err
	}
	finish := time.Now()
	c.lastIO = finish
	if c.heartbeat.ObserveRTT != nil {
		c.heartbeat.ObserveRTT(finish.Sub(start).Nanoseconds(), "ws_control_"+c.heartbeat.ControlFrame)
	}
	return nil
}

func (c *WebSocketClient) readMessage(ctx context.Context) ([]byte, error) {
	if c.coderConn != nil {
		_, data, err := c.coderConn.Read(ctx)
		return data, err
	}
	_, data, err := c.conn.ReadMessage()
	return data, err
}

func isCompressionNegotiationError(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "invalid compression negotiation")
}

func (h WebSocketHeartbeat) normalized() WebSocketHeartbeat {
	h.ControlFrame = strings.ToLower(strings.TrimSpace(h.ControlFrame))
	if !h.enabled() {
		return WebSocketHeartbeat{}
	}
	if h.Timeout <= 0 {
		h.Timeout = 5 * time.Second
	}
	return h
}

func (h WebSocketHeartbeat) enabled() bool {
	return len(h.Message) > 0 || h.controlFrameCode() != 0
}

func (h WebSocketHeartbeat) controlFrameCode() int {
	switch strings.ToLower(strings.TrimSpace(h.ControlFrame)) {
	case "ping":
		return websocket.PingMessage
	case "pong":
		return websocket.PongMessage
	default:
		return 0
	}
}

func websocketResult(start time.Time, finish time.Time, writeNS int64, bytesRead int64, body ...[]byte) Result {
	totalNS := finish.Sub(start).Nanoseconds()
	result := Result{
		StatusCode: 0,
		BytesRead:  bytesRead,
		Trace: Trace{
			StartedAt:        start.UTC(),
			TotalNS:          totalNS,
			Transport:        "websocket",
			RequestWriteNS:   writeNS,
			TTFBNS:           totalNS,
			ResponseWaitNS:   totalNS - writeNS,
			WroteRequestAtNS: writeNS,
		},
	}
	if len(body) > 0 {
		result.Body = body[0]
	}
	return result
}
