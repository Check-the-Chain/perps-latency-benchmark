package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"perps-latency-benchmark/internal/accounts"
	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/venues/registry"
)

type runOptions struct {
	configPath string
	envFiles   []string

	venue                 string
	runID                 string
	scenario              string
	iterations            int
	warmups               int
	batchSize             int
	ratePerSecond         float64
	maxInFlight           int
	latencyMode           string
	measurementMode       string
	confirmationTimeoutMS int
	stopOnError           bool
	cleanup               bool
	cleanupMode           string
	cleanupScope          string
	cleanupTimeoutMS      int

	timeoutMS           int
	maxIdleConns        int
	maxIdleConnsPerHost int
	disableCompression  bool

	url             string
	batchURL        string
	wsURL           string
	wsBatchURL      string
	transport       string
	method          string
	body            string
	bodyFile        string
	batchBody       string
	batchBodyFile   string
	wsBody          string
	wsBodyFile      string
	wsBatchBody     string
	wsBatchBodyFile string
	headers         []string

	transports         string
	mockLatencyMS      int
	confirmLive        bool
	allowInlineSecrets bool
	outputPath         string
	csvPath            string
}

func addRunFlags(cmd *cobra.Command, opts *runOptions) {
	flags := cmd.Flags()
	flags.StringVar(&opts.configPath, "config", "", "JSON config file.")
	flags.StringArrayVar(&opts.envFiles, "env-file", nil, "Load dotenv credentials file before running. Repeatable; shell environment wins.")
	flags.StringVar(&opts.venue, "venue", "", "Venue: mock, http, "+strings.Join(registry.Names(), ", ")+".")
	flags.StringVar(&opts.runID, "run-id", "", "Run identifier used in output and generated client order IDs.")
	flags.StringVar(&opts.scenario, "scenario", "", "Scenario: single or batch.")
	flags.IntVar(&opts.iterations, "iterations", 0, "Measured iterations.")
	flags.IntVar(&opts.warmups, "warmups", 0, "Warmup iterations excluded from stats.")
	flags.IntVar(&opts.batchSize, "batch-size", 0, "Orders per batch scenario.")
	flags.Float64Var(&opts.ratePerSecond, "rate", 0, "Open-loop fixed request rate per second. Default 0 runs closed-loop sequential mode.")
	flags.IntVar(&opts.maxInFlight, "max-in-flight", 0, "Maximum open-loop requests in flight.")
	flags.StringVar(&opts.latencyMode, "latency-mode", "", "Latency metric: total or ttfb.")
	flags.StringVar(&opts.measurementMode, "measurement-mode", "", "Measurement mode: ack or ws_confirmation.")
	flags.IntVar(&opts.confirmationTimeoutMS, "confirmation-timeout-ms", 0, "WebSocket confirmation timeout in milliseconds.")
	flags.BoolVar(&opts.stopOnError, "stop-on-error", false, "Stop after the first failed sample.")
	flags.BoolVar(&opts.cleanup, "cleanup", false, "Cancel benchmark orders outside the measured latency window.")
	flags.StringVar(&opts.cleanupMode, "cleanup-mode", "", "Cleanup mode: best_effort or strict.")
	flags.StringVar(&opts.cleanupScope, "cleanup-scope", "", "Cleanup scope: after_sample.")
	flags.IntVar(&opts.cleanupTimeoutMS, "cleanup-timeout-ms", 0, "Cleanup builder timeout in milliseconds.")

	flags.IntVar(&opts.timeoutMS, "timeout-ms", 0, "HTTP client timeout in milliseconds.")
	flags.IntVar(&opts.maxIdleConns, "max-idle-conns", 0, "HTTP transport MaxIdleConns.")
	flags.IntVar(&opts.maxIdleConnsPerHost, "max-idle-conns-per-host", 0, "HTTP transport MaxIdleConnsPerHost.")
	flags.BoolVar(&opts.disableCompression, "disable-compression", false, "Disable HTTP compression.")

	flags.StringVar(&opts.method, "method", "", "HTTP method for prebuilt requests.")
	flags.StringVar(&opts.transport, "transport", "", "Transport: http, https, or websocket.")
	flags.StringVar(&opts.url, "url", "", "HTTP URL for single/http requests.")
	flags.StringVar(&opts.batchURL, "batch-url", "", "HTTP URL for batch requests.")
	flags.StringVar(&opts.wsURL, "ws-url", "", "WebSocket URL for single/websocket requests.")
	flags.StringVar(&opts.wsBatchURL, "ws-batch-url", "", "WebSocket URL for batch/websocket requests.")
	flags.StringVar(&opts.body, "body", "", "Prebuilt HTTP request body.")
	flags.StringVar(&opts.bodyFile, "body-file", "", "File containing prebuilt HTTP request body.")
	flags.StringVar(&opts.batchBody, "batch-body", "", "Prebuilt HTTP batch request body.")
	flags.StringVar(&opts.batchBodyFile, "batch-body-file", "", "File containing prebuilt HTTP batch request body.")
	flags.StringVar(&opts.wsBody, "ws-body", "", "Prebuilt WebSocket message body.")
	flags.StringVar(&opts.wsBodyFile, "ws-body-file", "", "File containing prebuilt WebSocket message body.")
	flags.StringVar(&opts.wsBatchBody, "ws-batch-body", "", "Prebuilt WebSocket batch message body.")
	flags.StringVar(&opts.wsBatchBodyFile, "ws-batch-body-file", "", "File containing prebuilt WebSocket batch message body.")
	flags.StringArrayVar(&opts.headers, "header", nil, "Transport header as Key=Value. Repeatable.")

	flags.IntVar(&opts.mockLatencyMS, "mock-latency-ms", 0, "Mock server response delay.")
	flags.BoolVar(&opts.confirmLive, "confirm-live", false, "Required for non-mock venues.")
	flags.BoolVar(&opts.allowInlineSecrets, "allow-inline-secrets", false, "Allow credentials directly in JSON config. Intended only for local debugging.")
	flags.StringVar(&opts.outputPath, "output", "", "Write JSON result to path.")
	flags.StringVar(&opts.csvPath, "csv", "", "Write CSV samples to path.")
}

