package funding

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"perps-latency-benchmark/internal/bench"
)

const DefaultArbitrumChainID int64 = 42161
const DefaultArbitrumNativeUSDC = "0xaf88d065e77c8cC2239327C5EDb3A432268e5831"

type Config struct {
	DryRun     bool            `json:"dry_run"`
	IntervalMS int             `json:"interval_ms"`
	StatePath  string          `json:"state_path"`
	EnvFiles   []string        `json:"env_files"`
	Wallet     WalletConfig    `json:"wallet"`
	Accounts   []AccountConfig `json:"accounts"`
}

type WalletConfig struct {
	ChainID       int64   `json:"chain_id"`
	RPCURLEnv     string  `json:"rpc_url_env"`
	PrivateKeyEnv string  `json:"private_key_env"`
	USDCAddress   string  `json:"usdc_address"`
	DailyCapUSDC  float64 `json:"daily_cap_usdc"`
}

type AccountConfig struct {
	Name             string        `json:"name"`
	VenueConfig      string        `json:"venue_config"`
	EnvFiles         []string      `json:"env_files"`
	MinBalanceUSD    float64       `json:"min_balance_usd"`
	TargetBalanceUSD float64       `json:"target_balance_usd"`
	MaxDepositUSDC   float64       `json:"max_deposit_usdc"`
	CooldownMS       int           `json:"cooldown_ms"`
	DryRun           *bool         `json:"dry_run,omitempty"`
	Deposit          DepositConfig `json:"deposit"`
}

type DepositConfig struct {
	Type           string   `json:"type"`
	ChainID        int64    `json:"chain_id"`
	RPCURLEnv      string   `json:"rpc_url_env"`
	PrivateKeyEnv  string   `json:"private_key_env"`
	FromAddressEnv string   `json:"from_address_env"`
	USDCAddress    string   `json:"usdc_address"`
	TokenAddress   string   `json:"token_address"`
	ToAddress      string   `json:"to_address"`
	APIKeyEnv      string   `json:"api_key_env"`
	BridgeChainIn  string   `json:"bridge_chain_in"`
	BridgeChainOut string   `json:"bridge_chain_out"`
	BridgeAsset    string   `json:"bridge_asset"`
	MaxFeeUSDC     float64  `json:"max_fee_usdc"`
	BrokerID       int64    `json:"broker_id"`
	MinUSDC        float64  `json:"min_usdc"`
	BaseURL        string   `json:"base_url"`
	Command        []string `json:"command"`
}

type BalanceReader interface {
	Balance(ctx context.Context, account AccountConfig) (bench.BalanceSnapshot, error)
}

type Depositor interface {
	Deposit(ctx context.Context, plan DepositPlan) (DepositResult, error)
}

type DepositPlan struct {
	Account AccountConfig
	Wallet  WalletConfig
	Balance bench.BalanceSnapshot
	Amount  float64
	DryRun  bool
	Now     time.Time
}

