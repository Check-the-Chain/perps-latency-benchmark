package funding

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"perps-latency-benchmark/internal/bench"
)

type State struct {
	Accounts map[string]AccountState `json:"accounts"`
	Events   []DepositEvent          `json:"events,omitempty"`
}

type AccountState struct {
	LastAttemptAt time.Time      `json:"last_attempt_at,omitempty"`
	LastSuccessAt time.Time      `json:"last_success_at,omitempty"`
	LastAmount    float64        `json:"last_amount_usdc,omitempty"`
	LastStatus    string         `json:"last_status,omitempty"`
	LastTxHash    string         `json:"last_tx_hash,omitempty"`
	LastError     string         `json:"last_error,omitempty"`
	LastBalance   float64        `json:"last_balance_usd,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type DepositEvent struct {
	AccountName string    `json:"account_name"`
	AmountUSDC  float64   `json:"amount_usdc"`
	DryRun      bool      `json:"dry_run"`
	Status      string    `json:"status"`
	TxHash      string    `json:"tx_hash,omitempty"`
	CompletedAt time.Time `json:"completed_at"`
}

func LoadState(path string) (State, error) {
	if path == "" {
		return State{Accounts: map[string]AccountState{}}, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return State{Accounts: map[string]AccountState{}}, nil
	}
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	if state.Accounts == nil {
		state.Accounts = map[string]AccountState{}
	}
	state.Events = recentEvents(state.Events, time.Now().UTC())
	return state, nil
}

func SaveState(path string, state State) error {
	if path == "" {
		return nil
	}
	if state.Accounts == nil {
		state.Accounts = map[string]AccountState{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func RecordResult(state *State, plan DepositPlan, result DepositResult, err error) {
	if state.Accounts == nil {
		state.Accounts = map[string]AccountState{}
	}
	account := AccountState{
		LastAmount:  plan.Amount,
		LastBalance: plan.Balance.BalanceUSD,
		Metadata:    result.Metadata,
	}
	if !plan.DryRun {
		account.LastAttemptAt = plan.Now.UTC()
	}
	if err != nil {
		account.LastStatus = "error"
		account.LastError = err.Error()
	} else {
		account.LastStatus = result.Status
		account.LastTxHash = result.TxHash
		if result.Status == "success" {
			account.LastSuccessAt = result.CompletedAt.UTC()
		}
		state.Events = append(recentEvents(state.Events, plan.Now.UTC()), DepositEvent{
			AccountName: plan.Account.Name,
			AmountUSDC:  plan.Amount,
			DryRun:      result.DryRun,
			Status:      result.Status,
			TxHash:      result.TxHash,
			CompletedAt: result.CompletedAt.UTC(),
		})
	}
	state.Accounts[plan.Account.Name] = account
}

func RecordAccountObservation(state *State, account AccountConfig, balance bench.BalanceSnapshot, status string, detail string, now time.Time) {
	if state.Accounts == nil {
		state.Accounts = map[string]AccountState{}
	}
	record := state.Accounts[account.Name]
	record.LastStatus = status
	record.LastBalance = balance.BalanceUSD
	record.LastError = detail
	record.Metadata = balanceObservationMetadata(balance, now)
	state.Accounts[account.Name] = record
}

func RecordAccountError(state *State, account AccountConfig, balance bench.BalanceSnapshot, now time.Time, err error) {
	if state.Accounts == nil {
		state.Accounts = map[string]AccountState{}
	}
	record := state.Accounts[account.Name]
	record.LastStatus = "error"
	if balance.BalanceUSD != 0 {
		record.LastBalance = balance.BalanceUSD
	}
	if err != nil {
		record.LastError = err.Error()
	}
	if !now.IsZero() {
		record.Metadata = map[string]any{"observed_at": now.UTC().Format(time.RFC3339)}
	}
	state.Accounts[account.Name] = record
}

func balanceObservationMetadata(balance bench.BalanceSnapshot, now time.Time) map[string]any {
	metadata := map[string]any{}
	for key, value := range balance.Metadata {
		metadata[key] = value
	}
	if value := FundingBalanceUSD(balance); value != balance.BalanceUSD {
		metadata["funding_balance_usd"] = value
	}
	if !balance.CapturedAt.IsZero() {
		metadata["balance_captured_at"] = balance.CapturedAt.UTC().Format(time.RFC3339)
	}
	if !now.IsZero() {
		metadata["observed_at"] = now.UTC().Format(time.RFC3339)
	}
	if len(balance.Positions) > 0 {
		metadata["positions"] = balance.Positions
	}
	return metadata
}

func DailySpent(state State, now time.Time) float64 {
	var total float64
	cutoff := now.UTC().Add(-24 * time.Hour)
	for _, event := range state.Events {
		if event.DryRun || event.Status != "success" || event.CompletedAt.Before(cutoff) {
			continue
		}
		total += event.AmountUSDC
	}
	return total
}

func recentEvents(events []DepositEvent, now time.Time) []DepositEvent {
	cutoff := now.UTC().Add(-24 * time.Hour)
	filtered := events[:0]
	for _, event := range events {
		if event.CompletedAt.IsZero() || event.CompletedAt.Before(cutoff) {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered
}
