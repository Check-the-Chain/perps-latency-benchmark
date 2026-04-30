package cleanup

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/payload"
)

type CommandConfig struct {
	Type           string
	Command        []string
	Env            map[string]string
	Directory      string
	Timeout        time.Duration
	Method         string
	URL            string
	Headers        map[string]string
	StaticParams   map[string]any
	Client         *netlatency.Client
	Classifier     lifecycle.Classifier
	Description    string
	SkipNoRefs     bool
	OrderRefsField string
}

type CommandAdapter struct {
	cfg         CommandConfig
	builder     payload.Builder
	headers     http.Header
	mu          sync.Mutex
	runMetadata map[string]any
}

func NewCommandAdapter(cfg CommandConfig) (*CommandAdapter, error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("cleanup command adapter requires network client")
	}
	if len(cfg.Command) == 0 {
		return nil, fmt.Errorf("cleanup command is required")
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	commandCfg := payload.CommandConfig{
		Command:   cfg.Command,
		Env:       cfg.Env,
		Timeout:   timeout,
		Directory: cfg.Directory,
	}
	var builder payload.Builder
	var err error
	switch cmp.Or(cfg.Type, "persistent_command") {
	case "command":
		builder, err = payload.NewCommandBuilder(commandCfg)
	case "persistent_command":
		builder, err = payload.NewPersistentCommandBuilder(commandCfg)
	default:
		return nil, fmt.Errorf("unknown cleanup command type %q", cfg.Type)
	}
	if err != nil {
		return nil, err
	}

	headers := make(http.Header)
	for key, value := range cfg.Headers {
		headers.Set(key, value)
	}
	return &CommandAdapter{cfg: cfg, builder: builder, headers: headers}, nil
}

func (a *CommandAdapter) AfterSample(ctx context.Context, sample bench.Sample) bench.CleanupResult {
	start := time.Now()
	if a.cfg.SkipNoRefs && !hasOrderRefs(sample.Metadata, cmp.Or(a.cfg.OrderRefsField, "cleanup_orders")) {
		return bench.CleanupResult{Attempted: false, OK: true, Description: "no cleanup order refs"}
	}

	params := map[string]any{
		"phase":          "after_sample",
		"sample":         sample,
		"metadata":       sample.Metadata,
		"run_metadata":   a.currentRunMetadata(),
		"builder_params": a.cfg.StaticParams,
	}
	return a.run(ctx, start, payload.Request{
		Venue:     sample.Venue,
		Transport: sample.Transport,
		Scenario:  sample.Scenario,
		Iteration: sample.Iteration,
		BatchSize: sample.BatchSize,
		Params:    params,
	})
}

func (a *CommandAdapter) BeforeRun(ctx context.Context, run bench.CleanupRun) bench.CleanupResult {
	start := time.Now()
	cleanup := a.run(ctx, start, payload.Request{
		Venue:     run.Venue,
		Scenario:  run.Scenario,
		BatchSize: run.BatchSize,
		Params: map[string]any{
			"phase":          "before_run",
			"run":            run,
			"builder_params": a.cfg.StaticParams,
		},
	})
	a.mu.Lock()
	a.runMetadata = copyMap(cleanup.Metadata)
	a.mu.Unlock()
	return cleanup
}

func (a *CommandAdapter) AfterRun(ctx context.Context, result bench.Result) bench.CleanupResult {
	start := time.Now()
	return a.run(ctx, start, payload.Request{
		Venue:     result.Venue,
		Scenario:  result.Scenario,
		BatchSize: 1,
		Params: map[string]any{
			"phase":          "after_run",
			"result":         result,
			"run_metadata":   a.currentRunMetadata(),
			"builder_params": a.cfg.StaticParams,
		},
	})
}

func (a *CommandAdapter) run(ctx context.Context, start time.Time, req payload.Request) bench.CleanupResult {
	built, err := a.builder.Build(ctx, req)
	if err != nil {
		return cleanupError(start, err)
	}
	if built.Cleanup != nil {
		built.Cleanup.DurationNS = time.Since(start).Nanoseconds()
		return *built.Cleanup
	}

	body, err := payload.Bytes(built.Body, built.BodyBase64, nil)
	if err != nil {
		return cleanupError(start, err)
	}
	if len(body) == 0 {
		return bench.CleanupResult{
			Attempted:   false,
			OK:          true,
			DurationNS:  time.Since(start).Nanoseconds(),
			Description: cmp.Or(a.cfg.Description, "cleanup skipped"),
		}
	}
	result, err := a.cfg.Client.Do(ctx, netlatency.RequestTemplate{
		Method: cmp.Or(built.Method, a.cfg.Method, http.MethodPost),
		URL:    cmp.Or(built.URL, a.cfg.URL),
		Header: payload.MergeHeaders(a.headers, built.Headers),
		Body:   body,
	})
	classification := lifecycle.ClassifyResponse(lifecycle.ResponseInput{
		StatusCode: result.StatusCode,
		Body:       result.Body,
		Err:        err,
	})
	if a.cfg.Classifier != nil {
		classification = a.cfg.Classifier(lifecycle.ResponseInput{
			StatusCode: result.StatusCode,
			Body:       result.Body,
			Err:        err,
		})
	}
	ok := err == nil && classification.OK()
	cleanup := bench.CleanupResult{
		Attempted:   true,
		OK:          ok,
		StatusCode:  result.StatusCode,
		DurationNS:  time.Since(start).Nanoseconds(),
		BytesRead:   result.BytesRead,
		Description: cmp.Or(cleanupDescription(built.Metadata), a.cfg.Description, "cleanup"),
		Metadata:    cleanupMetadata(built.Metadata),
	}
	if err != nil {
		cleanup.Error = err.Error()
	} else if !ok {
		cleanup.Error = string(classification.Status)
		if classification.Reason != "" {
			cleanup.Error += ": " + classification.Reason
		}
	}
	return cleanup
}

func (a *CommandAdapter) currentRunMetadata() map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	return copyMap(a.runMetadata)
}

func (a *CommandAdapter) Close(ctx context.Context) error {
	if closer, ok := a.builder.(payload.Closer); ok {
		return closer.Close(ctx)
	}
	return nil
}

func cleanupError(start time.Time, err error) bench.CleanupResult {
	return bench.CleanupResult{
		Attempted:  true,
		OK:         false,
		Error:      err.Error(),
		DurationNS: time.Since(start).Nanoseconds(),
	}
}

func hasOrderRefs(metadata map[string]any, field string) bool {
	if len(metadata) == 0 {
		return false
	}
	refs, ok := metadata[field]
	if !ok || refs == nil {
		return false
	}
	if values, ok := refs.([]any); ok {
		return len(values) > 0
	}
	return true
}

func cleanupMetadata(metadata map[string]any) map[string]any {
	if value, ok := metadata["reconciliation"].(map[string]any); ok {
		return copyMap(value)
	}
	return nil
}

func cleanupDescription(metadata map[string]any) string {
	value, _ := metadata["cleanup"].(string)
	return value
}

func copyMap(value map[string]any) map[string]any {
	if len(value) == 0 {
		return nil
	}
	copied := make(map[string]any, len(value))
	for key, item := range value {
		copied[key] = item
	}
	return copied
}
