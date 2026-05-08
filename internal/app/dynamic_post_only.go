package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"perps-latency-benchmark/internal/booktop"
	"perps-latency-benchmark/internal/payload"
	"perps-latency-benchmark/internal/venues/spec"
)

const (
	defaultPostOnlyBookWaitMS    = 5000
	defaultPostOnlyBookMaxAgeMS  = 2000
	defaultPostOnlyOffsetBPS     = 500
	defaultPostOnlyHTTPRefreshMS = 5 * 60 * 1000
	defaultPostOnlyHTTPTimeoutMS = 3000
)

func dynamicPostOnlyPricingEnabled(params map[string]any) bool {
	if boolParam(params, "dynamic_post_only_price") {
		return true
	}
	switch normalizedParam(params, "post_only_price_source") {
	case "book", "book_top", "top_of_book", "orderbook", "order_book", "http_l2book", "l2book_http", "hyperliquid_l2book_http":
		return true
	default:
		return false
	}
}

func dynamicPostOnlyHTTPPricingEnabled(venue string, params map[string]any) bool {
	if venue != "hyperliquid" {
		return false
	}
	switch normalizedParam(params, "post_only_price_source") {
	case "http_l2book", "l2book_http", "hyperliquid_l2book_http":
		return true
	default:
		return false
	}
}

func dynamicPostOnlyBookUnavailable(venue string) error {
	return fmt.Errorf("%s dynamic post-only pricing requested, but no book-top stream is configured", venue)
}

func dynamicPostOnlyPriceHook(venue string, params map[string]any, tracker *booktop.Tracker) prebuiltBuilderHook {
	cfg := newDynamicPostOnlyConfig(venue, params)
	return func(ctx context.Context, req payload.Request) (map[string]any, map[string]any, error) {
		effective := cloneParams(req.Params)
		side := normalizedParam(effective, "side")
		if side == "" {
			side = "buy"
		}
		snapshot, err := waitForFreshBook(ctx, tracker, cfg.wait, cfg.maxAge)
		if err != nil {
			return nil, nil, fmt.Errorf("%s dynamic post-only price: %w", venue, err)
		}
		price := cfg.price(snapshot, side)
		effective["price"] = cfg.paramPrice(price)
		metadata := dynamicPostOnlyMetadata(cfg, "book_top", price, snapshot)
		return effective, metadata, nil
	}
}

func dynamicPostOnlyHTTPPriceHook(venue string, params map[string]any, definition spec.Definition, runtime spec.RuntimeConfig) prebuiltBuilderHook {
	cfg := newDynamicPostOnlyConfig(venue, params)
	source := sharedHTTPBookSource(httpBookSourceConfig{
		venue:   venue,
		client:  &http.Client{Timeout: durationMSParam(params, "post_only_price_http_timeout_ms", defaultPostOnlyHTTPTimeoutMS)},
		url:     dynamicPostOnlyInfoURL(params, definition, runtime),
		symbol:  strings.ToUpper(spec.TextParam(runtime.Params, "symbol", "BTC")),
		ttl:     durationMSParam(params, "post_only_price_refresh_ms", defaultPostOnlyHTTPRefreshMS),
		parser:  booktop.NewHyperliquidParser(),
		cacheID: normalizedParam(params, "post_only_price_source"),
	})
	return func(ctx context.Context, req payload.Request) (map[string]any, map[string]any, error) {
		effective := cloneParams(req.Params)
		side := normalizedParam(effective, "side")
		if side == "" {
			side = "buy"
		}
		snapshot, fetchedAt, cached, err := source.current(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("%s dynamic post-only price: %w", venue, err)
		}
		price := cfg.price(snapshot, side)
		effective["price"] = cfg.paramPrice(price)
		metadata := dynamicPostOnlyMetadata(cfg, "hyperliquid_l2book_http", price, snapshot)
		metadata["post_only_price_refresh_ms"] = source.refresh().Milliseconds()
		metadata["post_only_price_cached"] = cached
		metadata["post_only_price_fetched_at"] = fetchedAt.Format(time.RFC3339Nano)
		metadata["post_only_price_endpoint"] = source.url
		return effective, metadata, nil
	}
}

