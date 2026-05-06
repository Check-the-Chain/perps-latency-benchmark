package cost

import (
	"cmp"
	"context"
	"fmt"
	"time"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/payload"
)

type CommandConfig struct {
	Type         string
	Command      []string
	Env          map[string]string
	Directory    string
	Timeout      time.Duration
	StaticParams map[string]any
}

type CommandAdapter struct {
	builder payload.Builder
	closer  payload.Closer
	cfg     CommandConfig
}

func NewCommandAdapter(cfg CommandConfig) (*CommandAdapter, error) {
	if len(cfg.Command) == 0 {
		return nil, fmt.Errorf("cost command is required")
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	commandCfg := payload.CommandConfig{
		Command:   cfg.Command,
		Env:       cfg.Env,
		Directory: cfg.Directory,
		Timeout:   timeout,
	}
	var builder payload.Builder
	var err error
	switch cmp.Or(cfg.Type, "persistent_command") {
	case "command":
		builder, err = payload.NewCommandBuilder(commandCfg)
	case "persistent_command":
		builder, err = payload.NewPersistentCommandBuilder(commandCfg)
	default:
		return nil, fmt.Errorf("unknown cost command type %q", cfg.Type)
	}
	if err != nil {
		return nil, err
	}
	adapter := &CommandAdapter{builder: builder, cfg: cfg}
	if closer, ok := builder.(payload.Closer); ok {
		adapter.closer = closer
	}
	return adapter, nil
}

func (a *CommandAdapter) Balance(ctx context.Context, venue string) (bench.BalanceSnapshot, error) {
	built, err := a.builder.Build(ctx, payload.Request{
		Venue: venue,
		Params: map[string]any{
			"phase":          "balance",
			"builder_params": a.cfg.StaticParams,
		},
	})
	if err != nil {
		return bench.BalanceSnapshot{}, err
	}
	balance, err := decodeBalance(built.Metadata["balance"])
	if err != nil {
		return bench.BalanceSnapshot{}, err
	}
	if balance.Venue == "" {
		balance.Venue = venue
	}
	return balance, nil
}

func (a *CommandAdapter) SampleCost(ctx context.Context, sample bench.Sample, before bench.BalanceSnapshot) (bench.SampleCost, error) {
	built, err := a.builder.Build(ctx, payload.Request{
		Venue:     sample.Venue,
		Transport: sample.Transport,
		Scenario:  sample.Scenario,
		Iteration: sample.Iteration,
		BatchSize: sample.BatchSize,
		Params: map[string]any{
			"phase":          "sample_cost",
			"sample":         sample,
			"balance_before": before,
			"builder_params": a.cfg.StaticParams,
		},
	})
	if err != nil {
		return bench.SampleCost{}, err
	}
	cost, err := decodeCost(built.Metadata["cost"])
	if err != nil {
		return bench.SampleCost{}, err
	}
	if cost.Venue == "" {
		cost.Venue = sample.Venue
	}
	if cost.RunID == "" {
		cost.RunID = sample.RunID
	}
	if cost.CompletedAt.IsZero() {
		cost.CompletedAt = sample.CompletedAt
	}
	return cost, nil
}

func (a *CommandAdapter) Close(ctx context.Context) error {
	if a.closer == nil {
		return nil
	}
	return a.closer.Close(ctx)
}
