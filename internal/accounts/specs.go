package accounts

var venueSpecs = []VenueSpec{
	{
		Name:             "hyperliquid",
		WalletKinds:      []WalletKind{WalletEVM},
		Supported:        true,
		SetupWalletKind:  WalletEVM,
		SetupWalletLabel: "trading/agent wallet",
		Env: []EnvVar{
			{Name: "HYPERLIQUID_SECRET_KEY", Secret: true, Generated: true, Required: true, Note: "EVM private key for the Hyperliquid signing wallet or approved agent wallet."},
		},
		ManualSteps: []string{
			"Register or approve the EVM wallet as the Hyperliquid trading or agent wallet if needed.",
			"Fund the account or agent wallet with enough collateral for the target market.",
			"Use post-only orders by default while validating repeatability.",
			"Adjust examples/hyperliquid-builder.json if you want to change the default BTC order params.",
			"Run with: go run ./cmd/perps-bench run --config examples/hyperliquid-builder.json --env-file .env.wallets.local --confirm-live",
		},
	},
	{
		Name:      "aster",
		Supported: true,
		Env: []EnvVar{
			{Name: "ASTER_USER_ADDRESS", Required: true, Note: "Aster main/login wallet address used as the V3 user field."},
			{Name: "ASTER_API_WALLET_ADDRESS", Required: true, Note: "Aster API Wallet address used as the V3 signer field."},
			{Name: "ASTER_API_PRIVATE_KEY", Secret: true, Required: true, Note: "Private key for ASTER_API_WALLET_ADDRESS used for EIP-712 V3 request signing."},
		},
		ManualSteps: []string{
			"Create an Aster Pro API Wallet/Agent with trade and user-stream permissions.",
			"Use post-only GTX orders by default while validating repeatability.",
		},
	},
	{
		Name:        "grvt",
		WalletKinds: []WalletKind{WalletEVM},
		Supported:   true,
		Env: []EnvVar{
			{Name: "GRVT_PRIVATE_KEY", Secret: true, Generated: true, Required: true, Note: "EVM private key used by the GRVT signing SDK."},
			{Name: "GRVT_TRADING_ACCOUNT_ID", Required: true, Note: "Venue trading account/sub-account identifier."},
			{Name: "GRVT_SESSION_COOKIE", Secret: true, Note: "Optional session/auth material if required by the live endpoint."},
		},
		ManualSteps: []string{
			"Register/enable the generated EVM signer in GRVT if the account flow requires it.",
			"Refresh params.instruments before serious live runs.",
		},
	},
	{
		Name:        "edgex",
		WalletKinds: []WalletKind{WalletStark},
		Supported:   true,
		Env: []EnvVar{
			{Name: "EDGEX_STARK_PRIVATE_KEY", Secret: true, Generated: true, Required: true, Note: "Stark private key used for edgeX L2 order signing."},
			{Name: "EDGEX_ACCOUNT_ID", Required: true, Note: "edgeX account identifier."},
		},
		ManualSteps: []string{
			"Register/attach the Stark key to the edgeX account.",
			"Refresh params.metadata.contractList and params.metadata.coinList before live runs.",
		},
	},
	{
		Name:        "extended",
		WalletKinds: []WalletKind{WalletStark},
		Supported:   true,
		Env: []EnvVar{
			{Name: "EXTENDED_PRIVATE_KEY", Secret: true, Generated: true, Required: true, Note: "Stark private key used by the official Extended SDK."},
			{Name: "EXTENDED_PUBLIC_KEY", Generated: true, Required: true, Note: "Derived Stark public key."},
			{Name: "EXTENDED_VAULT", Required: true, Note: "Extended vault/account identifier."},
			{Name: "EXTENDED_API_KEY", Secret: true, Required: true, Note: "Extended REST API key."},
		},
		ManualSteps: []string{
			"Register/attach the Stark key to the Extended account.",
			"Confirm params.l2_config or params.market_model is current.",
		},
	},
	{
		Name:             "lighter",
		WalletKinds:      []WalletKind{WalletEVM},
		Supported:        true,
		SetupWalletKind:  WalletEVM,
		SetupWalletLabel: "ethereum setup/funding wallet",
		SetupWalletEnv:   "LIGHTER_L1_ADDRESS",
		Env: []EnvVar{
			{Name: "LIGHTER_L1_PRIVATE_KEY", Wallet: WalletEVM, Secret: true, Generated: true, Note: "Ethereum wallet private key for Lighter account registration/deposits. Not required for hot order submission after setup."},
			{Name: "LIGHTER_L1_ADDRESS", Wallet: WalletEVM, Generated: true, Note: "Derived Ethereum address for Lighter account registration/deposits."},
			{Name: "LIGHTER_PRIVATE_KEY", Secret: true, Required: true, Note: "Active Lighter API private key generated or registered in Lighter."},
			{Name: "LIGHTER_ACCOUNT_INDEX", Required: true, Note: "Lighter account index."},
			{Name: "LIGHTER_API_KEY_INDEX", Required: true, Note: "Lighter API key index."},
			{Name: "LIGHTER_MAKER_PRIVATE_KEY", Secret: true, Note: "Optional maker-only Lighter API private key used for post-only orders."},
			{Name: "LIGHTER_MAKER_API_KEY_INDEX", Note: "Optional maker-only Lighter API key index used for post-only orders."},
			{Name: "LIGHTER_TAKER_PRIVATE_KEY", Secret: true, Note: "Optional taker Lighter API private key used for market/IOC orders."},
			{Name: "LIGHTER_TAKER_API_KEY_INDEX", Note: "Optional taker Lighter API key index used for market/IOC orders."},
		},
		ManualSteps: []string{
			"Lighter account creation and deposits require an Ethereum wallet; use LIGHTER_L1_ADDRESS for registration/funding.",
			"Generate an API key in Lighter, then set LIGHTER_PRIVATE_KEY, LIGHTER_ACCOUNT_INDEX, and LIGHTER_API_KEY_INDEX.",
			"For lowest maker latency on premium accounts, set a separate maker-only API key in LIGHTER_MAKER_PRIVATE_KEY and LIGHTER_MAKER_API_KEY_INDEX.",
			"Fund the Lighter account with enough collateral for the configured market.",
			"Default builder params target BTC; adjust market_index and scaled price/size values if you change markets.",
			"Run with: go run ./cmd/perps-bench run --config examples/lighter-builder.json --env-file .env.wallets.local --confirm-live",
		},
	},
	{
		Name:      "lighter_free",
		Supported: true,
		Env: []EnvVar{
			{Name: "LIGHTER_FREE_PRIVATE_KEY", Secret: true, Required: true, Note: "Active Lighter free-tier API private key."},
			{Name: "LIGHTER_FREE_ACCOUNT_INDEX", Required: true, Note: "Lighter free-tier account index."},
			{Name: "LIGHTER_FREE_API_KEY_INDEX", Required: true, Note: "Lighter free-tier API key index."},
			{Name: "LIGHTER_FREE_TAKER_PRIVATE_KEY", Secret: true, Note: "Optional taker key override; defaults to LIGHTER_FREE_PRIVATE_KEY."},
			{Name: "LIGHTER_FREE_TAKER_API_KEY_INDEX", Note: "Optional taker key index override; defaults to LIGHTER_FREE_API_KEY_INDEX."},
		},
		ManualSteps: []string{
			"Use a separate L1 wallet/account so the Lighter fee tier is independent from the main Lighter runner.",
			"Run with: go run ./cmd/perps-bench run --config examples/lighter-free-market-builder.json --env-file .env.lighter-free.local --confirm-live",
		},
	},
	{
		Name:      "variational_omni",
		Supported: true,
		Env: []EnvVar{
			{Name: "VARIATIONAL_API_KEY", Secret: true, Required: true, Note: "Variational API key with the required company/account permissions."},
			{Name: "VARIATIONAL_API_SECRET", Secret: true, Required: true, Note: "Hex-encoded Variational API secret used for HMAC-SHA256 request signing."},
			{Name: "VARIATIONAL_BASE_URL", Note: "Optional API base URL override if Variational provides a private or environment-specific endpoint."},
			{Name: "VARIATIONAL_TARGET_COMPANIES", Note: "Optional comma-separated company IDs for RFQ submission."},
		},
		ManualSteps: []string{
			"Confirm with Variational whether your credentials are for Omni or Pro and which base URL should be used.",
			"For RFQ submit tests, provide target company IDs in config params.target_companies or VARIATIONAL_TARGET_COMPANIES.",
			"Start with the status smoke config before submitting RFQs.",
		},
	},
	{
		Name:      "pacifica",
		Supported: true,
		Env: []EnvVar{
			{Name: "PACIFICA_PRIVATE_KEY", Secret: true, Required: true, Note: "Base58 Solana private key for the Pacifica account or registered API agent wallet."},
			{Name: "PACIFICA_ACCOUNT", Required: true, Note: "Pacifica account public key. If unset in the builder, it defaults to the private key public key."},
			{Name: "PACIFICA_AGENT_WALLET", Note: "Optional registered API agent wallet public key when PACIFICA_PRIVATE_KEY is an agent key."},
		},
		ManualSteps: []string{
			"Register the API agent wallet in Pacifica if using an agent key.",
			"Use websocket transport and tif=ALO/TOB for lowest maker latency; GTC/IOC and market paths have documented latency protection delays.",
			"Fund the Pacifica account with enough collateral for the configured market.",
			"Run with: go run ./cmd/perps-bench run --config examples/pacifica-builder.json --env-file .env.pacifica.local --confirm-live",
		},
	},
	{
		Name:        "nado",
		WalletKinds: []WalletKind{WalletEVM},
		Supported:   true,
		Env: []EnvVar{
			{Name: "NADO_PRIVATE_KEY", Wallet: WalletEVM, Secret: true, Generated: true, Required: true, Note: "EVM private key used for Nado EIP-712 order signing."},
			{Name: "NADO_ADDRESS", Wallet: WalletEVM, Generated: true, Note: "Derived EVM address; optional because the builder derives it from NADO_PRIVATE_KEY."},
			{Name: "NADO_CHAIN_ID", Note: "Optional chain ID override; defaults to Ink mainnet chain ID 57073."},
			{Name: "NADO_ENDPOINT_CONTRACT", Note: "Endpoint contract used for authenticated subscription confirmation and cleanup signing."},
		},
		ManualSteps: []string{
			"Create/fund the Nado subaccount for the configured product and subaccount name.",
			"Use websocket Gateway executes for lowest submit latency; REST is available but not the default builder path.",
			"Keep the default POST_ONLY order type for maker latency and risk-controlled runs.",
			"Run with: go run ./cmd/perps-bench run --config examples/nado-builder.json --env-file .env.nado.local --confirm-live",
		},
	},
}
