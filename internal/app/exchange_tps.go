package app

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"perps-latency-benchmark/internal/exchangetps"
)

type exchangeTPSOptions struct {
	venue            string
	storePath        string
	minuteRetention  time.Duration
	asterWSURL       string
	hyperliquidWSURL string
	lighterURL       string
	flushInterval    time.Duration
	pollInterval     time.Duration
	runFor           time.Duration
}

func newCollectExchangeTPSCommand() *cobra.Command {
	opts := &exchangeTPSOptions{}
	cmd := &cobra.Command{
		Use:   "collect-exchange-tps",
		Short: "Collect compact whole-exchange TPS buckets.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runExchangeTPSCollector(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.venue, "venue", "aster", "Exchange to collect. Supported: aster, hyperliquid, lighter.")
	cmd.Flags().StringVar(&opts.storePath, "store", "data/exchange_tps.db", "SQLite path for compact exchange TPS buckets.")
	cmd.Flags().DurationVar(&opts.minuteRetention, "minute-retention", 365*24*time.Hour, "Retention for 1m bucket table. 1h rollups are retained.")
	cmd.Flags().StringVar(&opts.asterWSURL, "aster-ws-url", exchangetps.DefaultAsterWSURL, "Aster explorer WebSocket URL.")
	cmd.Flags().StringVar(&opts.hyperliquidWSURL, "hyperliquid-ws-url", exchangetps.DefaultHyperliquidWSURL, "Hyperliquid explorer WebSocket URL.")
	cmd.Flags().StringVar(&opts.lighterURL, "lighter-metrics-url", exchangetps.DefaultLighterMetricsURL, "Lighter exchangeMetrics URL.")
	cmd.Flags().DurationVar(&opts.flushInterval, "flush-interval", time.Second, "How often to flush finalized buckets.")
	cmd.Flags().DurationVar(&opts.pollInterval, "poll-interval", time.Minute, "How often to poll HTTP metric sources.")
	cmd.Flags().DurationVar(&opts.runFor, "run-for", 0, "Optional bounded runtime for smoke tests. Zero runs until interrupted.")
	return cmd
}

func runExchangeTPSCollector(ctx context.Context, opts *exchangeTPSOptions) error {
	if opts == nil {
		return errors.New("exchange TPS options are required")
	}
	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if opts.runFor > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(runCtx, opts.runFor)
		defer cancel()
	}

	err := exchangetps.RunCollector(runCtx, exchangetps.RunnerConfig{
		Venue:            opts.venue,
		StorePath:        opts.storePath,
		MinuteRetention:  opts.minuteRetention,
		AsterWSURL:       opts.asterWSURL,
		HyperliquidWSURL: opts.hyperliquidWSURL,
		LighterURL:       opts.lighterURL,
		FlushInterval:    opts.flushInterval,
		PollInterval:     opts.pollInterval,
	})
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		err = nil
	}
	return err
}
