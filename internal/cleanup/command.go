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
	Type               string
	Command            []string
	Env                map[string]string
	Directory          string
	Timeout            time.Duration
	Method             string
	URL                string
	WSURL              string
	WSBatchURL         string
	WSReadInitial      bool
	WSHeartbeat        netlatency.WebSocketHeartbeat
	Headers            map[string]string
	StaticParams       map[string]any
	Client             *netlatency.Client
	Classifier         lifecycle.Classifier
	CancelConfirmation func(context.Context, payload.Built) (*bench.Confirmation, error)
	Description        string
	SkipNoRefs         bool
	OrderRefsField     string
}

type CommandAdapter struct {
	cfg         CommandConfig
	builder     payload.Builder
	headers     http.Header
	mu          sync.Mutex
	runMetadata map[string]any
	wsMu        sync.Mutex
	wsClients   map[string]*netlatency.WebSocketClient
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
	cfg.Timeout = timeout

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

	preparedNS := time.Since(start).Nanoseconds()
	routes, hasRoute, err := cleanupRoutes(req, built, a.cfg, a.headers)
	if err != nil {
		return bench.PreparedCleanup{}, err
	}
	if !hasRoute {
		return bench.PreparedCleanup{Result: &bench.CleanupResult{
			Attempted:   false,
			OK:          true,
			PreparedNS:  preparedNS,
			DurationNS:  preparedNS,
			Description: cmp.Or(a.cfg.Description, "cleanup skipped"),
		}, PreparedNS: preparedNS}, nil
	}
	var routeErrors []string
	for _, route := range routes {
		prepared, ok, fallback, err := a.prepareCleanupRoute(ctx, start, preparedNS, route, built, routeErrors)
		if err != nil {
			return bench.PreparedCleanup{}, err
		}
		if ok {
			return prepared, nil
		}
		if fallback != "" {
			routeErrors = append(routeErrors, fallback)
		}
	}
	return bench.PreparedCleanup{}, fmt.Errorf("no cleanup route could be prepared: %s", strings.Join(routeErrors, "; "))
}

func (a *CommandAdapter) prepareCleanupRoute(ctx context.Context, start time.Time, preparedNS int64, route cleanupRoute, built payload.Built, routeErrors []string) (bench.PreparedCleanup, bool, string, error) {
	switch route.kind {
	case cleanupRouteWebSocket:
		headers := payload.MergeHeaders(a.headers, built.Headers)
		client, err := a.webSocketClient(route.wsURL, headers)
		if err != nil {
			return bench.PreparedCleanup{}, false, "", err
		}
		if err := client.EnsureConnected(ctx); err != nil {
			return bench.PreparedCleanup{}, false, "websocket prepare failed: " + err.Error(), nil
		}
		confirmation, err := a.prepareCancelConfirmation(ctx, built)
		if err != nil {
			return bench.PreparedCleanup{}, false, "", err
		}
		preparedNS = time.Since(start).Nanoseconds()
		return bench.PreparedCleanup{
			PreparedNS: preparedNS,
			Execute: func(execCtx context.Context) bench.CleanupResult {
				result, err := client.Do(execCtx, route.wsBody)
				cleanup := a.execution().resultFromNetworkWithConfirmation(execCtx, result, err, preparedNS, built, confirmation, a.cfg.Timeout)
				annotateCleanupRoute(&cleanup, string(cleanupRouteWebSocket), routeErrors)
				return cleanup
			},
		}, true, "", nil
	case cleanupRouteHTTP:
		confirmation, err := a.prepareCancelConfirmation(ctx, built)
		if err != nil {
			return bench.PreparedCleanup{}, false, "", err
		}
		return bench.PreparedCleanup{
			PreparedNS: preparedNS,
			Execute: func(execCtx context.Context) bench.CleanupResult {
				result, err := a.cfg.Client.Do(execCtx, route.http)
				cleanup := a.execution().resultFromNetworkWithConfirmation(execCtx, result, err, preparedNS, built, confirmation, a.cfg.Timeout)
				annotateCleanupRoute(&cleanup, string(cleanupRouteHTTP), routeErrors)
				return cleanup
			},
		}, true, "", nil
	default:
		return bench.PreparedCleanup{}, false, "", fmt.Errorf("unknown cleanup route kind %q", route.kind)
	}
}

func (a *CommandAdapter) prepareCancelConfirmation(ctx context.Context, built payload.Built) (*bench.Confirmation, error) {
	if a.cfg.CancelConfirmation == nil {
		return nil, nil
	}
	return a.cfg.CancelConfirmation(ctx, built)
}

func (a *CommandAdapter) webSocketClient(rawURL string, headers http.Header) (*netlatency.WebSocketClient, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("cleanup websocket url is required")
	}
	a.wsMu.Lock()
	defer a.wsMu.Unlock()
	if a.wsClients == nil {
		a.wsClients = make(map[string]*netlatency.WebSocketClient)
	}
	client := a.wsClients[rawURL]
	if client == nil {
		client = netlatency.NewWebSocketClientWithHeartbeat(rawURL, headers, a.cfg.WSReadInitial, a.cfg.WSHeartbeat)
		a.wsClients[rawURL] = client
	}
	return client, nil
}

func (a *CommandAdapter) execution() cleanupExecution {
	return cleanupExecution{classifier: a.cfg.Classifier, description: a.cfg.Description}
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
	var err error
	if closer, ok := a.builder.(payload.Closer); ok {
		err = closer.Close(ctx)
	}
	a.wsMu.Lock()
	defer a.wsMu.Unlock()
	for _, client := range a.wsClients {
		if closeErr := client.Close(); err == nil {
			err = closeErr
		}
	}
	clear(a.wsClients)
	return err
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
	delete(copied, "cancel_confirmation")
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
