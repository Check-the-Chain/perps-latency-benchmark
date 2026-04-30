package netlatency

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"time"
)

type ClientConfig struct {
	Timeout             time.Duration
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	DisableCompression  bool
}

type Client struct {
	httpClient *http.Client
	readLimit  int64
}

type Result struct {
	StatusCode int    `json:"status_code"`
	BytesRead  int64  `json:"bytes_read"`
	Trace      Trace  `json:"trace"`
	Body       []byte `json:"-"`
}

func NewClient(cfg ClientConfig) *Client {
	maxIdle := cfg.MaxIdleConns
	if maxIdle == 0 {
		maxIdle = 256
	}
	maxIdlePerHost := cfg.MaxIdleConnsPerHost
	if maxIdlePerHost == 0 {
		maxIdlePerHost = 256
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          maxIdle,
		MaxIdleConnsPerHost:   maxIdlePerHost,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    cfg.DisableCompression,
	}

	return &Client{
		httpClient: &http.Client{Transport: transport, Timeout: timeout},
		readLimit:  4 << 20,
	}
}

func (c *Client) Do(ctx context.Context, template RequestTemplate) (Result, error) {
	req, err := template.NewRequest(ctx)
	if err != nil {
		start := time.Now()
		recorder := newTraceRecorder(start)
		recorder.markFinish()
		return Result{Trace: recorder.snapshot()}, err
	}

	start := time.Now()
	recorder := newTraceRecorder(start)
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), recorder.clientTrace()))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		recorder.markFinish()
		return Result{Trace: recorder.snapshot()}, err
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, c.readLimit))
	recorder.markBodyReadDone()
	recorder.markFinish()

	result := Result{
		StatusCode: resp.StatusCode,
		BytesRead:  int64(len(body)),
		Trace:      recorder.snapshot(),
		Body:       body,
	}
	return result, readErr
}

func (c *Client) CloseIdleConnections() {
	c.httpClient.CloseIdleConnections()
}
