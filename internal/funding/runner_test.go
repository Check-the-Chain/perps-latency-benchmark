package funding

import (
	"context"
	"errors"
	"testing"
	"time"

	"perps-latency-benchmark/internal/bench"
)

func TestRecordResultPersistsAttemptState(t *testing.T) {
	now := time.Unix(100, 0)
	state := State{}
	plan := DepositPlan{
		Account: AccountConfig{Name: "hyperliquid"},
		Balance: bench.BalanceSnapshot{
			BalanceUSD: 10,
		},
		Amount: 25,
		DryRun: true,
		Now:    now,
	}
	RecordResult(&state, plan, DepositResult{
		Status:      "dry_run",
		AmountUSDC:  25,
		CompletedAt: now.Add(time.Second),
	}, nil)

	account := state.Accounts["hyperliquid"]
	if account.LastStatus != "dry_run" || account.LastAmount != 25 || account.LastBalance != 10 {
		t.Fatalf("account state = %+v", account)
	}
	if !account.LastAttemptAt.IsZero() {
		t.Fatalf("dry run should not set live cooldown attempt time: %+v", account)
	}
}

func TestDailySpentIgnoresDryRunsAndOldEvents(t *testing.T) {
	now := time.Unix(200000, 0)
	state := State{Events: []DepositEvent{
		{AmountUSDC: 10, Status: "success", CompletedAt: now.Add(-time.Hour)},
		{AmountUSDC: 20, Status: "dry_run", DryRun: true, CompletedAt: now.Add(-time.Hour)},
		{AmountUSDC: 30, Status: "success", CompletedAt: now.Add(-25 * time.Hour)},
	}}

	if got := DailySpent(state, now); got != 10 {
		t.Fatalf("daily spent = %f, want 10", got)
	}
}

func TestRunnerIsolatesAccountFailures(t *testing.T) {
	now := time.Unix(300, 0)
	runner := Runner{
		Config: Config{
			DryRun: true,
			Accounts: []AccountConfig{
				{Name: "balance-error", MinBalanceUSD: 10, TargetBalanceUSD: 20},
				{Name: "deposit-error", MinBalanceUSD: 10, TargetBalanceUSD: 20},
				{Name: "success", MinBalanceUSD: 10, TargetBalanceUSD: 20},
			},
		},
		Balances: fakeBalanceReader{
			balances: map[string]bench.BalanceSnapshot{
				"deposit-error": {BalanceUSD: 5},
				"success":       {BalanceUSD: 5},
			},
			errors: map[string]error{
				"balance-error": errors.New("balance unavailable"),
			},
		},
		Deposit: fakeDepositor{
			errors: map[string]error{
				"deposit-error": errors.New("deposit unavailable"),
			},
		},
	}
	results, err := runner.RunOnce(context.Background(), now)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(results) != 1 || results[0].AccountName != "success" {
		t.Fatalf("results = %+v, want only success account", results)
	}
	if got := runner.State.Accounts["balance-error"]; got.LastStatus != "error" || got.LastError != "balance unavailable" {
		t.Fatalf("balance-error state = %+v", got)
	}
	if got := runner.State.Accounts["deposit-error"]; got.LastStatus != "error" || got.LastError != "deposit unavailable" {
		t.Fatalf("deposit-error state = %+v", got)
	}
	if got := runner.State.Accounts["success"]; got.LastStatus != "dry_run" || got.LastAmount != 15 {
		t.Fatalf("success state = %+v", got)
	}
}

type fakeBalanceReader struct {
	balances map[string]bench.BalanceSnapshot
	errors   map[string]error
}

func (r fakeBalanceReader) Balance(_ context.Context, account AccountConfig) (bench.BalanceSnapshot, error) {
	if err := r.errors[account.Name]; err != nil {
		return bench.BalanceSnapshot{}, err
	}
	return r.balances[account.Name], nil
}

type fakeDepositor struct {
	errors map[string]error
}

func (d fakeDepositor) Deposit(_ context.Context, plan DepositPlan) (DepositResult, error) {
	result := DepositResult{
		AccountName: plan.Account.Name,
		AmountUSDC:  plan.Amount,
		DryRun:      plan.DryRun,
		Status:      "dry_run",
		CompletedAt: plan.Now.Add(time.Second),
	}
	if err := d.errors[plan.Account.Name]; err != nil {
		return result, err
	}
	return result, nil
}
