package spec

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"strconv"
	"strings"
	"time"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/booktop"
	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/names"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/payload"
	"perps-latency-benchmark/internal/venues/prebuilt"
)

type Capabilities struct {
	HTTPSingle      bool
	HTTPBatch       bool
	WebSocketSingle bool
	WebSocketBatch  bool
	Cleanup         bool
	Neutralization  bool
}

type BuilderParams struct {
	Required []string
	Defaults map[string]any
}

type CleanupCommand struct {
	Type           string
	Command        []string
	Description    string
	OrderRefsField string
	SkipNoRefs     bool
}

type CostCommand struct {
	Type    string
	Command []string
	Timeout time.Duration
}

type RateLimitCommand struct {
	Command []string
	Timeout time.Duration
	Decode  func([]byte) (RateLimitStatus, error)
}

type RateLimitStatus struct {
	RequestsUsed    int
	RequestsCap     int
	RequestsSurplus int
}

type RuntimeConfig struct {
	BaseURL string
	WSURL   string
	Params  map[string]any
}

type BookTop struct {
	Build func(RuntimeConfig) (booktop.Config, bool)
}

type ExpectedFill struct {
	Build func(RuntimeConfig) (ExpectedFillOrder, bool)
}

type ExpectedFillOrder struct {
	Side string
	Size float64
}

type RunLock struct {
	Build func(RuntimeConfig) (RunLockTarget, bool)
}

type RunLockTarget struct {
	Name        string
	BusyMessage string
}

type Definition struct {
	Name               string
	Aliases            []string
	DefaultBaseURL     string
	DefaultHTTPPath    string
	DefaultBatchPath   string
	DefaultHTTPURL     string
	DefaultBatchURL    string
	DefaultWSURL       string
	DefaultWSBatchURL  string
	WSReadInitial      bool
	WSHeartbeat        WebSocketHeartbeat
	Capabilities       Capabilities
	BuilderParams      BuilderParams
	CleanupCommand     CleanupCommand
	CostCommand        CostCommand
	RateLimitCommand   RateLimitCommand
	BookTop            BookTop
	ExpectedFill       ExpectedFill
	RunLock            RunLock
	Classifier         lifecycle.Classifier
	Confirmation       prebuilt.ConfirmationFactory
	CancelConfirmation func(context.Context, payload.Built) (*bench.Confirmation, error)
	Docs               []string
	Notes              []string
}

type WebSocketHeartbeat struct {
	Message      string
	ControlFrame string
	IdleAfter    time.Duration
	Timeout      time.Duration
}

type Config struct {
	BaseURL string
	WSURL   string
	Request prebuilt.Config
}

func (d Definition) Build(cfg Config) (bench.Venue, error) {
	if d.Name == "" {
		return nil, fmt.Errorf("venue definition missing name")
	}

	req := cfg.Request
	req.Name = d.Name
	req.BuilderParams = d.BuilderParams.Merge(req.BuilderParams)
	if req.URL == "" {
		req.URL = d.httpURL(cfg.BaseURL)
	}
	if req.BatchURL == "" {
		req.BatchURL = d.batchURL(cfg.BaseURL)
	}
	if req.WSURL == "" {
		req.WSURL = cmp.Or(cfg.WSURL, d.DefaultWSURL)
	}
	if req.WSBatchURL == "" {
		req.WSBatchURL = cmp.Or(d.DefaultWSBatchURL, req.WSURL)
	}
	req.WSReadInitial = d.WSReadInitial
	req.WSHeartbeat = d.WSHeartbeat.toNetLatency()
	req.Classifier = d.Classifier
	req.Confirmation = d.Confirmation

	return prebuilt.New(req)
}

func (h WebSocketHeartbeat) toNetLatency() netlatency.WebSocketHeartbeat {
	return netlatency.WebSocketHeartbeat{
		Message:      []byte(h.Message),
		ControlFrame: h.ControlFrame,
		IdleAfter:    h.IdleAfter,
		Timeout:      h.Timeout,
	}
}

func (p BuilderParams) Merge(params map[string]any) map[string]any {
	if len(p.Defaults) == 0 {
		return params
	}
	merged := maps.Clone(p.Defaults)
	for key, value := range params {
		merged[key] = value
	}
	return merged
}

func (d Definition) Names() []string {
	normalized := []string{names.Normalize(d.Name)}
	for _, alias := range d.Aliases {
		normalized = append(normalized, names.Normalize(alias))
	}
	return normalized
}

func (d Definition) HTTPURL(baseURL string) string {
	return d.httpURL(baseURL)
}

func (d Definition) BatchURL(baseURL string) string {
	return d.batchURL(baseURL)
}

func (d Definition) BookTopConfig(runtime RuntimeConfig) (booktop.Config, bool) {
	if d.BookTop.Build == nil {
		return booktop.Config{}, false
	}
	cfg, ok := d.BookTop.Build(runtime)
	if !ok {
		return booktop.Config{}, false
	}
	cfg.Venue = d.Name
	return cfg, true
}

func (d Definition) ExpectedFillOrder(runtime RuntimeConfig) (ExpectedFillOrder, bool) {
	if d.ExpectedFill.Build == nil {
		return ExpectedFillOrder{}, false
	}
	order, ok := d.ExpectedFill.Build(runtime)
	if !ok {
		return ExpectedFillOrder{}, false
	}
	order.Side = strings.ToLower(strings.TrimSpace(order.Side))
	return order, true
}

func (d Definition) RunLockTarget(runtime RuntimeConfig) (RunLockTarget, bool) {
	if d.RunLock.Build == nil {
		return RunLockTarget{}, false
	}
	return d.RunLock.Build(runtime)
}

func (d Definition) httpURL(baseURL string) string {
	if d.DefaultHTTPURL != "" {
		return d.DefaultHTTPURL
	}
	return joinURL(cmp.Or(baseURL, d.DefaultBaseURL), d.DefaultHTTPPath)
}

func TextParam(params map[string]any, key string, fallback string) string {
	if params == nil {
		return fallback
	}
	if value, ok := params[key]; ok && value != nil {
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" {
			return text
		}
	}
	return fallback
}

func FloatParam(params map[string]any, key string) float64 {
	if params == nil {
		return 0
	}
	value, ok := params[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	case string:
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed
	default:
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(typed)), 64)
		return parsed
	}
}

func CoalesceURL(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimRight(value, "/")
		}
	}
	return ""
}

func (d Definition) batchURL(baseURL string) string {
	if d.DefaultBatchURL != "" {
		return d.DefaultBatchURL
	}
	if d.DefaultBatchPath == "" {
		return ""
	}
	return joinURL(cmp.Or(baseURL, d.DefaultBaseURL), d.DefaultBatchPath)
}

func (d Definition) Supports(transport string, scenario bench.Scenario) bool {
	capabilities := d.Capabilities
	if transport == "" || transport == "https" {
		transport = "http"
	}
	switch transport {
	case "http":
		if scenario == bench.ScenarioBatch {
			return capabilities.HTTPBatch
		}
		return capabilities.HTTPSingle
	case "websocket":
		if scenario == bench.ScenarioBatch {
			return capabilities.WebSocketBatch
		}
		return capabilities.WebSocketSingle
	default:
		return false
	}
}

func joinURL(base string, path string) string {
	if base == "" || path == "" {
		return ""
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + strings.TrimLeft(path, "/")
	return parsed.String()
}
