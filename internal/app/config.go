package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/secrets"
)

type fileConfig struct {
	Venue       string                 `json:"venue"`
	EnvFiles    []string               `json:"env_files"`
	Benchmark   benchmarkConfig        `json:"benchmark"`
	HTTP        httpConfig             `json:"http"`
	Risk        lifecycle.RiskConfig   `json:"risk"`
	Cleanup     cleanupConfig          `json:"cleanup"`
	Mock        mockConfig             `json:"mock"`
	Request     requestConfig          `json:"request"`
	Venues      map[string]venueConfig `json:"venues"`
	Hyperliquid hyperliquidConfig      `json:"hyperliquid"`
	Lighter     lighterConfig          `json:"lighter"`
}

type benchmarkConfig struct {
	RunID         string  `json:"run_id"`
	Scenario      string  `json:"scenario"`
	Iterations    int     `json:"iterations"`
	Warmups       int     `json:"warmups"`
	BatchSize     int     `json:"batch_size"`
	RatePerSecond float64 `json:"rate_per_second"`
	MaxInFlight   int     `json:"max_in_flight"`
	StopOnError   bool    `json:"stop_on_error"`
	LatencyMode   string  `json:"latency_mode"`
}

type httpConfig struct {
	TimeoutMS           int  `json:"timeout_ms"`
	MaxIdleConns        int  `json:"max_idle_conns"`
	MaxIdleConnsPerHost int  `json:"max_idle_conns_per_host"`
	DisableCompression  bool `json:"disable_compression"`
}

type cleanupConfig struct {
	Enabled   bool   `json:"enabled"`
	Mode      string `json:"mode"`
	Scope     string `json:"scope"`
	TimeoutMS int    `json:"timeout_ms"`
}

type mockConfig struct {
	LatencyMS int    `json:"latency_ms"`
	Status    int    `json:"status"`
	Body      string `json:"body"`
}

type requestConfig struct {
	Transport       string            `json:"transport"`
	Method          string            `json:"method"`
	URL             string            `json:"url"`
	BatchURL        string            `json:"batch_url"`
	WSURL           string            `json:"ws_url"`
	WSBatchURL      string            `json:"ws_batch_url"`
	Headers         map[string]string `json:"headers"`
	Body            string            `json:"body"`
	BodyFile        string            `json:"body_file"`
	BatchBody       string            `json:"batch_body"`
	BatchBodyFile   string            `json:"batch_body_file"`
	WSBody          string            `json:"ws_body"`
	WSBodyFile      string            `json:"ws_body_file"`
	WSBatchBody     string            `json:"ws_batch_body"`
	WSBatchBodyFile string            `json:"ws_batch_body_file"`
	Builder         builderConfig     `json:"builder"`
}

type builderConfig struct {
	Type      string            `json:"type"`
	Command   []string          `json:"command"`
	Env       map[string]string `json:"env"`
	TimeoutMS int               `json:"timeout_ms"`
	Directory string            `json:"directory"`
	Params    map[string]any    `json:"params"`
}

type venueConfig struct {
	BaseURL string        `json:"base_url"`
	WSURL   string        `json:"ws_url"`
	Request requestConfig `json:"request"`
}

type hyperliquidConfig struct {
	BaseURL string        `json:"base_url"`
	WSURL   string        `json:"ws_url"`
	Request requestConfig `json:"request"`
}

type lighterConfig struct {
	BaseURL        string        `json:"base_url"`
	WSURL          string        `json:"ws_url"`
	SendTxURL      string        `json:"send_tx_url"`
	SendTxBatchURL string        `json:"send_tx_batch_url"`
	Request        requestConfig `json:"request"`
}

func loadFileConfig(path string) (fileConfig, error) {
	if path == "" {
		return fileConfig{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fileConfig{}, err
	}
	var cfg fileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fileConfig{}, err
	}
	normalizeFileConfig(&cfg)
	return cfg, nil
}

func normalizeFileConfig(cfg *fileConfig) {
	if cfg == nil {
		return
	}
	mergeLegacyVenueConfig(cfg, "hyperliquid", venueConfig{
		BaseURL: cfg.Hyperliquid.BaseURL,
		WSURL:   cfg.Hyperliquid.WSURL,
		Request: cfg.Hyperliquid.Request,
	})
	mergeLegacyLighterConfig(cfg)
	cfg.Hyperliquid = hyperliquidConfig{}
	cfg.Lighter = lighterConfig{}
}

