package accounts

var venueSpecs = []VenueSpec{
	{
		Name:        "hyperliquid",
		WalletKinds: []WalletKind{WalletEVM},
		Supported:   true,
		Env: []EnvVar{
			{Name: "HYPERLIQUID_SECRET_KEY", Secret: true, Generated: true, Required: true, Note: "EVM private key for the Hyperliquid signing wallet or approved agent wallet."},
		},
		ManualSteps: []string{
			"Fund the account or agent wallet with enough collateral for the target market.",
			"Use post-only orders by default while validating repeatability.",
		},
	},
	{
		Name:        "aster",
		WalletKinds: []WalletKind{WalletEVM},
		Supported:   false,
		ManualSteps: []string{
			"Aster V3 order signing is documented, but no SDK-backed builder is included yet.",
			"Do not use generated keys for live Aster benchmarks until the venue builder exists.",
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
		Name:        "lighter",
		WalletKinds: []WalletKind{WalletEVM},
		Supported:   true,
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
			"Default builder params target BTC; adjust market_index and scaled price/size values if you change markets.",
		},
	},
	{
		Name:      "variational_omni",
		Supported: false,
		ManualSteps: []string{
			"Current public Omni docs do not verify a trading/order API, so there is no generated wallet setup.",
		},
	},
}
