package bench

import (
	"context"
	"time"

	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/netlatency"
)

type Scenario string

const (
	ScenarioSingle Scenario = "single"
	ScenarioBatch  Scenario = "batch"
)

type LatencyMode string

const (
	LatencyModeTotal LatencyMode = "total"
	LatencyModeTTFB  LatencyMode = "ttfb"
)

type MeasurementMode string

const (
	MeasurementModeAck            MeasurementMode = "ack"
	MeasurementModeWSConfirmation MeasurementMode = "ws_confirmation"
)

type Config struct {
	RunID               string
	Scenario            Scenario
	Iterations          int
	Warmups             int
	BatchSize           int
	RatePerSecond       float64
	MaxInFlight         int
	StopOnError         bool
	LatencyMode         LatencyMode
	MeasurementMode     MeasurementMode
	ConfirmationTimeout time.Duration
	Cleanup             CleanupConfig
}

type CleanupMode string

const (
	CleanupModeOff        CleanupMode = "off"
	CleanupModeBestEffort CleanupMode = "best_effort"
	CleanupModeStrict     CleanupMode = "strict"
)

type CleanupScope string

const (
	CleanupScopeAfterSample CleanupScope = "after_sample"
)

type CleanupConfig struct {
	Enabled   bool
	Mode      CleanupMode
	Scope     CleanupScope
	TimeoutMS int
}

type PreparedRequest struct {
	Transport        string
	Request          netlatency.RequestTemplate
	ParallelRequests []netlatency.RequestTemplate
	Execute          func(context.Context) (netlatency.Result, error)
	Confirm          *Confirmation
	Classifier       lifecycle.Classifier
	Metadata         map[string]any
}

type Confirmation struct {
	Wait  func(context.Context, netlatency.Result) (netlatency.Result, error)
	Close func() error
}

type Venue interface {
	Name() string
	Prepare(ctx context.Context, scenario Scenario, iteration int, batchSize int) (PreparedRequest, error)
	Close(ctx context.Context) error
}

type CleanupAdapter interface {
	AfterSample(ctx context.Context, sample Sample) CleanupResult
	Close(ctx context.Context) error
}

type PreparedCleanupAdapter interface {
	PrepareAfterSample(ctx context.Context, sample Sample) (PreparedCleanup, error)
}

type PreparedCleanup struct {
	Execute    func(context.Context) CleanupResult
	Result     *CleanupResult
	PreparedNS int64
}

type RunCleanupAdapter interface {
	BeforeRun(ctx context.Context, run CleanupRun) CleanupResult
	AfterRun(ctx context.Context, result Result) CleanupResult
}

type CleanupRun struct {
	Venue      string   `json:"venue"`
	RunID      string   `json:"run_id"`
	Scenario   Scenario `json:"scenario"`
	Iterations int      `json:"iterations"`
	Warmups    int      `json:"warmups"`
	BatchSize  int      `json:"batch_size"`
}

type NetworkBaselineSnapshot struct {
	FloorNS int64
	Source  string
}

type NetworkBaselineProvider interface {
	Snapshot() NetworkBaselineSnapshot
}

type NetworkBaselineObserver interface {
	NetworkBaselineProvider
	ObserveTrace(netlatency.Trace)
	ObserveRTT(valueNS int64, source string)
}

type CleanupResult struct {
	Attempted    bool             `json:"attempted"`
	OK           bool             `json:"ok"`
	StatusCode   int              `json:"status_code,omitempty"`
	Error        string           `json:"error,omitempty"`
	DurationNS   int64            `json:"duration_ns,omitempty"`
	PreparedNS   int64            `json:"prepared_ns,omitempty"`
	ScheduledAt  time.Time        `json:"scheduled_at,omitempty"`
	SentAt       time.Time        `json:"sent_at,omitempty"`
	WriteDelayNS int64            `json:"write_delay_ns,omitempty"`
	StartDelayNS int64            `json:"start_delay_ns,omitempty"`
	BytesRead    int64            `json:"bytes_read,omitempty"`
	Description  string           `json:"description,omitempty"`
	Trace        netlatency.Trace `json:"trace,omitempty"`
	Metadata     map[string]any   `json:"metadata,omitempty"`
}