type dynamicPostOnlyConfig struct {
	venue      string
	offsetBPS  float64
	wait       time.Duration
	maxAge     time.Duration
	multiplier float64
	tick       float64
}

func newDynamicPostOnlyConfig(venue string, params map[string]any) dynamicPostOnlyConfig {
	return dynamicPostOnlyConfig{
		venue:      venue,
		offsetBPS:  floatParam(params, "post_only_price_offset_bps", defaultPostOnlyOffsetBPS),
		wait:       durationMSParam(params, "post_only_book_wait_ms", defaultPostOnlyBookWaitMS),
		maxAge:     durationMSParam(params, "post_only_book_max_age_ms", defaultPostOnlyBookMaxAgeMS),
		multiplier: dynamicPostOnlyPriceMultiplier(venue, params),
		tick:       floatParam(params, "post_only_price_tick", 1),
	}
}

func (c dynamicPostOnlyConfig) price(snapshot booktop.Snapshot, side string) float64 {
	offset := c.offsetBPS / 10000
	isSell := side == "sell" || side == "ask"
	var raw float64
	if isSell {
		raw = snapshot.Ask * (1 + offset) * c.multiplier
		return roundToTick(raw, c.tick, math.Ceil)
	}
	raw = snapshot.Bid * (1 - offset) * c.multiplier
	return roundToTick(raw, c.tick, math.Floor)
}

func (c dynamicPostOnlyConfig) paramPrice(price float64) any {
	if c.venue == "lighter" {
		return int64(price)
	}
	return decimalText(price)
}

func (c dynamicPostOnlyConfig) displayPrice(price float64) any {
	if c.multiplier == 0 || c.multiplier == 1 {
		return decimalText(price)
	}
	return decimalText(price / c.multiplier)
}

func dynamicPostOnlyMetadata(cfg dynamicPostOnlyConfig, source string, price float64, snapshot booktop.Snapshot) map[string]any {
	metadata := map[string]any{
		"post_only_price_source":     source,
		"post_only_price":            cfg.displayPrice(price),
		"post_only_price_offset_bps": cfg.offsetBPS,
		"post_only_book_bid":         snapshot.Bid,
		"post_only_book_ask":         snapshot.Ask,
		"post_only_book_received_at": snapshot.ReceivedAt.Format(time.RFC3339Nano),
		"post_only_book_age_ms":      time.Since(snapshot.ReceivedAt).Milliseconds(),
	}
	if !snapshot.ExchangeAt.IsZero() {
		metadata["post_only_book_exchange_at"] = snapshot.ExchangeAt.Format(time.RFC3339Nano)
	}
	return metadata
}

func waitForFreshBook(ctx context.Context, tracker *booktop.Tracker, wait time.Duration, maxAge time.Duration) (booktop.Snapshot, error) {
	deadline := time.Now().Add(wait)
	for {
		now := time.Now()
		if snapshot, ok := tracker.Snapshot(); ok {
			age := snapshot.Age(now)
			if maxAge <= 0 || age <= maxAge {
				return snapshot, nil
			}
			if now.After(deadline) {
				return booktop.Snapshot{}, fmt.Errorf("book is stale: age=%s max_age=%s", age.Round(time.Millisecond), maxAge)
			}
		} else if now.After(deadline) {
			if err := tracker.Err(); err != nil {
				return booktop.Snapshot{}, err
			}
			return booktop.Snapshot{}, fmt.Errorf("no book snapshot within %s", wait)
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return booktop.Snapshot{}, ctx.Err()
		case <-timer.C:
		}
	}
}

func dynamicPostOnlyPriceMultiplier(venue string, params map[string]any) float64 {
	if value, ok := params["post_only_price_multiplier"]; ok {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(value)), 64)
		if err == nil && parsed > 0 {
			return parsed
		}
	}
	if venue == "lighter" {
		return 10
	}
	return 1
}

var (
	httpBookSourcesMu sync.Mutex
	httpBookSources   = map[string]*httpBookSource{}
)

type httpBookSourceConfig struct {
	venue   string
	client  *http.Client
	url     string
	symbol  string
	ttl     time.Duration
	parser  booktop.Parser
	cacheID string
}