func mergeLegacyVenueConfig(cfg *fileConfig, name string, legacy venueConfig) {
	if legacy.BaseURL == "" && legacy.WSURL == "" && requestIsZero(legacy.Request) {
		return
	}
	if cfg.Venues == nil {
		cfg.Venues = make(map[string]venueConfig)
	}
	current := cfg.Venues[name]
	if legacy.BaseURL != "" {
		current.BaseURL = legacy.BaseURL
	}
	if legacy.WSURL != "" {
		current.WSURL = legacy.WSURL
	}
	current.Request = mergeRequest(current.Request, legacy.Request)
	cfg.Venues[name] = current
}

func mergeLegacyLighterConfig(cfg *fileConfig) {
	if cfg.Lighter.BaseURL == "" &&
		cfg.Lighter.WSURL == "" &&
		cfg.Lighter.SendTxURL == "" &&
		cfg.Lighter.SendTxBatchURL == "" &&
		requestIsZero(cfg.Lighter.Request) {
		return
	}
	if cfg.Venues == nil {
		cfg.Venues = make(map[string]venueConfig)
	}
	current := cfg.Venues["lighter"]
	if cfg.Lighter.BaseURL != "" {
		current.BaseURL = cfg.Lighter.BaseURL
	}
	if cfg.Lighter.WSURL != "" {
		current.WSURL = cfg.Lighter.WSURL
	}
	if current.Request.URL == "" {
		current.Request.URL = cfg.Lighter.SendTxURL
	}
	if current.Request.BatchURL == "" {
		current.Request.BatchURL = cfg.Lighter.SendTxBatchURL
	}
	current.Request = mergeRequest(current.Request, cfg.Lighter.Request)
	cfg.Venues["lighter"] = current
}

func requestIsZero(req requestConfig) bool {
	return req.Transport == "" &&
		req.Method == "" &&
		req.URL == "" &&
		req.BatchURL == "" &&
		req.WSURL == "" &&
		req.WSBatchURL == "" &&
		len(req.Headers) == 0 &&
		req.Body == "" &&
		req.BodyFile == "" &&
		req.BatchBody == "" &&
		req.BatchBodyFile == "" &&
		req.WSBody == "" &&
		req.WSBodyFile == "" &&
		req.WSBatchBody == "" &&
		req.WSBatchBodyFile == "" &&
		builderIsZero(req.Builder)
}

func builderIsZero(builder builderConfig) bool {
	return builder.Type == "" &&
		len(builder.Command) == 0 &&
		len(builder.Env) == 0 &&
		builder.TimeoutMS == 0 &&
		builder.Directory == "" &&
		len(builder.Params) == 0
}

func validateNoInlineSecrets(cfg fileConfig) error {
	findings, err := secrets.FindInlineSecrets(cfg)
	if err != nil {
		return err
	}
	if len(findings) == 0 {
		return nil
	}
	paths := make([]string, 0, len(findings))
	for _, finding := range findings {
		paths = append(paths, finding.Path)
	}
	return fmt.Errorf("config contains inline secrets at %s; move credentials to env files or shell environment, or rerun with --allow-inline-secrets for local debugging", strings.Join(paths, ", "))
}

func (c benchmarkConfig) toBenchConfig() bench.Config {
	return bench.Config{
		RunID:         c.RunID,
		Scenario:      bench.Scenario(c.Scenario),
		Iterations:    c.Iterations,
		Warmups:       c.Warmups,
		BatchSize:     c.BatchSize,
		RatePerSecond: c.RatePerSecond,
		MaxInFlight:   c.MaxInFlight,
		StopOnError:   c.StopOnError,
		LatencyMode:   bench.LatencyMode(c.LatencyMode),
		Cleanup: bench.CleanupConfig{
			Enabled:   false,
			Mode:      bench.CleanupModeOff,
			Scope:     bench.CleanupScopeAfterSample,
			TimeoutMS: 0,
		},
	}.Normalized()
}

func (c cleanupConfig) toBenchCleanupConfig() bench.CleanupConfig {
	return bench.CleanupConfig{
		Enabled:   c.Enabled,
		Mode:      bench.CleanupMode(c.Mode),
		Scope:     bench.CleanupScope(c.Scope),
		TimeoutMS: c.TimeoutMS,
	}.Normalized()
}
