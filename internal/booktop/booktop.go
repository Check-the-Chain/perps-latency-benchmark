package booktop

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Config struct {
	Venue  string
	URL    string
	Symbol string
	Parser Parser
}

type Snapshot struct {
	Venue      string    `json:"venue"`
	Bid        float64   `json:"bid,omitempty"`
	BidSize    float64   `json:"bid_size,omitempty"`
	Ask        float64   `json:"ask,omitempty"`
	AskSize    float64   `json:"ask_size,omitempty"`
	Bids       []Level   `json:"bids,omitempty"`
	Asks       []Level   `json:"asks,omitempty"`
	ReceivedAt time.Time `json:"received_at"`
	ExchangeAt time.Time `json:"exchange_at,omitempty"`
}

type Level struct {
	Price float64 `json:"price"`
	Size  float64 `json:"size"`
}

const maxSnapshotLevels = 15

func (s Snapshot) Age(at time.Time) time.Duration {
	if s.ReceivedAt.IsZero() || at.IsZero() {
		return 0
	}
	return at.Sub(s.ReceivedAt)
}

type Tracker struct {
	cfg    Config
	parser Parser

	mu      sync.RWMutex
	latest  Snapshot
	history []Snapshot
	err     error
	conn    *websocket.Conn
}

func NewTracker(cfg Config) *Tracker {
	parser := cfg.Parser
	if parser == nil {
		parser = NewGenericParser()
	}
	return &Tracker{cfg: cfg, parser: parser}
}

func (t *Tracker) Run(ctx context.Context) {
	for {
		if err := t.runOnce(ctx); err != nil {
			t.setErr(err)
		}
		select {
		case <-ctx.Done():
			t.close()
			return
		case <-time.After(time.Second):
		}
	}
}

func (t *Tracker) Snapshot() (Snapshot, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.latest.Bid <= 0 || t.latest.Ask <= 0 {
		return Snapshot{}, false
	}
	return t.latest, true
}

func (t *Tracker) SnapshotAt(at time.Time) (Snapshot, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if at.IsZero() {
		if t.latest.Bid <= 0 || t.latest.Ask <= 0 {
			return Snapshot{}, false
		}
		return t.latest, true
	}
	for index := len(t.history) - 1; index >= 0; index-- {
		snapshot := t.history[index]
		if snapshot.Bid <= 0 || snapshot.Ask <= 0 {
			continue
		}
		if !snapshot.ReceivedAt.After(at) {
			return snapshot, true
		}
	}
	return Snapshot{}, false
}

func (t *Tracker) Err() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.err
}

func (t *Tracker) runOnce(ctx context.Context) error {
	url := strings.TrimSpace(t.cfg.URL)
	if url == "" {
		return fmt.Errorf("missing book websocket URL")
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, http.Header{})
	if err != nil {
		return err
	}
	t.mu.Lock()
	t.conn = conn
	t.mu.Unlock()
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	defer func() {
		close(done)
		t.close()
	}()

	if msg := t.parser.Subscribe(t.cfg); len(msg) > 0 {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return err
		}
	}
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		snapshot, ok := t.parser.Parse(data)
		if !ok {
			continue
		}
		now := time.Now().UTC()
		snapshot.Venue = t.cfg.Venue
		snapshot.ReceivedAt = now
		t.mu.Lock()
		t.latest = snapshot
		t.history = append(t.history, snapshot)
		if len(t.history) > 2048 {
			copy(t.history, t.history[len(t.history)-2048:])
			t.history = t.history[:2048]
		}
		t.err = nil
		t.mu.Unlock()
	}
}

func (t *Tracker) close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn != nil {
		_ = t.conn.Close()
		t.conn = nil
	}
}

func (t *Tracker) setErr(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.err = err
}

type Parser interface {
	Subscribe(Config) []byte
	Parse([]byte) (Snapshot, bool)
}

func NewGenericParser() Parser {
	return genericParser{}
}

func NewHyperliquidParser() Parser {
	return hyperliquidParser{}
}

func NewLighterParser() Parser {
	return &lighterParser{}
}

func NewAsterParser() Parser {
	return asterParser{}
}

func NewExtendedParser() Parser {
	return &extendedParser{}
}

func NewPacificaParser() Parser {
	return pacificaParser{}
}

