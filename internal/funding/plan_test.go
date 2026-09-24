package funding

import (
	"strings"
	"testing"
	"time"

	"perps-latency-benchmark/internal/bench"
)

func TestPlanDepositSkipsHealthyBalance(t *testing.T) {
	decision := PlanDeposit(Config{}, AccountConfig{
		Name:             "hyperliquid",
		MinBalanceUSD:    20,
		TargetBalanceUSD: 50,
	}, bench.BalanceSnapshot{BalanceUSD: 25}, AccountState{}, time.Unix(100, 0))

	if decision.Plan != nil || decision.Skipped == "" {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestPlanDepositClampsToMaxDeposit(t *testing.T) {
	now := time.Unix(100, 0)
	decision := PlanDeposit(Config{DryRun: true}, AccountConfig{
		Name:             "lighter",
		MinBalanceUSD:    20,
		TargetBalanceUSD: 100,
		MaxDepositUSDC:   30,
		Deposit:          DepositConfig{MinUSDC: 5},
	}, bench.BalanceSnapshot{BalanceUSD: 10}, AccountState{}, now)

	if decision.Plan == nil {
		t.Fatalf("missing plan: %+v", decision)
	}
	if decision.Plan.Amount != 30 {
		t.Fatalf("amount = %f, want 30", decision.Plan.Amount)
	}
	if !decision.Plan.DryRun {
		t.Fatalf("dry run = false")
	}
}

func TestPlanDepositHonorsCooldown(t *testing.T) {
	now := time.Unix(100, 0)
	decision := PlanDeposit(Config{}, AccountConfig{
		Name:             "lighter",
		MinBalanceUSD:    20,
		TargetBalanceUSD: 50,
		CooldownMS:       int(time.Hour.Milliseconds()),
	}, bench.BalanceSnapshot{BalanceUSD: 10}, AccountState{LastAttemptAt: now.Add(-time.Minute)}, now)

	if decision.Plan != nil || !strings.Contains(decision.Skipped, "cooldown") {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestUSDCAmountUsesSixDecimals(t *testing.T) {
	got, err := usdcAmount(12.345678)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "12345678" {
		t.Fatalf("amount = %s", got)
	}
}