func runBenchmark(ctx context.Context, cmd *cobra.Command, opts *runOptions) error {
	cfg, err := loadFileConfig(opts.configPath)
	if err != nil {
		return err
	}
	applyFlagOverrides(cmd, opts, &cfg)
	normalizeFileConfig(&cfg)
	if err := prepareRuntimeEnvironment(cfg, opts); err != nil {
		return err
	}

	venueName := cfg.Venue
	if venueName == "" {
		venueName = "mock"
	}
	venueName = strings.ToLower(venueName)
	if venueName != "mock" && !opts.confirmLive {
		return fmt.Errorf("refusing to run live venue %q without --confirm-live", venueName)
	}
	if err := validateRunConfig(venueName, cfg); err != nil {
		return err
	}
	if err := validateLifecycleForRun(venueName, cfg); err != nil {
		return err
	}
	if err := validateCleanupForRun(venueName, cfg); err != nil {
		return err
	}
	if err := checkAccountsForRun(venueName, cfg); err != nil {
		return err
	}

	lock, err := acquireRunLock(venueName, cfg)
	if err != nil {
		return err
	}
	defer lock.Release()

	result, err := runWithConfig(ctx, venueName, cfg)
	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.OutOrStdout(), bench.FormatSummary(result))
	if opts.outputPath != "" {
		if err := bench.WriteJSON(opts.outputPath, result); err != nil {
			return err
		}
	}
	if opts.csvPath != "" {
		if err := bench.WriteCSV(opts.csvPath, result.Samples); err != nil {
			return err
		}
	}
	return nil
}

func normalizedVenue(name string, fallback string) string {
	if name == "" {
		name = fallback
	}
	return strings.ToLower(name)
}