type genericParser struct{}

func (genericParser) Subscribe(Config) []byte {
	return nil
}

func (genericParser) Parse(data []byte) (Snapshot, bool) {
	return parseBookData(data, parseGeneric)
}

type hyperliquidParser struct{}

func (hyperliquidParser) Subscribe(cfg Config) []byte {
	return []byte(fmt.Sprintf(`{"method":"subscribe","subscription":{"type":"l2Book","coin":%q}}`, cfg.Symbol))
}

func (hyperliquidParser) Parse(data []byte) (Snapshot, bool) {
	return parseBookData(data, parseHyperliquid)
}

type lighterParser struct {
	bids map[float64]float64
	asks map[float64]float64
}

func (lighterParser) Subscribe(cfg Config) []byte {
	return []byte(fmt.Sprintf(`{"type":"subscribe","channel":"order_book/%s"}`, cfg.Symbol))
}

func (p *lighterParser) Parse(data []byte) (Snapshot, bool) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return Snapshot{}, false
	}
	body := confirmMap(root["order_book"])
	if len(body) == 0 {
		body = root
	}
	messageType := strings.ToLower(text(root["type"]))
	replace := p.bids == nil || p.asks == nil || !strings.HasPrefix(messageType, "update/")
	if replace {
		p.bids = make(map[float64]float64)
		p.asks = make(map[float64]float64)
	}
	applyExtendedLevels(p.bids, findAny(body, "bids", "bid", "b"), false)
	applyExtendedLevels(p.asks, findAny(body, "asks", "ask", "a"), false)
	trimBook(p.bids, true, maxSnapshotLevels)
	trimBook(p.asks, false, maxSnapshotLevels)
	bid, bidSize := bestBid(p.bids)
	ask, askSize := bestAsk(p.asks)
	if bid <= 0 || ask <= 0 {
		return Snapshot{}, false
	}
	return Snapshot{
		Bid:        bid,
		BidSize:    bidSize,
		Ask:        ask,
		AskSize:    askSize,
		Bids:       sortedBookLevels(p.bids, true),
		Asks:       sortedBookLevels(p.asks, false),
		ExchangeAt: lighterTime(root, body),
	}, true
}

type asterParser struct{}

func (asterParser) Subscribe(Config) []byte {
	return nil
}

func (asterParser) Parse(data []byte) (Snapshot, bool) {
	return parseBookData(data, parseAster)
}

func parseBookData(data []byte, parse func(any) Snapshot) (Snapshot, bool) {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return Snapshot{}, false
	}
	snapshot := parse(value)
	if snapshot.Bid <= 0 || snapshot.Ask <= 0 {
		return Snapshot{}, false
	}
	return snapshot, true
}

func parseHyperliquid(value any) Snapshot {
	data := findMap(value, "data")
	if len(data) == 0 {
		data = confirmMap(value)
	}
	levels, _ := data["levels"].([]any)
	if len(levels) < 2 {
		return Snapshot{}
	}
	bids := parseLevels(levels[0], true)
	asks := parseLevels(levels[1], false)
	return snapshotFromLevels(bids, asks, unixMillis(data["time"]))
}

func parseAster(value any) Snapshot {
	root, ok := value.(map[string]any)
	if !ok {
		return Snapshot{}
	}
	bids := parseLevels(root["b"], true)
	asks := parseLevels(root["a"], false)
	return snapshotFromLevels(bids, asks, unixMillis(root["E"]))
}

type pacificaParser struct{}

func (pacificaParser) Subscribe(cfg Config) []byte {
	return []byte(fmt.Sprintf(`{"method":"subscribe","params":{"source":"bbo","symbol":%q}}`, cfg.Symbol))
}

func (pacificaParser) Parse(data []byte) (Snapshot, bool) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return Snapshot{}, false
	}
	if channel := text(root["channel"]); channel != "" && channel != "bbo" {
		return Snapshot{}, false
	}
	body := confirmMap(root["data"])
	if len(body) == 0 {
		body = root
	}
	bid := number(body["b"])
	bidSize := number(body["B"])
	ask := number(body["a"])
	askSize := number(body["A"])
	if bid <= 0 || ask <= 0 {
		return Snapshot{}, false
	}
	return Snapshot{
		Bid:        bid,
		BidSize:    bidSize,
		Ask:        ask,
		AskSize:    askSize,
		Bids:       []Level{{Price: bid, Size: bidSize}},
		Asks:       []Level{{Price: ask, Size: askSize}},
		ExchangeAt: unixMillis(body["t"]),
	}, true
}

