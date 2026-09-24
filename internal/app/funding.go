package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/funding"
)

type fundingOptions struct {
	configPath         string
	once               bool
	dryRun             bool
	dryRunSet          bool
	confirmLive        bool
	allowInlineSecrets bool
}

func newFundingCommand() *cobra.Command {
	opts := &fundingOptions{}
	cmd := &cobra.Command{
		Use:   "funding",
		Short: "Run benchmark account funding monitors.",
	}
	monitor := &cobra.Command{
		Use:   "monitor",
		Short: "Top up venue accounts when configured balances fall below thresholds.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runFundingMonitor(cmd.Context(), cmd, opts)
		},
	}
	monitor.Flags().StringVar(&opts.configPath, "config", "examples/funding.json", "Funding monitor JSON config.")
	monitor.Flags().BoolVar(&opts.once, "once", false, "Run one funding check and exit.")
	monitor.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Override config and skip on-chain deposits.")
	monitor.Flags().BoolVar(&opts.confirmLive, "confirm-live", false, "Required before submitting on-chain deposits.")
	monitor.Flags().BoolVar(&opts.allowInlineSecrets, "allow-inline-secrets", false, "Allow secret-looking values in config. Intended only for local debugging.")
	monitor.Flags().Lookup("dry-run").NoOptDefVal = "true"
	monitor.PreRun = func(cmd *cobra.Command, _ []string) {
		opts.dryRunSet = cmd.Flags().Changed("dry-run")
	}
	cmd.AddCommand(monitor)
	return cmd
}

func runFundingMonitor(ctx context.Context, cmd *cobra.Command, opts *fundingOptions) error {
	cfg, err := loadFundingConfig(opts.configPath)
	if err != nil {
		return err
	}
	cfg = cfg.Normalized()
	if opts.dryRunSet {
		cfg.DryRun = opts.dryRun
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := prepareFundingEnvironment(cfg); err != nil {
		return err
	}
	if !opts.allowInlineSecrets {
		if err := validateFundingConfigNoInlineSecrets(cfg); err != nil {
			return err
		}
	}
	if cfg.RequiresLiveConfirmation() && !opts.confirmLive {
		return fmt.Errorf("refusing to submit funding transactions without --confirm-live")
	}
	state, err := funding.LoadState(cfg.StatePath)
	if err != nil {
		return err
	}
	runner := funding.Runner{
		Config:   cfg,
		Balances: fundingBalanceReader{allowInlineSecrets: opts.allowInlineSecrets},
		Deposit:  funding.EVMDepositor{},
		State:    state,
		Out:      cmd.OutOrStdout(),
	}
	emitProcessEvent("funding-monitor", "", "", "start", "", map[string]any{"config": opts.configPath})
	for {
		now := time.Now().UTC()
		if _, err := runner.RunOnce(ctx, now); err != nil {
			_ = funding.SaveState(cfg.StatePath, runner.State)
			emitProcessEvent("funding-monitor", "", "", "exit", err.Error(), map[string]any{"config": opts.configPath})
			return err
		}
		if err := funding.SaveState(cfg.StatePath, runner.State); err != nil {
			emitProcessEvent("funding-monitor", "", "", "exit", err.Error(), map[string]any{"config": opts.configPath, "phase": "save_state"})
			return err
		}
		if opts.once {
			emitProcessEvent("funding-monitor", "", "", "stop", "", map[string]any{"config": opts.configPath})
			return nil
		}
		timer := time.NewTimer(time.Duration(cfg.IntervalMS) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			emitProcessEvent("funding-monitor", "", "", "stop", ctx.Err().Error(), map[string]any{"config": opts.configPath})
			return nil
		case <-timer.C:
		}
	}
}

func loadFundingConfig(path string) (funding.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return funding.Config{}, err
	}
	var cfg funding.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return funding.Config{}, err
	}
	return cfg, nil
}

func prepareFundingEnvironment(cfg funding.Config) error {
	shellEnv := currentEnvKeys()
	for _, envFile := range cfg.EnvFiles {
		if envFile == "" {
			continue
		}
		values, err := godotenv.Read(envFile)
		if err != nil {
			return fmt.Errorf("load env file %q: %w", envFile, err)
		}
		for key, value := range values {
			if _, exists := shellEnv[key]; exists {
				continue
			}
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("set env %q from %q: %w", key, envFile, err)
			}
		}
	}
	return nil
}

func validateFundingConfigNoInlineSecrets(cfg funding.Config) error {
	if looksLikeInlineSecret(cfg.Wallet.PrivateKeyEnv) {
		return fmt.Errorf("wallet.private_key_env looks like an inline secret; use an environment variable name")
	}
	for _, account := range cfg.Accounts {
		if looksLikeInlineSecret(account.Deposit.PrivateKeyEnv) {
			return fmt.Errorf("%s deposit.private_key_env looks like an inline secret; use an environment variable name", account.Name)
		}
	}
	return nil
}

func looksLikeInlineSecret(value string) bool {
	return len(value) > 40 && (value[:2] == "0x" || value[:2] == "0X")
}

type fundingBalanceReader struct {
	allowInlineSecrets bool
}

func (r fundingBalanceReader) Balance(ctx context.Context, account funding.AccountConfig) (bench.BalanceSnapshot, error) {
	plan, err := prepareRunPlan(ctx, runPlanOptions{
		ConfigPath:         account.VenueConfig,
		EnvFiles:           account.EnvFiles,
		FallbackVenue:      "",
		ConfirmLive:        true,
		AllowInlineSecrets: r.allowInlineSecrets,
	})
	if err != nil {
		return bench.BalanceSnapshot{}, err
	}
	adapter, err := buildCostAdapter(plan.VenueName, plan.Config)
	if err != nil {
		return bench.BalanceSnapshot{}, err
	}
	if adapter == nil {
		return bench.BalanceSnapshot{}, fmt.Errorf("%s has no balance/cost adapter", plan.VenueName)
	}
	defer adapter.Close(ctx)
	return adapter.Balance(ctx, plan.VenueName)
}
