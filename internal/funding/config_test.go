package funding

import "testing"

func TestConfigValidateRequiresConfirmableLiveCap(t *testing.T) {
	cfg := Config{
		DryRun: false,
		Wallet: WalletConfig{
			ChainID:       DefaultArbitrumChainID,
			RPCURLEnv:     "ARBITRUM_RPC_URL",
			PrivateKeyEnv: "BENCHMARK_ARBITRUM_PRIVATE_KEY",
			USDCAddress:   DefaultArbitrumNativeUSDC,
		},
		Accounts: []AccountConfig{{
			Name:             "hyperliquid",
			VenueConfig:      "examples/hyperliquid-taker-builder.json",
			MinBalanceUSD:    20,
			TargetBalanceUSD: 50,
			Deposit: DepositConfig{
				Type:      "evm_usdc_transfer",
				ToAddress: "0x2df1c51e09aecf9cacb7bc98cb1742757f163df7",
			},
		}},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing daily live cap to fail validation")
	}
}

func TestConfigRequiresLiveConfirmationHonorsAccountOverride(t *testing.T) {
	live := false
	cfg := Config{
		DryRun: true,
		Accounts: []AccountConfig{{
			Name:   "lighter",
			DryRun: &live,
		}},
	}

	if !cfg.RequiresLiveConfirmation() {
		t.Fatal("account dry_run=false should require live confirmation")
	}
}

func TestConfigValidateRejectsNonArbitrumDirectRoute(t *testing.T) {
	cfg := Config{
		DryRun: true,
		Wallet: WalletConfig{
			ChainID:       DefaultArbitrumChainID,
			RPCURLEnv:     "ARBITRUM_RPC_URL",
			PrivateKeyEnv: "BENCHMARK_ARBITRUM_PRIVATE_KEY",
			USDCAddress:   DefaultArbitrumNativeUSDC,
			DailyCapUSDC:  100,
		},
		Accounts: []AccountConfig{{
			Name:             "lighter",
			VenueConfig:      "examples/lighter-market-builder.json",
			MinBalanceUSD:    20,
			TargetBalanceUSD: 50,
			Deposit: DepositConfig{
				Type:    "lighter_cctp_intent",
				ChainID: 8453,
			},
		}},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected non-Arbitrum direct route to fail validation")
	}
}

func TestConfigValidateAcceptsExtendedAndAsterRoutes(t *testing.T) {
	cfg := Config{
		DryRun: true,
		Wallet: WalletConfig{
			ChainID:       DefaultArbitrumChainID,
			RPCURLEnv:     "ARBITRUM_RPC_URL",
			PrivateKeyEnv: "BENCHMARK_ARBITRUM_PRIVATE_KEY",
			USDCAddress:   DefaultArbitrumNativeUSDC,
			DailyCapUSDC:  100,
		},
		Accounts: []AccountConfig{
			{
				Name:             "extended",
				VenueConfig:      "examples/extended-taker-builder.json",
				MinBalanceUSD:    20,
				TargetBalanceUSD: 50,
				Deposit: DepositConfig{
					Type:      "extended_rhino_bridge",
					APIKeyEnv: "EXTENDED_API_KEY",
				},
			},
			{
				Name:             "aster",
				VenueConfig:      "examples/aster-taker-builder.json",
				MinBalanceUSD:    20,
				TargetBalanceUSD: 50,
				Deposit: DepositConfig{
					Type:         "aster_treasury_deposit",
					TokenAddress: DefaultArbitrumNativeUSDC,
				},
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}