type extendedParser struct {
	bids map[float64]float64
	asks map[float64]float64
}

func (p *extendedParser) Subscribe(Config) []byte {
	return nil
}

func (p *extendedParser) Parse(data []byte) (Snapshot, bool) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return Snapshot{}, false
	}
	body := confirmMap(root["data"])
	if len(body) == 0 {
		body = root
	}
	messageType := strings.ToUpper(text(firstPresent(root, "type", "t")))
	if messageType == "" {
		messageType = strings.ToUpper(text(firstPresent(body, "type", "t")))
	}
	switch messageType {
	case "SNAPSHOT":
		p.bids = make(map[float64]float64)
		p.asks = make(map[float64]float64)
		applyExtendedLevels(p.bids, findAny(body, "bids", "bid", "b"), false)
		applyExtendedLevels(p.asks, findAny(body, "asks", "ask", "a"), false)
	case "DELTA":
		if p.bids == nil {
			p.bids = make(map[float64]float64)
		}
		if p.asks == nil {
			p.asks = make(map[float64]float64)
		}
		applyExtendedLevels(p.bids, findAny(body, "bids", "bid", "b"), true)
		applyExtendedLevels(p.asks, findAny(body, "asks", "ask", "a"), true)
	default:
		snapshot := parseGeneric(root)
		if snapshot.Bid <= 0 || snapshot.Ask <= 0 {
			return Snapshot{}, false
		}
		return snapshot, true
	}
	trimBook(p.bids, true, maxSnapshotLevels)
	trimBook(p.asks, false, maxSnapshotLevels)
	bid, bidSize := bestBid(p.bids)
	ask, askSize := bestAsk(p.asks)
	if bid <= 0 || ask <= 0 {
		return Snapshot{}, false
	}
	return Snapshot{
		Bid:        bid,
		BidSize:    bidSize,
		Ask:        ask,
		AskSize:    askSize,
		Bids:       sortedBookLevels(p.bids, true),
		Asks:       sortedBookLevels(p.asks, false),
		ExchangeAt: unixMillis(root["ts"]),
	}, true
}

func applyExtendedLevels(book map[float64]float64, value any, delta bool) {
	for _, level := range objectList(value) {
		price := firstNumber(level, "price", "p", "px")
		if price <= 0 {
			continue
		}
		size := firstNumber(level, "qty", "quantity", "size", "sz", "q")
		if delta {
			current := firstNumber(level, "current", "c")
			if current > 0 || hasKey(level, "current", "c") {
				size = current
			}
		}
		if size <= 0 {
			delete(book, price)
			continue
		}
		book[price] = size
	}
}

func bestBid(book map[float64]float64) (float64, float64) {
	var price float64
	var size float64
	for candidate, candidateSize := range book {
		if candidateSize <= 0 {
			continue
		}
		if price == 0 || candidate > price {
			price = candidate
			size = candidateSize
		}
	}
	return price, size
}

func bestAsk(book map[float64]float64) (float64, float64) {
	var price float64
	var size float64
	for candidate, candidateSize := range book {
		if candidateSize <= 0 {
			continue
		}
		if price == 0 || candidate < price {
			price = candidate
			size = candidateSize
		}
	}
	return price, size
}

func parseGeneric(value any) Snapshot {
	bids := findAny(value, "bids", "bid", "b")
	asks := findAny(value, "asks", "ask", "a")
	return snapshotFromLevels(parseLevels(bids, true), parseLevels(asks, false), time.Time{})
}

func snapshotFromLevels(bids []Level, asks []Level, exchangeAt time.Time) Snapshot {
	if len(bids) == 0 || len(asks) == 0 {
		return Snapshot{}
	}
	bids = limitLevels(bids, maxSnapshotLevels)
	asks = limitLevels(asks, maxSnapshotLevels)
	return Snapshot{
		Bid:        bids[0].Price,
		BidSize:    bids[0].Size,
		Ask:        asks[0].Price,
		AskSize:    asks[0].Size,
		Bids:       bids,
		Asks:       asks,
		ExchangeAt: exchangeAt,
	}
}

