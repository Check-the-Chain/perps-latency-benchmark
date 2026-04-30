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

type Config struct {
	Scenario      Scenario
	Iterations    int
	Warmups       int
	BatchSize     int
	RatePerSecond float64
	MaxInFlight   int
	StopOnError   bool
	LatencyMode   LatencyMode
	Cleanup       CleanupConfig
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
	Transport  string
	Request    netlatency.RequestTemplate
	Execute    func(context.Context) (netlatency.Result, error)
	Classifier lifecycle.Classifier
	Metadata   map[string]any
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

type CleanupResult struct {
	Attempted   bool   `json:"attempted"`
	OK          bool   `json:"ok"`
	StatusCode  int    `json:"status_code,omitempty"`
	Error       string `json:"error,omitempty"`
	DurationNS  int64  `json:"duration_ns,omitempty"`
	BytesRead   int64  `json:"bytes_read,omitempty"`
	Description string `json:"description,omitempty"`
}

type Sample struct {
	Venue          string                   `json:"venue"`
	Scenario       Scenario                 `json:"scenario"`
	Transport      string                   `json:"transport"`
	Index          int                      `json:"index"`
	Iteration      int                      `json:"iteration"`
	Warmup         bool                     `json:"warmup"`
	BatchSize      int                      `json:"batch_size"`
	ScheduledAt    time.Time                `json:"scheduled_at,omitempty"`
	SentAt         time.Time                `json:"sent_at,omitempty"`
	PreparedNS     int64                    `json:"prepared_ns"`
	NetworkNS      int64                    `json:"network_ns"`
	CorrectedNS    int64                    `json:"corrected_ns,omitempty"`
	StartDelayNS   int64                    `json:"start_delay_ns,omitempty"`
	StatusCode     int                      `json:"status_code,omitempty"`
	BytesRead      int64                    `json:"bytes_read,omitempty"`
	OK             bool                     `json:"ok"`
	Error          string                   `json:"error,omitempty"`
	Classification lifecycle.Classification `json:"classification"`
	Cleanup        *CleanupResult           `json:"cleanup,omitempty"`
	Trace          netlatency.Trace         `json:"trace"`
	Metadata       map[string]any           `json:"metadata,omitempty"`
	CompletedAt    time.Time                `json:"completed_at"`
}

type Result struct {
	Venue       string      `json:"venue"`
	Scenario    Scenario    `json:"scenario"`
	LatencyMode LatencyMode `json:"latency_mode"`
	Samples     []Sample    `json:"samples"`
}

type ComparisonResult struct {
	Venue       string      `json:"venue"`
	Scenario    Scenario    `json:"scenario"`
	LatencyMode LatencyMode `json:"latency_mode"`
	Results     []Result    `json:"results"`
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
