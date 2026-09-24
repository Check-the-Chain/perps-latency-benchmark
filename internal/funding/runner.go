package funding

import (
	"context"
	"fmt"
	"io"
	"time"
)

type Runner struct {
	Config   Config
	Balances BalanceReader
	Deposit  Depositor
	State    State
	Out      io.Writer
}

func (r *Runner) RunOnce(ctx context.Context, now time.Time) ([]DepositResult, error) {
	cfg := r.Config.Normalized()
	if r.Balances == nil {
		return nil, fmt.Errorf("funding runner requires balance reader")
	}
	if r.Deposit == nil {
		return nil, fmt.Errorf("funding runner requires depositor")
	}
	if r.State.Accounts == nil {
		r.State.Accounts = map[string]AccountState{}
	}
	var results []DepositResult
	for _, account := range cfg.Accounts {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}
		balance, err := r.Balances.Balance(ctx, account)
		if err != nil {
			RecordAccountError(&r.State, account, balance, now, err)
			if r.Out != nil {
				fmt.Fprintf(r.Out, "%s funding balance failed: %v\n", account.Name, err)
			}
			continue
		}
		decision := PlanDeposit(cfg, account, balance, r.State.Accounts[account.Name], now)
		if decision.Plan == nil {
			if r.Out != nil {
				fmt.Fprintf(r.Out, "%s funding skipped: %s balance=%.6f min=%.6f\n", account.Name, decision.Skipped, balance.BalanceUSD, account.MinBalanceUSD)
			}
			continue
		}
		if cfg.Wallet.DailyCapUSDC > 0 && !decision.Plan.DryRun && DailySpent(r.State, now)+decision.Plan.Amount > cfg.Wallet.DailyCapUSDC {
			if r.Out != nil {
				fmt.Fprintf(r.Out, "%s funding skipped: daily cap would be exceeded amount=%.6f spent=%.6f cap=%.6f\n", account.Name, decision.Plan.Amount, DailySpent(r.State, now), cfg.Wallet.DailyCapUSDC)
			}
			continue
		}
		result, err := r.Deposit.Deposit(ctx, *decision.Plan)
		RecordResult(&r.State, *decision.Plan, result, err)
		if err != nil {
			if r.Out != nil {
				fmt.Fprintf(r.Out, "%s funding deposit failed: %v\n", account.Name, err)
			}
			continue
		}
		results = append(results, result)
		if r.Out != nil {
			fmt.Fprintf(r.Out, "%s funding %s amount=%.6f route=%s tx=%s\n", account.Name, result.Status, result.AmountUSDC, result.Route, result.TxHash)
		}
	}
	return results, nil
}