func parseLevels(value any, bids bool) []Level {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	levels := make([]Level, 0, len(raw))
	for _, item := range raw {
		price, size := parseLevel(item)
		if price <= 0 || size <= 0 {
			continue
		}
		levels = append(levels, Level{Price: price, Size: size})
	}
	sortLevels(levels, bids)
	return limitLevels(levels, maxSnapshotLevels)
}

func sortedBookLevels(book map[float64]float64, bids bool) []Level {
	return limitLevels(sortedBookLevelsAll(book, bids), maxSnapshotLevels)
}

func sortedBookLevelsAll(book map[float64]float64, bids bool) []Level {
	levels := make([]Level, 0, len(book))
	for price, size := range book {
		if price <= 0 || size <= 0 {
			continue
		}
		levels = append(levels, Level{Price: price, Size: size})
	}
	sortLevels(levels, bids)
	return levels
}

func sortLevels(levels []Level, bids bool) {
	sort.Slice(levels, func(i, j int) bool {
		if bids {
			return levels[i].Price > levels[j].Price
		}
		return levels[i].Price < levels[j].Price
	})
}

func limitLevels(levels []Level, max int) []Level {
	if max <= 0 || len(levels) <= max {
		return levels
	}
	return levels[:max]
}

func trimBook(book map[float64]float64, bids bool, max int) {
	if max <= 0 || len(book) <= max {
		return
	}
	for _, level := range sortedBookLevelsAll(book, bids)[max:] {
		delete(book, level.Price)
	}
}

func parseLevel(value any) (float64, float64) {
	switch typed := value.(type) {
	case []any:
		if len(typed) == 0 {
			return 0, 0
		}
		price := number(typed[0])
		var size float64
		if len(typed) > 1 {
			size = number(typed[1])
		}
		return price, size
	case map[string]any:
		price := firstNumber(typed, "px", "price", "p")
		size := firstNumber(typed, "sz", "size", "qty", "quantity", "q")
		return price, size
	default:
		return 0, 0
	}
}

func findMap(value any, key string) map[string]any {
	found, _ := findAny(value, key).(map[string]any)
	return found
}

func confirmMap(value any) map[string]any {
	found, _ := value.(map[string]any)
	return found
}

func objectList(value any) []map[string]any {
	switch typed := value.(type) {
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if object, ok := item.(map[string]any); ok {
				out = append(out, object)
			}
		}
		return out
	case []map[string]any:
		return typed
	default:
		return nil
	}
}

func findAny(value any, keys ...string) any {
	keySet := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		keySet[strings.ToLower(key)] = struct{}{}
	}
	var walk func(any) any
	walk = func(current any) any {
		switch typed := current.(type) {
		case map[string]any:
			for key, value := range typed {
				if _, ok := keySet[strings.ToLower(key)]; ok {
					return value
				}
			}
			for _, value := range typed {
				if found := walk(value); found != nil {
					return found
				}
			}
		case []any:
			for _, value := range typed {
				if found := walk(value); found != nil {
					return found
				}
			}
		}
		return nil
	}
	return walk(value)
}

func firstNumber(values map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			if number := number(value); number > 0 {
				return number
			}
		}
	}
	return 0
}

func firstPresent(values map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value
		}
	}
	return nil
}

func hasKey(values map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := values[key]; ok {
			return true
		}
	}
	return false
}

func number(value any) float64 {
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
		return 0
	}
}

func text(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func unixMillis(value any) time.Time {
	ms := number(value)
	if ms <= 0 || math.IsNaN(ms) {
		return time.Time{}
	}
	return time.UnixMilli(int64(ms)).UTC()
}

func unixMicros(value any) time.Time {
	micros := number(value)
	if micros <= 0 || math.IsNaN(micros) {
		return time.Time{}
	}
	return time.UnixMicro(int64(micros)).UTC()
}

func lighterTime(root map[string]any, body map[string]any) time.Time {
	if at := unixMicros(firstPresent(root, "last_updated_at")); !at.IsZero() {
		return at
	}
	if at := unixMicros(firstPresent(body, "last_updated_at")); !at.IsZero() {
		return at
	}
	return unixMillis(firstPresent(root, "timestamp"))
}