type httpBookSource struct {
	client *http.Client
	url    string
	symbol string
	ttl    time.Duration
	parser booktop.Parser

	mu             sync.Mutex
	cachedSnapshot booktop.Snapshot
	fetchedAt      time.Time
}

func sharedHTTPBookSource(cfg httpBookSourceConfig) *httpBookSource {
	key := strings.Join([]string{cfg.venue, cfg.cacheID, cfg.url, cfg.symbol}, "\x00")
	httpBookSourcesMu.Lock()
	defer httpBookSourcesMu.Unlock()
	if source, ok := httpBookSources[key]; ok {
		source.update(cfg)
		return source
	}
	source := &httpBookSource{
		client: cfg.client,
		url:    cfg.url,
		symbol: cfg.symbol,
		ttl:    cfg.ttl,
		parser: cfg.parser,
	}
	httpBookSources[key] = source
	return source
}

func (s *httpBookSource) update(cfg httpBookSourceConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.client = cfg.client
	s.ttl = cfg.ttl
	s.parser = cfg.parser
}

func (s *httpBookSource) refresh() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ttl
}

func (s *httpBookSource) current(ctx context.Context) (booktop.Snapshot, time.Time, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if !s.fetchedAt.IsZero() && s.ttl > 0 && now.Sub(s.fetchedAt) < s.ttl {
		return s.cachedSnapshot, s.fetchedAt, true, nil
	}
	snapshot, err := s.fetch(ctx)
	if err != nil {
		return booktop.Snapshot{}, time.Time{}, false, err
	}
	snapshot.ReceivedAt = now.UTC()
	s.cachedSnapshot = snapshot
	s.fetchedAt = now.UTC()
	return snapshot, s.fetchedAt, false, nil
}

func (s *httpBookSource) fetch(ctx context.Context) (booktop.Snapshot, error) {
	if s.url == "" {
		return booktop.Snapshot{}, fmt.Errorf("Hyperliquid info URL is empty")
	}
	body, err := json.Marshal(map[string]any{"type": "l2Book", "coin": s.symbol})
	if err != nil {
		return booktop.Snapshot{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return booktop.Snapshot{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return booktop.Snapshot{}, err
	}
	defer resp.Body.Close()
	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return booktop.Snapshot{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return booktop.Snapshot{}, fmt.Errorf("Hyperliquid l2Book HTTP status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	snapshot, ok := s.parser.Parse(raw)
	if !ok {
		return booktop.Snapshot{}, fmt.Errorf("Hyperliquid l2Book response did not contain a usable book")
	}
	return snapshot, nil
}

func dynamicPostOnlyInfoURL(params map[string]any, definition spec.Definition, runtime spec.RuntimeConfig) string {
	if raw := strings.TrimSpace(fmt.Sprint(params["post_only_price_url"])); raw != "" && raw != "<nil>" {
		return raw
	}
	baseURL := strings.TrimRight(runtime.BaseURL, "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(definition.DefaultBaseURL, "/")
	}
	if baseURL == "" {
		baseURL = "https://api.hyperliquid.xyz"
	}
	return baseURL + "/info"
}

func roundToTick(value float64, tick float64, round func(float64) float64) float64 {
	if tick <= 0 {
		return value
	}
	return round(value/tick) * tick
}

func cloneParams(params map[string]any) map[string]any {
	out := make(map[string]any, len(params))
	for key, value := range params {
		out[key] = value
	}
	return out
}

func boolParam(params map[string]any, key string) bool {
	value, ok := params[key]
	if !ok || value == nil {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return err == nil && parsed
	default:
		parsed, err := strconv.ParseBool(strings.TrimSpace(fmt.Sprint(typed)))
		return err == nil && parsed
	}
}

func normalizedParam(params map[string]any, key string) string {
	value, ok := params[key]
	if !ok || value == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
}

func floatParam(params map[string]any, key string, fallback float64) float64 {
	value, ok := params[key]
	if !ok || value == nil {
		return fallback
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(value)), 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func durationMSParam(params map[string]any, key string, fallback int) time.Duration {
	ms := floatParam(params, key, float64(fallback))
	return time.Duration(ms * float64(time.Millisecond))
}

func decimalText(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "0"
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}
