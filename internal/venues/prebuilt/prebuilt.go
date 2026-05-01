package prebuilt

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/payload"
)

type Config struct {
	Name            string
	Transport       string
	Method          string
	URL             string
	BatchURL        string
	WSURL           string
	WSBatchURL      string
	Headers         map[string]string
	Body            string
	BodyFile        string
	BatchBody       string
	BatchBodyFile   string
	WSBody          string
	WSBodyFile      string
	WSBatchBody     string
	WSBatchBodyFile string
	WSReadInitial   bool
	WSHeartbeat     netlatency.WebSocketHeartbeat
	Builder         payload.Builder
	BuilderParams   map[string]any
	Classifier      lifecycle.Classifier
	Confirmation    ConfirmationFactory
}

type ConfirmationFactory func(context.Context, payload.Built) (*bench.Confirmation, error)

type Venue struct {
	name          string
	method        string
	url           string
	batchURL      string
	wsURL         string
	wsBatchURL    string
	headers       http.Header
	body          []byte
	batchBody     []byte
	wsBody        []byte
	wsBatchBody   []byte
	transport     string
	builder       payload.Builder
	builderParams map[string]any
	classifier    lifecycle.Classifier
	confirmation  ConfirmationFactory
	wsClient      *netlatency.WebSocketClient
	wsBatchClient *netlatency.WebSocketClient
}

func New(cfg Config) (*Venue, error) {
	transport := strings.ToLower(cfg.Transport)
	if transport == "" {
		transport = "http"
	}
	if transport == "https" {
		transport = "http"
	}
	if transport != "http" && transport != "websocket" {
		return nil, fmt.Errorf("unsupported transport %q", cfg.Transport)
	}
	if transport == "http" && cfg.URL == "" {
		return nil, fmt.Errorf("request url is required for http transport")
	}
	if transport == "websocket" && cfg.WSURL == "" {
		return nil, fmt.Errorf("ws_url is required for websocket transport")
	}
	body, err := bodyBytes(cfg.Body, cfg.BodyFile)
	if err != nil {
		return nil, err
	}
	batchBody, err := bodyBytes(cfg.BatchBody, cfg.BatchBodyFile)
	if err != nil {
		return nil, err
	}
	if len(batchBody) == 0 {
		batchBody = body
	}
	wsBody, err := bodyBytes(cfg.WSBody, cfg.WSBodyFile)
	if err != nil {
		return nil, err
	}
	if len(wsBody) == 0 {
		wsBody = body
	}
	wsBatchBody, err := bodyBytes(cfg.WSBatchBody, cfg.WSBatchBodyFile)
	if err != nil {
		return nil, err
	}
	if len(wsBatchBody) == 0 {
		wsBatchBody = wsBody
	}
	if cfg.Builder == nil {
		if transport == "http" && len(body) == 0 {
			return nil, fmt.Errorf("request body or body_file is required")
		}
		if transport == "websocket" && len(wsBody) == 0 {
			return nil, fmt.Errorf("request ws_body, ws_body_file, body, or body_file is required")
		}
	}

	name := cfg.Name
	if name == "" {
		name = "http"
	}
	method := cfg.Method
	if method == "" {
		method = http.MethodPost
	}

	headers := make(http.Header)
	for key, value := range cfg.Headers {
		headers.Set(key, value)
	}
	if headers.Get("Content-Type") == "" {
		headers.Set("Content-Type", "application/json")
	}

	return &Venue{
		name:          name,
		method:        method,
		url:           cfg.URL,
		batchURL:      cfg.BatchURL,
		wsURL:         cfg.WSURL,
		wsBatchURL:    cfg.WSBatchURL,
		headers:       headers,
		body:          body,
		batchBody:     batchBody,
		wsBody:        wsBody,
		wsBatchBody:   wsBatchBody,
		transport:     transport,
		builder:       cfg.Builder,
		builderParams: cfg.BuilderParams,
		classifier:    cfg.Classifier,
		confirmation:  cfg.Confirmation,
		wsClient:      newWSClientWithHeartbeat(transport, cfg.WSURL, headers, cfg.WSReadInitial, cfg.WSHeartbeat),
		wsBatchClient: newWSClientWithHeartbeat(transport, cmp.Or(cfg.WSBatchURL, cfg.WSURL), headers, cfg.WSReadInitial, cfg.WSHeartbeat),
	}, nil
}

func (v *Venue) Name() string {
	return v.name
}