type BalanceSnapshot struct {
	Venue         string         `json:"venue"`
	Currency      string         `json:"currency,omitempty"`
	BalanceUSD    float64        `json:"balance_usd,omitempty"`
	EquityUSD     float64        `json:"equity_usd,omitempty"`
	UnrealizedUSD float64        `json:"unrealized_usd,omitempty"`
	CapturedAt    time.Time      `json:"captured_at,omitempty"`
	Positions     []Position     `json:"positions,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type Position struct {
	Market        string  `json:"market,omitempty"`
	Symbol        string  `json:"symbol,omitempty"`
	Size          string  `json:"size,omitempty"`
	UnrealizedUSD float64 `json:"unrealized_usd,omitempty"`
	AvgEntryPrice string  `json:"avg_entry_price,omitempty"`
}

type SampleCost struct {
	Venue                 string          `json:"venue"`
	RunID                 string          `json:"run_id,omitempty"`
	CompletedAt           time.Time       `json:"completed_at,omitempty"`
	EntryOrderID          string          `json:"entry_order_id,omitempty"`
	ExitOrderID           string          `json:"exit_order_id,omitempty"`
	EntryQty              float64         `json:"entry_qty,omitempty"`
	ExitQty               float64         `json:"exit_qty,omitempty"`
	EntryValueUSD         float64         `json:"entry_value_usd,omitempty"`
	ExitValueUSD          float64         `json:"exit_value_usd,omitempty"`
	EntryFeeUSD           float64         `json:"entry_fee_usd,omitempty"`
	ExitFeeUSD            float64         `json:"exit_fee_usd,omitempty"`
	PriceMoveCostUSD      float64         `json:"price_move_cost_usd,omitempty"`
	TradeCostUSD          float64         `json:"trade_cost_usd,omitempty"`
	BalanceBeforeUSD      float64         `json:"balance_before_usd,omitempty"`
	BalanceAfterUSD       float64         `json:"balance_after_usd,omitempty"`
	BalanceDeltaUSD       float64         `json:"balance_delta_usd,omitempty"`
	ReconciliationDiffUSD float64         `json:"reconciliation_diff_usd,omitempty"`
	Clean                 bool            `json:"clean"`
	QualityReason         string          `json:"quality_reason,omitempty"`
	Metadata              map[string]any  `json:"metadata,omitempty"`
	BalanceBefore         BalanceSnapshot `json:"balance_before,omitempty"`
	BalanceAfter          BalanceSnapshot `json:"balance_after,omitempty"`
}

type Sample struct {
	Venue              string                   `json:"venue"`
	RunID              string                   `json:"run_id,omitempty"`
	Scenario           Scenario                 `json:"scenario"`
	Transport          string                   `json:"transport"`
	OrderType          string                   `json:"order_type,omitempty"`
	Index              int                      `json:"index"`
	Iteration          int                      `json:"iteration"`
	Warmup             bool                     `json:"warmup"`
	BatchSize          int                      `json:"batch_size"`
	ScheduledAt        time.Time                `json:"scheduled_at,omitempty"`
	SentAt             time.Time                `json:"sent_at,omitempty"`
	PreparedNS         int64                    `json:"prepared_ns"`
	NetworkNS          int64                    `json:"network_ns"`
	RawNetworkNS       int64                    `json:"raw_network_ns,omitempty"`
	AdjustedNetworkNS  int64                    `json:"adjusted_network_ns,omitempty"`
	NetworkFloorNS     int64                    `json:"network_floor_ns,omitempty"`
	NetworkFloorSource string                   `json:"network_floor_source,omitempty"`
	SpeedBumpNS        int64                    `json:"speed_bump_ns,omitempty"`
	SpeedBumpSource    string                   `json:"speed_bump_source,omitempty"`
	SubmissionNS       int64                    `json:"submission_ns,omitempty"`
	CorrectedNS        int64                    `json:"corrected_ns,omitempty"`
	StartDelayNS       int64                    `json:"start_delay_ns,omitempty"`
	WriteDelayNS       int64                    `json:"write_delay_ns,omitempty"`
	StatusCode         int                      `json:"status_code,omitempty"`
	BytesRead          int64                    `json:"bytes_read,omitempty"`
	OK                 bool                     `json:"ok"`
	Error              string                   `json:"error,omitempty"`
	Classification     lifecycle.Classification `json:"classification"`
	Cleanup            *CleanupResult           `json:"cleanup,omitempty"`
	Cost               *SampleCost              `json:"cost,omitempty"`
	OrderRefs          []OrderRef               `json:"order_refs,omitempty"`
	CloseoutOrderRefs  []OrderRef               `json:"closeout_order_refs,omitempty"`
	ExpectedEntryFill  *ExpectedFill            `json:"expected_entry_fill,omitempty"`
	ExpectedExitFill   *ExpectedFill            `json:"expected_exit_fill,omitempty"`
	Trace              netlatency.Trace         `json:"trace"`
	Metadata           map[string]any           `json:"metadata,omitempty"`
	MeasurementMode    MeasurementMode          `json:"measurement_mode,omitempty"`
	CompletedAt        time.Time                `json:"completed_at"`
}

type OrderRef struct {
	Venue            string `json:"venue,omitempty"`
	Symbol           string `json:"symbol,omitempty"`
	Market           string `json:"market,omitempty"`
	MarketIndex      int    `json:"market_index,omitempty"`
	Side             string `json:"side,omitempty"`
	Size             string `json:"size,omitempty"`
	Asset            int    `json:"asset"`
	ClientOrderID    string `json:"client_order_id,omitempty"`
	ClientOrderIndex string `json:"client_order_index,omitempty"`
	OrderIndex       string `json:"order_index,omitempty"`
	ExternalID       string `json:"external_id,omitempty"`
	Cloid            string `json:"cloid,omitempty"`
}

type ExpectedFill struct {
	Phase          string     `json:"phase,omitempty"`
	Side           string     `json:"side,omitempty"`
	Size           float64    `json:"size,omitempty"`
	ExpectedPrice  float64    `json:"expected_price,omitempty"`
	TopBid         float64    `json:"top_bid,omitempty"`
	TopAsk         float64    `json:"top_ask,omitempty"`
	TopBidSize     float64    `json:"top_bid_size,omitempty"`
	TopAskSize     float64    `json:"top_ask_size,omitempty"`
	TopAvailable   float64    `json:"top_available,omitempty"`
	TopSufficient  bool       `json:"top_sufficient"`
	BookAvailable  float64    `json:"book_available,omitempty"`
	BookSufficient *bool      `json:"book_sufficient,omitempty"`
	BookLevels     int        `json:"book_levels,omitempty"`
	DepthWeighted  bool       `json:"depth_weighted,omitempty"`
	BookAgeNS      int64      `json:"book_age_ns,omitempty"`
	BookReceivedAt time.Time  `json:"book_received_at,omitempty"`
	BookExchangeAt *time.Time `json:"book_exchange_at,omitempty"`
}

type Result struct {
	Venue           string          `json:"venue"`
	RunID           string          `json:"run_id,omitempty"`
	Scenario        Scenario        `json:"scenario"`
	LatencyMode     LatencyMode     `json:"latency_mode"`
	MeasurementMode MeasurementMode `json:"measurement_mode,omitempty"`
	StartupCleanup  *CleanupResult  `json:"startup_cleanup,omitempty"`
	Reconciliation  *CleanupResult  `json:"reconciliation,omitempty"`
	Samples         []Sample        `json:"samples"`
}

type ComparisonResult struct {
	Venue           string          `json:"venue"`
	RunID           string          `json:"run_id,omitempty"`
	Scenario        Scenario        `json:"scenario"`
	LatencyMode     LatencyMode     `json:"latency_mode"`
	MeasurementMode MeasurementMode `json:"measurement_mode,omitempty"`
	Results         []Result        `json:"results"`
}

func (r Result) MeasuredSamples() []Sample {
	measured := make([]Sample, 0, len(r.Samples))
	for _, sample := range r.Samples {
		if !sample.Warmup {
			measured = append(measured, sample)
		}
	}
	return measured
}

func (c Config) Normalized() Config {
	if c.Scenario == "" {
		c.Scenario = ScenarioSingle
	}
	if c.Iterations == 0 {
		c.Iterations = 10
	}
	if c.BatchSize == 0 {
		c.BatchSize = 1
	}
	if c.MaxInFlight == 0 {
		c.MaxInFlight = 1
		if c.RatePerSecond > 0 {
			c.MaxInFlight = 128
		}
	}
	if c.LatencyMode == "" {
		c.LatencyMode = LatencyModeTotal
	}
	if c.MeasurementMode == "" {
		c.MeasurementMode = MeasurementModeAck
	}
	if c.ConfirmationTimeout == 0 {
		c.ConfirmationTimeout = 10 * time.Second
	}
	c.Cleanup = c.Cleanup.Normalized()
	return c
}

func (c CleanupConfig) Normalized() CleanupConfig {
	if !c.Enabled {
		c.Mode = CleanupModeOff
		return c
	}
	if c.Mode == "" {
		c.Mode = CleanupModeBestEffort
	}
	if c.Scope == "" {
		c.Scope = CleanupScopeAfterSample
	}
	if c.TimeoutMS == 0 {
		c.TimeoutMS = 5000
	}
	return c
}