func runTransportComparison(ctx context.Context, cmd *cobra.Command, opts *runOptions) error {
	cfg, err := loadFileConfig(opts.configPath)
	if err != nil {
		return err
	}
	applyFlagOverrides(cmd, opts, &cfg)
	normalizeFileConfig(&cfg)
	if err := prepareRuntimeEnvironment(cfg, opts); err != nil {
		return err
	}

	venueName := cfg.Venue
	if venueName == "" {
		venueName = "http"
	}
	venueName = strings.ToLower(venueName)
	if venueName != "mock" && !opts.confirmLive {
		return fmt.Errorf("refusing to run live venue %q without --confirm-live", venueName)
	}
	if err := checkAccountsForRun(venueName, cfg); err != nil {
		return err
	}

	transports := parseTransports(opts.transports)
	if len(transports) == 0 {
		return fmt.Errorf("no transports requested")
	}

	benchConfig := cfg.Benchmark.toBenchConfig()
	if benchConfig.RunID == "" {
		benchConfig.RunID = bench.NewRunID()
	}
	cfg.Benchmark.RunID = benchConfig.RunID
	if benchConfig.Warmups == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: compare-transports is running with warmups=0; first HTTPS sample may include connection/TLS setup while WebSocket connect is prepared outside the timed send path")
	}
	comparison := bench.ComparisonResult{
		Venue:           venueName,
		RunID:           benchConfig.RunID,
		Scenario:        benchConfig.Scenario,
		LatencyMode:     benchConfig.LatencyMode,
		MeasurementMode: benchConfig.MeasurementMode,
	}
	lock, err := acquireRunLock(venueName, cfg)
	if err != nil {
		return err
	}
	defer lock.Release()

	for _, transport := range transports {
		variantCfg := cloneFileConfig(cfg)
		setTransport(&variantCfg, venueName, transport)
		variantCfg.Benchmark.RunID = benchConfig.RunID + "-" + transport
		if err := validateRunConfig(venueName, variantCfg); err != nil {
			return err
		}
		if err := validateLifecycleForRun(venueName, variantCfg); err != nil {
			return err
		}
		if err := validateCleanupForRun(venueName, variantCfg); err != nil {
			return err
		}
		result, err := runWithConfig(ctx, venueName, variantCfg)
		if err != nil {
			return fmt.Errorf("%s: %w", transport, err)
		}
		comparison.Results = append(comparison.Results, result)
		fmt.Fprintf(cmd.OutOrStdout(), "\n[%s]\n%s\n", transport, bench.FormatSummary(result))
	}

	if opts.outputPath != "" {
		if err := bench.WriteComparisonJSON(opts.outputPath, comparison); err != nil {
			return err
		}
	}
	if opts.csvPath != "" {
		var samples []bench.Sample
		for _, result := range comparison.Results {
			samples = append(samples, result.Samples...)
		}
		if err := bench.WriteCSV(opts.csvPath, samples); err != nil {
			return err
		}
	}
	return nil
}

func setTransport(cfg *fileConfig, venueName string, transport string) {
	if definition, ok := registry.Lookup(venueName); ok {
		venueName = definition.Name
	}
	cfg.Request.Transport = transport
	if cfg.Venues != nil {
		venueCfg := cfg.Venues[venueName]
		venueCfg.Request.Transport = transport
		cfg.Venues[venueName] = venueCfg
	}
}

func validateRunConfig(venueName string, cfg fileConfig) error {
	switch cfg.Benchmark.toBenchConfig().MeasurementMode {
	case bench.MeasurementModeAck, bench.MeasurementModeWSConfirmation:
	default:
		return fmt.Errorf("benchmark.measurement_mode must be ack or ws_confirmation")
	}
	if venueName == "mock" || venueName == "http" {
		return validateBuilderConfig(cfg.Request.Builder)
	}
	definition, ok := registry.Lookup(venueName)
	if !ok {
		return nil
	}
	benchConfig := cfg.Benchmark.toBenchConfig()
	venueCfg := cfgForVenue(definition.Name, cfg)
	request := mergeRequest(cfg.Request, venueCfg.Request)
	transport := normalizedTransport(request.Transport)
	if !definition.Supports(transport, benchConfig.Scenario) {
		return fmt.Errorf("%s does not support %s %s order submission in the verified venue definition", definition.Name, transport, benchConfig.Scenario)
	}
	if err := validateBuilderConfig(request.Builder); err != nil {
		return err
	}
	if request.Builder.Type != "" {
		params := definition.BuilderParams.Merge(request.Builder.Params)
		for _, key := range definition.BuilderParams.Required {
			if missingParam(params, key) {
				return fmt.Errorf("%s builder missing required params.%s", definition.Name, key)
			}
		}
	}
	return nil
}

func validateBuilderConfig(cfg builderConfig) error {
	if cfg.Type == "" {
		return nil
	}
	switch strings.ToLower(cfg.Type) {
	case "command", "persistent_command":
	default:
		return fmt.Errorf("unknown builder type %q", cfg.Type)
	}
	if len(cfg.Command) == 0 {
		return fmt.Errorf("builder command is required")
	}
	return nil
}

func missingParam(params map[string]any, key string) bool {
	if len(params) == 0 {
		return true
	}
	value, ok := params[key]
	if !ok || value == nil {
		return true
	}
	if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
		return true
	}
	return false
}

func normalizedTransport(transport string) string {
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case "", "http", "https":
		return "http"
	case "ws", "wss", "websocket":
		return "websocket"
	default:
		return strings.ToLower(strings.TrimSpace(transport))
	}
}