type DepositResult struct {
	AccountName string         `json:"account_name"`
	Venue       string         `json:"venue,omitempty"`
	AmountUSDC  float64        `json:"amount_usdc"`
	DryRun      bool           `json:"dry_run"`
	Status      string         `json:"status"`
	TxHash      string         `json:"tx_hash,omitempty"`
	Route       string         `json:"route,omitempty"`
	Message     string         `json:"message,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CompletedAt time.Time      `json:"completed_at"`
}

func (c Config) Normalized() Config {
	if c.IntervalMS == 0 {
		c.IntervalMS = int((5 * time.Minute).Milliseconds())
	}
	if c.StatePath == "" {
		c.StatePath = "data/funding-state.json"
	}
	if c.Wallet.ChainID == 0 {
		c.Wallet.ChainID = DefaultArbitrumChainID
	}
	if c.Wallet.RPCURLEnv == "" {
		c.Wallet.RPCURLEnv = "ARBITRUM_RPC_URL"
	}
	if c.Wallet.PrivateKeyEnv == "" {
		c.Wallet.PrivateKeyEnv = "BENCHMARK_ARBITRUM_PRIVATE_KEY"
	}
	if c.Wallet.USDCAddress == "" {
		c.Wallet.USDCAddress = DefaultArbitrumNativeUSDC
	}
	for index := range c.Accounts {
		if c.Accounts[index].CooldownMS == 0 {
			c.Accounts[index].CooldownMS = int((30 * time.Minute).Milliseconds())
		}
	}
	return c
}

func (c Config) Validate() error {
	c = c.Normalized()
	if c.IntervalMS <= 0 {
		return fmt.Errorf("interval_ms must be positive")
	}
	if c.Wallet.ChainID != DefaultArbitrumChainID {
		return fmt.Errorf("wallet.chain_id must be %d for Arbitrum funding", DefaultArbitrumChainID)
	}
	if c.Wallet.RPCURLEnv == "" {
		return fmt.Errorf("wallet.rpc_url_env is required")
	}
	if c.Wallet.PrivateKeyEnv == "" {
		return fmt.Errorf("wallet.private_key_env is required")
	}
	if !common.IsHexAddress(c.Wallet.USDCAddress) {
		return fmt.Errorf("wallet.usdc_address is not a valid address")
	}
	if c.Wallet.DailyCapUSDC <= 0 && c.RequiresLiveConfirmation() {
		return fmt.Errorf("wallet.daily_cap_usdc must be positive when any account can submit live deposits")
	}
	seen := map[string]struct{}{}
	for index, account := range c.Accounts {
		if account.Name == "" {
			return fmt.Errorf("accounts[%d].name is required", index)
		}
		if _, exists := seen[account.Name]; exists {
			return fmt.Errorf("duplicate account %q", account.Name)
		}
		seen[account.Name] = struct{}{}
		if account.VenueConfig == "" {
			return fmt.Errorf("%s venue_config is required", account.Name)
		}
		if account.MinBalanceUSD <= 0 {
			return fmt.Errorf("%s min_balance_usd must be positive", account.Name)
		}
		if account.TargetBalanceUSD <= account.MinBalanceUSD {
			return fmt.Errorf("%s target_balance_usd must exceed min_balance_usd", account.Name)
		}
		if account.MaxDepositUSDC < 0 {
			return fmt.Errorf("%s max_deposit_usdc cannot be negative", account.Name)
		}
		if account.CooldownMS < 0 {
			return fmt.Errorf("%s cooldown_ms cannot be negative", account.Name)
		}
		if account.Deposit.MinUSDC < 0 {
			return fmt.Errorf("%s deposit.min_usdc cannot be negative", account.Name)
		}
		if err := validateDepositConfig(c, account); err != nil {
			return err
		}
	}
	return nil
}

func (c Config) RequiresLiveConfirmation() bool {
	for _, account := range c.Accounts {
		if !AccountDryRun(c.DryRun, account) {
			return true
		}
	}
	return false
}

func validateDepositConfig(cfg Config, account AccountConfig) error {
	deposit := account.Deposit
	chainID := deposit.ChainID
	if chainID == 0 {
		chainID = cfg.Wallet.ChainID
	}
	if chainID != DefaultArbitrumChainID && strings.TrimSpace(deposit.Type) != "command" {
		return fmt.Errorf("%s deposit.chain_id must be %d for direct Arbitrum routes", account.Name, DefaultArbitrumChainID)
	}
	for field, address := range map[string]string{"deposit.usdc_address": deposit.USDCAddress, "deposit.token_address": deposit.TokenAddress} {
		if address != "" && !common.IsHexAddress(address) {
			return fmt.Errorf("%s %s is not a valid address", account.Name, field)
		}
	}
	if common.IsHexAddress(deposit.FromAddressEnv) {
		return fmt.Errorf("%s deposit.from_address_env must name an environment variable, not inline an address", account.Name)
	}
	switch strings.TrimSpace(deposit.Type) {
	case "evm_usdc_transfer":
		if !common.IsHexAddress(deposit.ToAddress) {
			return fmt.Errorf("%s deposit.to_address is required and must be a valid address", account.Name)
		}
	case "lighter_cctp_intent":
		if deposit.FromAddressEnv == "" && deposit.PrivateKeyEnv == "" && cfg.Wallet.PrivateKeyEnv == "" {
			return fmt.Errorf("%s lighter_cctp_intent requires from_address_env or a private key env", account.Name)
		}
	case "extended_rhino_bridge":
		if deposit.APIKeyEnv == "" {
			return fmt.Errorf("%s extended_rhino_bridge requires deposit.api_key_env", account.Name)
		}
	case "aster_treasury_deposit":
		if deposit.TokenAddress == "" {
			return fmt.Errorf("%s aster_treasury_deposit requires deposit.token_address", account.Name)
		}
	case "command":
		if len(deposit.Command) == 0 {
			return fmt.Errorf("%s command deposit requires deposit.command", account.Name)
		}
	case "":
		return fmt.Errorf("%s deposit.type is required", account.Name)
	default:
		return fmt.Errorf("%s unsupported deposit.type %q", account.Name, deposit.Type)
	}
	return nil
}

func AccountDryRun(global bool, account AccountConfig) bool {
	if account.DryRun != nil {
		return *account.DryRun
	}
	return global
}

func AccountCooldown(account AccountConfig) time.Duration {
	if account.CooldownMS <= 0 {
		return 0
	}
	return time.Duration(account.CooldownMS) * time.Millisecond
}