func (v *Venue) Prepare(ctx context.Context, scenario bench.Scenario, iteration int, batchSize int) (bench.PreparedRequest, error) {
	built, err := v.build(ctx, scenario, iteration, batchSize)
	if err != nil {
		return bench.PreparedRequest{}, err
	}
	if v.transport == "websocket" {
		return v.prepareWebSocket(ctx, scenario, iteration, batchSize, built)
	}

	targetURL := payload.FirstNonEmpty(built.URL, v.url)
	bodyText := built.Body
	bodyBase64 := built.BodyBase64
	fallbackBody := v.body
	if scenario == bench.ScenarioBatch {
		bodyText = firstPointer(built.BatchBody, built.Body)
		bodyBase64 = payload.FirstNonEmpty(built.BatchBodyBase64, built.BodyBase64)
		fallbackBody = firstBytes(v.batchBody, v.body)
		if batchURL := payload.FirstNonEmpty(built.BatchURL, v.batchURL); batchURL != "" {
			targetURL = batchURL
		}
	}
	body, err := payload.Bytes(bodyText, bodyBase64, fallbackBody)
	if err != nil {
		return bench.PreparedRequest{}, err
	}
	confirmation, err := v.prepareConfirmation(ctx, built)
	if err != nil {
		return bench.PreparedRequest{}, err
	}
	return bench.PreparedRequest{
		Transport: httpTransportLabel(targetURL),
		Request: netlatency.RequestTemplate{
			Method: payload.FirstNonEmpty(built.Method, v.method),
			URL:    targetURL,
			Header: payload.MergeHeaders(v.headers, built.Headers),
			Body:   body,
		},
		Classifier: v.classifier,
		Confirm:    confirmation,
		Metadata: mergeMetadata(built.Metadata, map[string]any{
			"iteration":  iteration,
			"batch_size": batchSize,
			"prebuilt":   true,
		}),
	}, nil
}

func (v *Venue) prepareWebSocket(ctx context.Context, scenario bench.Scenario, iteration int, batchSize int, built payload.Built) (bench.PreparedRequest, error) {
	bodyText := firstPointer(built.WSBody, built.Body)
	bodyBase64 := payload.FirstNonEmpty(built.WSBodyBase64, built.BodyBase64)
	fallbackBody := firstBytes(v.wsBody, v.body)
	client := v.wsClient
	if scenario == bench.ScenarioBatch {
		bodyText = firstPointer(built.WSBatchBody, built.BatchBody, built.WSBody, built.Body)
		bodyBase64 = payload.FirstNonEmpty(built.WSBatchBodyBase64, built.BatchBodyBase64, built.WSBodyBase64, built.BodyBase64)
		fallbackBody = firstBytes(v.wsBatchBody, v.batchBody, v.wsBody, v.body)
		client = v.wsBatchClient
	}
	body, err := payload.Bytes(bodyText, bodyBase64, fallbackBody)
	if err != nil {
		return bench.PreparedRequest{}, err
	}
	if err := client.EnsureConnected(ctx); err != nil {
		return bench.PreparedRequest{}, err
	}
	confirmation, err := v.prepareConfirmation(ctx, built)
	if err != nil {
		return bench.PreparedRequest{}, err
	}
	return bench.PreparedRequest{
		Transport: "websocket",
		Execute: func(ctx context.Context) (netlatency.Result, error) {
			return client.Do(ctx, body)
		},
		Classifier: v.classifier,
		Confirm:    confirmation,
		Metadata: mergeMetadata(built.Metadata, map[string]any{
			"iteration":  iteration,
			"batch_size": batchSize,
			"prebuilt":   true,
		}),
	}, nil
}

func (v *Venue) prepareConfirmation(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	if v.confirmation == nil {
		return nil, nil
	}
	return v.confirmation(ctx, built)
}

func (v *Venue) Close(ctx context.Context) error {
	var err error
	if closer, ok := v.builder.(payload.Closer); ok {
		err = closer.Close(ctx)
	}
	if v.wsClient != nil {
		if closeErr := v.wsClient.Close(); err == nil {
			err = closeErr
		}
	}
	if v.wsBatchClient != nil && v.wsBatchClient != v.wsClient {
		if closeErr := v.wsBatchClient.Close(); err == nil {
			err = closeErr
		}
	}
	return err
}

func (v *Venue) build(ctx context.Context, scenario bench.Scenario, iteration int, batchSize int) (payload.Built, error) {
	if v.builder == nil {
		return payload.Built{}, nil
	}
	return v.builder.Build(ctx, payload.Request{
		Venue:       v.name,
		Transport:   v.transport,
		Scenario:    scenario,
		Iteration:   iteration,
		BatchSize:   batchSize,
		RequestedAt: time.Now().UTC(),
		Params:      v.builderParams,
	})
}

func bodyBytes(inline string, file string) ([]byte, error) {
	if inline != "" {
		return []byte(inline), nil
	}
	if file == "" {
		return nil, nil
	}
	return os.ReadFile(file)
}

func firstPointer(values ...*string) *string {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstBytes(values ...[]byte) []byte {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func httpTransportLabel(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err == nil && parsed.Scheme == "https" {
		return "https"
	}
	return "http"
}

func newWSClientWithHeartbeat(transport string, url string, headers http.Header, readInitial bool, heartbeat netlatency.WebSocketHeartbeat) *netlatency.WebSocketClient {
	if transport != "websocket" || url == "" {
		return nil
	}
	return netlatency.NewWebSocketClientWithHeartbeat(url, headers, readInitial, heartbeat)
}

func mergeMetadata(primary map[string]any, fallback map[string]any) map[string]any {
	merged := make(map[string]any, len(primary)+len(fallback))
	for key, value := range fallback {
		merged[key] = value
	}
	for key, value := range primary {
		merged[key] = value
	}
	return merged
}