func validateLifecycleForRun(venueName string, cfg fileConfig) error {
	if venueName == "mock" {
		return nil
	}
	request := cfg.Request
	params := request.Builder.Params
	if definition, ok := registry.Lookup(venueName); ok {
		venueCfg := cfgForVenue(definition.Name, cfg)
		request = mergeRequest(cfg.Request, venueCfg.Request)
		params = definition.BuilderParams.Merge(request.Builder.Params)
	}
	profile := lifecycle.ProfileFromParams(params)
	if err := lifecycle.ValidateRisk(cfg.Risk, profile); err != nil {
		return fmt.Errorf("risk validation failed: %w", err)
	}
	if lifecycle.FillLikely(profile) && cfg.Risk.AllowFill {
		if !supportsNeutralization(venueName) {
			return fmt.Errorf("risk validation failed: fill-likely order profiles require a venue cleanup/neutralization adapter; %s does not have one wired yet", venueName)
		}
		cleanupCfg := cfg.Cleanup.toBenchCleanupConfig()
		if !cleanupCfg.Enabled || cleanupCfg.Mode != bench.CleanupModeStrict || cleanupCfg.Scope != bench.CleanupScopeAfterSample {
			return fmt.Errorf("risk validation failed: fill-likely order profiles require strict after-sample cleanup")
		}
	}
	return nil
}

func supportsNeutralization(venueName string) bool {
	switch venueName {
	case "hyperliquid", "lighter":
		return true
	default:
		return false
	}
}

func validateCleanupForRun(venueName string, cfg fileConfig) error {
	cleanupCfg := cfg.Cleanup.toBenchCleanupConfig()
	if !cleanupCfg.Enabled {
		return nil
	}
	switch cleanupCfg.Mode {
	case bench.CleanupModeBestEffort, bench.CleanupModeStrict:
	default:
		return fmt.Errorf("cleanup.mode must be best_effort or strict")
	}
	if cleanupCfg.Scope != bench.CleanupScopeAfterSample {
		return fmt.Errorf("cleanup.scope must be after_sample")
	}
	switch venueName {
	case "hyperliquid", "lighter":
		return nil
	default:
		return fmt.Errorf("%s does not have a cleanup adapter wired yet", venueName)
	}
}

func checkAccountsForRun(venueName string, cfg fileConfig) error {
	if venueName == "mock" || venueName == "http" {
		return nil
	}
	definition, ok := registry.Lookup(venueName)
	if !ok {
		return nil
	}
	venueCfg := cfgForVenue(definition.Name, cfg)
	request := mergeRequest(cfg.Request, venueCfg.Request)
	if request.Builder.Type == "" {
		return nil
	}
	spec, ok := accounts.Spec(definition.Name)
	if !ok {
		return nil
	}
	if err := accounts.Check([]accounts.VenueSpec{spec}); err != nil {
		return fmt.Errorf("account setup check failed for %s: %w", definition.Name, err)
	}
	return nil
}

func runWithConfig(ctx context.Context, venueName string, cfg fileConfig) (bench.Result, error) {
	benchConfig := cfg.Benchmark.toBenchConfig()
	if benchConfig.RunID == "" {
		benchConfig.RunID = bench.NewRunID()
	}
	cfg.Benchmark.RunID = benchConfig.RunID
	injectRunID(&cfg, venueName, benchConfig.RunID)

	client := netlatency.NewClient(netlatency.ClientConfig{
		Timeout:             durationMS(cfg.HTTP.TimeoutMS),
		MaxIdleConns:        cfg.HTTP.MaxIdleConns,
		MaxIdleConnsPerHost: cfg.HTTP.MaxIdleConnsPerHost,
		DisableCompression:  cfg.HTTP.DisableCompression,
	})
	defer client.CloseIdleConnections()

	venue, err := buildVenue(venueName, cfg)
	if err != nil {
		return bench.Result{}, err
	}

	benchConfig.Cleanup = cfg.Cleanup.toBenchCleanupConfig()
	cleanupAdapter, err := buildCleanupAdapter(venueName, cfg, client)
	if err != nil {
		return bench.Result{}, err
	}

	return bench.Runner{
		Config:  benchConfig,
		Client:  client,
		Venue:   venue,
		Cleanup: cleanupAdapter,
	}.Run(ctx)
}

func parseTransports(raw string) []string {
	parts := strings.Split(raw, ",")
	transports := make([]string, 0, len(parts))
	for _, part := range parts {
		transport := strings.TrimSpace(strings.ToLower(part))
		if transport == "" {
			continue
		}
		if transport == "ws" || transport == "wss" {
			transport = "websocket"
		}
		transports = append(transports, transport)
	}
	return transports
}

func durationMS(ms int) time.Duration {
	if ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}
