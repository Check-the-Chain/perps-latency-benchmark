package netlatency

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"sync"
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

type parallelOutcome struct {
	index  int
	result Result
	err    error
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
		IdleConnTimeout:       10 * time.Minute,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    cfg.DisableCompression,
	}

	return &Client{
		httpClient: &http.Client{Transport: transport, Timeout: timeout},
		readLimit:  4 << 20,
	}
}

func (c *Client) WarmHost(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return err
	}
	warmURL := parsed.Scheme + "://" + parsed.Host + "/"
	warmCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(warmCtx, http.MethodGet, warmURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return nil
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

func (c *Client) DoParallelFastest(ctx context.Context, templates []RequestTemplate) (Result, error) {
	if len(templates) == 0 {
		return Result{}, errors.New("parallel HTTP execution requires at least one request")
	}
	if len(templates) == 1 {
		return c.Do(ctx, templates[0])
	}

	var ready sync.WaitGroup
	ready.Add(len(templates))
	start := make(chan struct{})
	outcomes := make(chan parallelOutcome, len(templates))
	for index, template := range templates {
		go func() {
			ready.Done()
			<-start
			result, err := c.Do(ctx, template)
			outcomes <- parallelOutcome{index: index, result: result, err: err}
		}()
	}
	ready.Wait()
	close(start)

	collected := make([]parallelOutcome, 0, len(templates))
	for range templates {
		collected = append(collected, <-outcomes)
	}
	return fastestOutcome(collected)
}

func fastestOutcome(outcomes []parallelOutcome) (Result, error) {
	var best *parallelOutcome
	for index := range outcomes {
		current := &outcomes[index]
		if current.err != nil && hasSuccessfulOutcome(outcomes) {
			continue
		}
		if best == nil || finishAt(current.result.Trace).Before(finishAt(best.result.Trace)) {
			best = current
		}
	}
	if best == nil {
		return Result{}, errors.New("parallel HTTP execution produced no result")
	}
	return best.result, best.err
}

func hasSuccessfulOutcome(outcomes []parallelOutcome) bool {
	for _, outcome := range outcomes {
		if outcome.err == nil {
			return true
		}
	}
	return false
}

func finishAt(trace Trace) time.Time {
	if trace.StartedAt.IsZero() {
		return time.Now()
	}
	return trace.StartedAt.Add(time.Duration(trace.TotalNS))
}

func (c *Client) CloseIdleConnections() {
	c.httpClient.CloseIdleConnections()
}
