package confirmws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	coderws "github.com/coder/websocket"
	gorillaws "github.com/gorilla/websocket"

	"perps-latency-benchmark/internal/netlatency"
)

type Client struct {
	conn      *gorillaws.Conn
	coderConn *coderws.Conn
	readLimit int64
	writeMu   sync.Mutex
	pingStop  chan struct{}
	closeOnce sync.Once
}

type DialOptions struct {
	EnableCompression          bool
	CompressionContextTakeover bool
}

func Dial(ctx context.Context, url string, headers http.Header, readInitial bool) (*Client, error) {
	return DialWithOptions(ctx, url, headers, readInitial, DialOptions{})
}

func DialWithOptions(ctx context.Context, url string, headers http.Header, readInitial bool, opts DialOptions) (*Client, error) {
	if opts.CompressionContextTakeover {
		return dialCoder(ctx, url, headers, readInitial, opts)
	}
	dialer := *gorillaws.DefaultDialer
	dialer.EnableCompression = opts.EnableCompression
	conn, _, err := dialer.DialContext(ctx, url, headers)
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

func dialCoder(ctx context.Context, url string, headers http.Header, readInitial bool, opts DialOptions) (*Client, error) {
	dialOpts := &coderws.DialOptions{
		HTTPHeader: headers.Clone(),
	}
	if opts.CompressionContextTakeover {
		dialOpts.CompressionMode = coderws.CompressionContextTakeover
	} else if opts.EnableCompression {
		dialOpts.CompressionMode = coderws.CompressionNoContextTakeover
	}
	conn, _, err := coderws.Dial(ctx, url, dialOpts)
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(4 << 20)
	client := &Client{coderConn: conn, readLimit: 4 << 20}
	if readInitial {
		if _, _, err := client.readRaw(ctx); err != nil {
			_ = client.Close()
			return nil, err
		}
	}
	return client, nil
}

func (c *Client) Close() error {
	if c == nil || (c.conn == nil && c.coderConn == nil) {
		return nil
	}
	c.closeOnce.Do(func() {
		if c.pingStop != nil {
			close(c.pingStop)
		}
	})
	var err error
	if c.conn != nil {
		err = c.conn.Close()
		c.conn = nil
	}
	if c.coderConn != nil {
		err = c.coderConn.Close(coderws.StatusNormalClosure, "")
		c.coderConn = nil
	}
	return err
}

func (c *Client) WriteJSON(ctx context.Context, value any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.coderConn != nil {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return c.coderConn.Write(ctx, coderws.MessageText, data)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.conn.SetWriteDeadline(deadline)
	} else {
		_ = c.conn.SetWriteDeadline(time.Time{})
	}
	return c.conn.WriteJSON(value)
}

func (c *Client) StartPingFrames(interval time.Duration, timeout time.Duration) {
	if c == nil || (c.conn == nil && c.coderConn == nil) || interval <= 0 {
		return
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if c.pingStop != nil {
		return
	}
	stop := make(chan struct{})
	c.pingStop = stop
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				deadline := time.Now().Add(timeout)
				var err error
				if c.coderConn != nil {
					ctx, cancel := context.WithDeadline(context.Background(), deadline)
					err = c.coderConn.Ping(ctx)
					cancel()
				} else {
					c.writeMu.Lock()
					err = c.conn.WriteControl(gorillaws.PingMessage, nil, deadline)
					c.writeMu.Unlock()
				}
				if err != nil {
					_ = c.Close()
					return
				}
			case <-stop:
				return
			}
		}
	}()
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
	if c.coderConn != nil {
		_, data, err := c.coderConn.Read(ctx)
		return data, time.Now(), err
	}
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
