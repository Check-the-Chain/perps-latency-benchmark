package cleanup

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"strings"
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
	prepared, err := a.PrepareAfterSample(ctx, sample)
	if err != nil {
		return cleanupError(time.Now(), err)
	}
	if prepared.Result != nil {
		return *prepared.Result
	}
	if prepared.Execute == nil {
		return bench.CleanupResult{Attempted: false, OK: true}
	}
	cleanup := prepared.Execute(ctx)
	cleanup.PreparedNS = prepared.PreparedNS
	return cleanup
}

func (a *CommandAdapter) PrepareAfterSample(ctx context.Context, sample bench.Sample) (bench.PreparedCleanup, error) {
	start := time.Now()
	orderRefs := sample.OrderRefs
	if len(orderRefs) == 0 {
		orderRefs = bench.OrderRefsFromMetadata(sample.Metadata, cmp.Or(a.cfg.OrderRefsField, "cleanup_orders"))
	}
	if a.cfg.SkipNoRefs && len(orderRefs) == 0 {
		return bench.PreparedCleanup{
			Result:     &bench.CleanupResult{Attempted: false, OK: true, Description: "no cleanup order refs"},
			PreparedNS: time.Since(start).Nanoseconds(),
		}, nil
	}
	metadata := copyMap(sample.Metadata)
	if len(orderRefs) > 0 {
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata[cmp.Or(a.cfg.OrderRefsField, "cleanup_orders")] = bench.OrderRefsToMetadata(orderRefs)
	}

	params := map[string]any{
		"phase":          "after_sample",
		"sample":         sample,
		"order_refs":     orderRefs,
		"metadata":       metadata,
		"run_metadata":   a.currentRunMetadata(),
		"builder_params": a.cfg.StaticParams,
	}
	req := payload.Request{
		Venue:     sample.Venue,
		Transport: sample.Transport,
		Scenario:  sample.Scenario,
		Iteration: sample.Iteration,
		BatchSize: sample.BatchSize,
		Params:    params,
	}
	prepared, err := a.prepare(ctx, start, req)
	if err != nil {
		return bench.PreparedCleanup{}, err
	}
	return a.withRetryableAfterSampleCleanup(req, prepared), nil
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
	prepared, err := a.prepare(ctx, start, req)
	if err != nil {
		return cleanupError(start, err)
	}
	if prepared.Result != nil {
		return *prepared.Result
	}
	cleanup := prepared.Execute(ctx)
	cleanup.PreparedNS = prepared.PreparedNS
	return cleanup
}

func (a *CommandAdapter) prepare(ctx context.Context, start time.Time, req payload.Request) (bench.PreparedCleanup, error) {
	built, err := a.builder.Build(ctx, req)
	if err != nil {
		return bench.PreparedCleanup{}, err
	}
	if built.Cleanup != nil {
		built.Cleanup.DurationNS = time.Since(start).Nanoseconds()
		built.Cleanup.PreparedNS = time.Since(start).Nanoseconds()
		return bench.PreparedCleanup{Result: built.Cleanup, PreparedNS: built.Cleanup.PreparedNS}, nil
	}

	body, err := payload.Bytes(built.Body, built.BodyBase64, nil)
	if err != nil {
		return bench.PreparedCleanup{}, err
	}
	preparedNS := time.Since(start).Nanoseconds()
	if len(body) == 0 {
		return bench.PreparedCleanup{Result: &bench.CleanupResult{
			Attempted:   false,
			OK:          true,
			PreparedNS:  preparedNS,
			DurationNS:  preparedNS,
			Description: cmp.Or(a.cfg.Description, "cleanup skipped"),
		}, PreparedNS: preparedNS}, nil
	}
	template := netlatency.RequestTemplate{
		Method: cmp.Or(built.Method, a.cfg.Method, http.MethodPost),
		URL:    cmp.Or(built.URL, a.cfg.URL),
		Header: payload.MergeHeaders(a.headers, built.Headers),
		Body:   body,
	}
	return bench.PreparedCleanup{
		PreparedNS: preparedNS,
		Execute: func(execCtx context.Context) bench.CleanupResult {
			executeStart := time.Now()
			result, err := a.cfg.Client.Do(execCtx, template)
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
				PreparedNS:  preparedNS,
				DurationNS:  time.Since(executeStart).Nanoseconds(),
				BytesRead:   result.BytesRead,
				Description: cmp.Or(cleanupDescription(built.Metadata), a.cfg.Description, "cleanup"),
				Metadata:    cleanupMetadata(built.Metadata),
				Trace:       result.Trace,
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
		},
	}, nil
}

func (a *CommandAdapter) withRetryableAfterSampleCleanup(req payload.Request, prepared bench.PreparedCleanup) bench.PreparedCleanup {
	if prepared.Result != nil || prepared.Execute == nil {
		return prepared
	}
	firstExecute := prepared.Execute
	prepared.Execute = func(execCtx context.Context) bench.CleanupResult {
		cleanup := firstExecute(execCtx)
		if !retryableCleanupResult(cleanup) {
			return cleanup
		}
		firstError := cleanup.Error
		for attempt := 1; attempt <= 2; attempt++ {
			if attempt > 1 {
				select {
				case <-execCtx.Done():
					cleanup.Error = appendCleanupError(cleanup.Error, execCtx.Err().Error())
					return cleanup
				case <-time.After(250 * time.Millisecond):
				}
			}
			retryPrepared, err := a.prepare(execCtx, time.Now(), req)
			if err != nil {
				cleanup.Error = appendCleanupError(cleanup.Error, err.Error())
				return cleanup
			}
			retryCleanup := runPreparedCleanup(execCtx, retryPrepared)
			annotateCleanupRetry(&retryCleanup, attempt, firstError)
			cleanup = retryCleanup
			if cleanup.OK || !retryableCleanupResult(cleanup) {
				return cleanup
			}
		}
		return cleanup
	}
	return prepared
}

func runPreparedCleanup(ctx context.Context, prepared bench.PreparedCleanup) bench.CleanupResult {
	if prepared.Result != nil {
		cleanup := *prepared.Result
		cleanup.PreparedNS = prepared.PreparedNS
		return cleanup
	}
	if prepared.Execute == nil {
		return bench.CleanupResult{Attempted: false, OK: true, PreparedNS: prepared.PreparedNS}
	}
	cleanup := prepared.Execute(ctx)
	cleanup.PreparedNS = prepared.PreparedNS
	return cleanup
}

func retryableCleanupResult(cleanup bench.CleanupResult) bool {
	if cleanup.OK {
		return false
	}
	text := strings.ToLower(cleanup.Error + " " + cleanup.Description)
	return strings.Contains(text, "nonce_error") ||
		strings.Contains(text, "nonce") ||
		strings.Contains(text, "timestamp") ||
		strings.Contains(text, "device time") ||
		strings.Contains(text, "time must match") ||
		strings.Contains(text, "recvwindow")
}

func annotateCleanupRetry(cleanup *bench.CleanupResult, attempt int, firstError string) {
	if cleanup.Metadata == nil {
		cleanup.Metadata = map[string]any{}
	}
	cleanup.Metadata["cleanup_retry_count"] = attempt
	if firstError != "" {
		cleanup.Metadata["cleanup_first_error"] = firstError
	}
}

func appendCleanupError(current string, next string) string {
	if current == "" {
		return next
	}
	if next == "" {
		return current
	}
	return current + "; retry: " + next
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
	if len(metadata) == 0 {
		return nil
	}
	copied := copyMap(metadata)
	delete(copied, "cleanup")
	return copied
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
