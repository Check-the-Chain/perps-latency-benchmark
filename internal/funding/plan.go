package funding

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"perps-latency-benchmark/internal/bench"
)

type Decision struct {
	Account AccountConfig         `json:"account"`
	Balance bench.BalanceSnapshot `json:"balance"`
	Plan    *DepositPlan          `json:"plan,omitempty"`
	Skipped string                `json:"skipped,omitempty"`
}

func PlanDeposit(cfg Config, account AccountConfig, balance bench.BalanceSnapshot, state AccountState, now time.Time) Decision {
	if account.Name == "" {
		return Decision{Account: account, Balance: balance, Skipped: "account name is required"}
	}
	if account.MinBalanceUSD <= 0 {
		return Decision{Account: account, Balance: balance, Skipped: "min_balance_usd must be positive"}
	}
	if account.TargetBalanceUSD <= account.MinBalanceUSD {
		return Decision{Account: account, Balance: balance, Skipped: "target_balance_usd must exceed min_balance_usd"}
	}
	fundingBalance := FundingBalanceUSD(balance)
	if fundingBalance >= account.MinBalanceUSD {
		return Decision{Account: account, Balance: balance, Skipped: "balance above threshold"}
	}
	if !state.LastAttemptAt.IsZero() && now.Sub(state.LastAttemptAt) < AccountCooldown(account) {
		return Decision{Account: account, Balance: balance, Skipped: fmt.Sprintf("cooldown active until %s", state.LastAttemptAt.Add(AccountCooldown(account)).UTC().Format(time.RFC3339))}
	}
	amount := account.TargetBalanceUSD - fundingBalance
	if account.MaxDepositUSDC > 0 && amount > account.MaxDepositUSDC {
		amount = account.MaxDepositUSDC
	}
	minDeposit := account.Deposit.MinUSDC
	if minDeposit <= 0 {
		minDeposit = 1
	}
	if amount < minDeposit {
		return Decision{Account: account, Balance: balance, Skipped: fmt.Sprintf("needed %.6f USDC is below min deposit %.6f", amount, minDeposit)}
	}
	return Decision{
		Account: account,
		Balance: balance,
		Plan: &DepositPlan{
			Account: account,
			Wallet:  cfg.Wallet,
			Balance: balance,
			Amount:  amount,
			DryRun:  AccountDryRun(cfg.DryRun, account),
			Now:     now,
		},
	}
}

func FundingBalanceUSD(balance bench.BalanceSnapshot) float64 {
	if balance.Metadata != nil {
		if value, ok := numericMetadata(balance.Metadata["funding_balance_usd"]); ok {
			return value
		}
	}
	return balance.BalanceUSD
}

func numericMetadata(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}
