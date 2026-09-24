package app

import (
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

type flagReader interface {
	Changed(name string) bool
}

func applyFlagOverrides(cmd *cobra.Command, opts *runOptions, cfg *fileConfig) {
	flags := cmd.Flags()
	if flags.Changed("venue") {
		cfg.Venue = opts.venue
	}
	if flags.Changed("run-id") {
		cfg.Benchmark.RunID = opts.runID
	}
	if flags.Changed("env-file") {
		cfg.EnvFiles = append(cfg.EnvFiles, opts.envFiles...)
	}
	if flags.Changed("scenario") {
		cfg.Benchmark.Scenario = opts.scenario
	}
	if flags.Changed("iterations") {
		cfg.Benchmark.Iterations = opts.iterations
	}
	if flags.Changed("warmups") {
		cfg.Benchmark.Warmups = opts.warmups
	}
	if flags.Changed("batch-size") {
		cfg.Benchmark.BatchSize = opts.batchSize
	}
	if flags.Changed("rate") {
		cfg.Benchmark.RatePerSecond = opts.ratePerSecond
	}
	if flags.Changed("max-in-flight") {
		cfg.Benchmark.MaxInFlight = opts.maxInFlight
	}
	if flags.Changed("latency-mode") {
		cfg.Benchmark.LatencyMode = opts.latencyMode
	}
	if flags.Changed("measurement-mode") {
		cfg.Benchmark.MeasurementMode = opts.measurementMode
	}
	if flags.Changed("confirmation-timeout-ms") {
		cfg.Benchmark.ConfirmationTimeoutMS = opts.confirmationTimeoutMS
	}
	if flags.Changed("stop-on-error") {
		cfg.Benchmark.StopOnError = opts.stopOnError
	}
	if flags.Changed("cleanup") {
		cfg.Cleanup.Enabled = opts.cleanup
	}
	if flags.Changed("cleanup-mode") {
		cfg.Cleanup.Mode = opts.cleanupMode
	}
	if flags.Changed("cleanup-scope") {
		cfg.Cleanup.Scope = opts.cleanupScope
	}
	if flags.Changed("cleanup-timeout-ms") {
		cfg.Cleanup.TimeoutMS = opts.cleanupTimeoutMS
	}
	if flags.Changed("timeout-ms") {
		cfg.HTTP.TimeoutMS = opts.timeoutMS
	}
	if flags.Changed("max-idle-conns") {
		cfg.HTTP.MaxIdleConns = opts.maxIdleConns
	}
	if flags.Changed("max-idle-conns-per-host") {
		cfg.HTTP.MaxIdleConnsPerHost = opts.maxIdleConnsPerHost
	}
	if flags.Changed("disable-compression") {
		cfg.HTTP.DisableCompression = opts.disableCompression
	}
	if flags.Changed("mock-latency-ms") {
		cfg.Mock.LatencyMS = opts.mockLatencyMS
	}
	applyRequestFlagOverrides(flags, opts, &cfg.Request)
}

func applyRequestFlagOverrides(flags flagReader, opts *runOptions, req *requestConfig) {
	if flags.Changed("method") {
		req.Method = opts.method
	}
	if flags.Changed("transport") {
		req.Transport = opts.transport
	}
	if flags.Changed("url") {
		req.URL = opts.url
	}
	if flags.Changed("batch-url") {
		req.BatchURL = opts.batchURL
	}
	if flags.Changed("ws-url") {
		req.WSURL = opts.wsURL
	}
	if flags.Changed("ws-batch-url") {
		req.WSBatchURL = opts.wsBatchURL
	}
	if flags.Changed("body") {
		req.Body = opts.body
	}
	if flags.Changed("body-file") {
		req.BodyFile = opts.bodyFile
	}
	if flags.Changed("batch-body") {
		req.BatchBody = opts.batchBody
	}
	if flags.Changed("batch-body-file") {
		req.BatchBodyFile = opts.batchBodyFile
	}
	if flags.Changed("ws-body") {
		req.WSBody = opts.wsBody
	}
	if flags.Changed("ws-body-file") {
		req.WSBodyFile = opts.wsBodyFile
	}
	if flags.Changed("ws-batch-body") {
		req.WSBatchBody = opts.wsBatchBody
	}
	if flags.Changed("ws-batch-body-file") {
		req.WSBatchBodyFile = opts.wsBatchBodyFile
	}
	if flags.Changed("header") {
		if req.Headers == nil {
			req.Headers = make(map[string]string)
		}
		for _, header := range opts.headers {
			key, value, ok := strings.Cut(header, "=")
			if ok {
				req.Headers[key] = value
			}
		}
	}
}

func cfgForVenue(name string, cfg fileConfig) venueConfig {
	if cfg.Venues != nil {
		return cfg.Venues[name]
	}
	return venueConfig{}
}

func mergeRequest(base requestConfig, overlay requestConfig) requestConfig {
	if overlay.Method == "" {
		overlay.Method = base.Method
	}
	if overlay.Transport == "" {
		overlay.Transport = base.Transport
	}
	if overlay.URL == "" {
		overlay.URL = base.URL
	}
	if overlay.BatchURL == "" {
		overlay.BatchURL = base.BatchURL
	}
	if overlay.WSURL == "" {
		overlay.WSURL = base.WSURL
	}
	if overlay.WSBatchURL == "" {
		overlay.WSBatchURL = base.WSBatchURL
	}
	if overlay.Body == "" {
		overlay.Body = base.Body
	}
	if overlay.BodyFile == "" {
		overlay.BodyFile = base.BodyFile
	}
	if overlay.BatchBody == "" {
		overlay.BatchBody = base.BatchBody
	}
	if overlay.BatchBodyFile == "" {
		overlay.BatchBodyFile = base.BatchBodyFile
	}
	if overlay.WSBody == "" {
		overlay.WSBody = base.WSBody
	}
	if overlay.WSBodyFile == "" {
		overlay.WSBodyFile = base.WSBodyFile
	}
	if overlay.WSBatchBody == "" {
		overlay.WSBatchBody = base.WSBatchBody
	}
	if overlay.WSBatchBodyFile == "" {
		overlay.WSBatchBodyFile = base.WSBatchBodyFile
	}
	if overlay.Builder.Type == "" {
		overlay.Builder = base.Builder
	} else {
		if len(overlay.Builder.Command) == 0 {
			overlay.Builder.Command = base.Builder.Command
		}
		if overlay.Builder.Env == nil {
			overlay.Builder.Env = base.Builder.Env
		} else {
			for key, value := range base.Builder.Env {
				if _, exists := overlay.Builder.Env[key]; !exists {
					overlay.Builder.Env[key] = value
				}
			}
		}
		if overlay.Builder.TimeoutMS == 0 {
			overlay.Builder.TimeoutMS = base.Builder.TimeoutMS
		}
		if overlay.Builder.Directory == "" {
			overlay.Builder.Directory = base.Builder.Directory
		}
		if overlay.Builder.Params == nil {
			overlay.Builder.Params = base.Builder.Params
		} else {
			for key, value := range base.Builder.Params {
				if _, exists := overlay.Builder.Params[key]; !exists {
					overlay.Builder.Params[key] = value
				}
			}
		}
	}
	if overlay.Headers == nil {
		overlay.Headers = base.Headers
	} else {
		for key, value := range base.Headers {
			if _, exists := overlay.Headers[key]; !exists {
				overlay.Headers[key] = value
			}
		}
	}
	return overlay
}

func cloneFileConfig(in fileConfig) fileConfig {
	out := in
	out.EnvFiles = slices.Clone(in.EnvFiles)
	out.Request = cloneRequestConfig(in.Request)
	if in.Venues != nil {
		out.Venues = make(map[string]venueConfig, len(in.Venues))
		for name, cfg := range in.Venues {
			cfg.Request = cloneRequestConfig(cfg.Request)
			out.Venues[name] = cfg
		}
	}
	out.Hyperliquid.Request = cloneRequestConfig(in.Hyperliquid.Request)
	out.Lighter.Request = cloneRequestConfig(in.Lighter.Request)
	return out
}

func cloneRequestConfig(in requestConfig) requestConfig {
	out := in
	out.Headers = maps.Clone(in.Headers)
	out.Builder = cloneBuilderConfig(in.Builder)
	return out
}

func cloneBuilderConfig(in builderConfig) builderConfig {
	out := in
	out.Command = slices.Clone(in.Command)
	out.Env = maps.Clone(in.Env)
	out.Params = maps.Clone(in.Params)
	return out
}
