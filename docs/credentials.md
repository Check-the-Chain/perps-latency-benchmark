# Credentials

Keep JSON benchmark configs non-secret and commit-safe. Real credentials should
live in local dotenv files or the shell environment.

Recommended layout:

- `.env.local`: optional shared local settings.
- `.env.hyperliquid.local`: Hyperliquid credentials.
- `.env.lighter.local`: Lighter credentials.
- `.env.aster.local`: Aster credentials.
- `.env.grvt.local`: GRVT credentials.
- `.env.edgex.local`: edgeX credentials.
- `.env.extended.local`: Extended credentials.
- `.env.variational.local`: Variational credentials.
- `.env.pacifica.local`: Pacifica credentials.
- `.env.nado.local`: Nado credentials.
- `.env.nado-direct.local`: Nado direct-backend runner credentials.

The matching `.env.*.example` files are committed as templates. Local dotenv
files are ignored by git.

Load credentials explicitly:

```bash
go run ./cmd/perps-bench run \
  --config examples/hyperliquid-builder.json \
  --env-file .env.local \
  --env-file .env.hyperliquid.local \
  --confirm-live
```

`--env-file` is repeatable. Files are loaded in the order given. Later dotenv
files override earlier dotenv files, but existing shell environment variables
win over all dotenv values.

Account CLI
-----------

Use the account setup commands before live runs:

```bash
go run ./cmd/perps-bench accounts plan --venues hyperliquid,lighter
go run ./cmd/perps-bench accounts generate --venues hyperliquid,lighter --out .env.wallets.local
go run ./cmd/perps-bench accounts checklist --venues hyperliquid,lighter --env-file .env.wallets.local
go run ./cmd/perps-bench accounts print --venues hyperliquid,lighter --env-file .env.wallets.local
go run ./cmd/perps-bench accounts check --venues hyperliquid,lighter --env-file .env.wallets.local
```

Wallet generation deduplicates by wallet kind. For example, Hyperliquid and
GRVT both use the generated EVM key, Lighter uses that same EVM key for
account registration/deposits, while edgeX and Extended share the generated
Stark key. Lighter trading API key material is filled from Lighter after API key
creation.

`accounts generate` writes private material only to the local dotenv file and
prints public identifiers only. It preserves existing variables in the output
file so repeated runs do not rotate wallets accidentally.

`accounts checklist` prints the public identifiers plus the remaining manual
venue setup steps, such as funding the Hyperliquid wallet and filling Lighter's
account/API key indexes after API key registration.

Inline Secrets
--------------

By default, the CLI rejects JSON configs that contain inline secret-looking keys,
including:

- `secret`
- `private_key`
- `api_key`
- `token`
- `cookie`
- `signature`
- `mnemonic`
- `seed`
- `passphrase`

Use `--allow-inline-secrets` only for short local debugging. Do not use it for
committed configs or shared benchmark runs.

Venue Variables
---------------

Hyperliquid:

```bash
HYPERLIQUID_SECRET_KEY=
```

Lighter:

```bash
LIGHTER_L1_PRIVATE_KEY=
LIGHTER_L1_ADDRESS=
LIGHTER_PRIVATE_KEY=
LIGHTER_ACCOUNT_INDEX=
LIGHTER_API_KEY_INDEX=
LIGHTER_MAKER_PRIVATE_KEY=
LIGHTER_MAKER_API_KEY_INDEX=
LIGHTER_TAKER_PRIVATE_KEY=
LIGHTER_TAKER_API_KEY_INDEX=
```

Lighter has two key roles. Its official docs state that account creation and
deposits are tied to an Ethereum wallet; order submission uses Lighter API keys
owned by the account. See the official [Lighter API docs](https://docs.lighter.xyz/perpetual-futures/api)
and [API key docs](https://apidocs.lighter.xyz/docs/api-keys). `LIGHTER_L1_PRIVATE_KEY`
/ `LIGHTER_L1_ADDRESS` are setup credentials, while `LIGHTER_PRIVATE_KEY`,
`LIGHTER_ACCOUNT_INDEX`, and `LIGHTER_API_KEY_INDEX` are required by benchmark
order builders.

Use Lighter to generate the trading API key, then copy the active key material
and indexes into `LIGHTER_PRIVATE_KEY`, `LIGHTER_ACCOUNT_INDEX`, and
`LIGHTER_API_KEY_INDEX`.

For Lighter maker latency runs, use a separate API key and mark that key
maker-only in Lighter. Put it in `LIGHTER_MAKER_PRIVATE_KEY` and
`LIGHTER_MAKER_API_KEY_INDEX`. The Lighter builder automatically uses the maker
key for post-only orders and the taker key for market/IOC orders. Taker runs use
`LIGHTER_TAKER_PRIVATE_KEY` / `LIGHTER_TAKER_API_KEY_INDEX` when present, then
fall back to `LIGHTER_PRIVATE_KEY` / `LIGHTER_API_KEY_INDEX`.

Run only one live Lighter benchmark process per Lighter account/API key pair.
Lighter nonces are tied to the API key, so
parallel maker/market or HTTPS/WebSocket runners need separate Lighter API keys.
The CLI enforces this with a local process lock and exits before submitting if
another runner is already using the same pair.

The starter builders default to small post-only BTC orders. Lighter uses scaled
integer order units: the current BTC default is `market_index=1`,
`base_amount=100` and `price=750000`, representing 0.001 BTC at 75,000.0.

GRVT:

```bash
GRVT_PRIVATE_KEY=
GRVT_TRADING_ACCOUNT_ID=
GRVT_ENV=prod
GRVT_SESSION_COOKIE=
```

Aster:

```bash
ASTER_USER_ADDRESS=
ASTER_API_WALLET_ADDRESS=
ASTER_API_PRIVATE_KEY=
```

Aster futures V3 uses API Wallet signing. `ASTER_USER_ADDRESS` is the
main/login wallet address, while `ASTER_API_WALLET_ADDRESS` and
`ASTER_API_PRIVATE_KEY` are the API Wallet signer. The same API Wallet material
is used for order submission, signed user data stream listenKey creation,
cleanup, and read-only account checks. Funding is not needed for local
payload-building tests, but a funded account is needed for proper live submit,
private-stream confirmation, cleanup, and neutralization tests.

edgeX:

```bash
EDGEX_ACCOUNT_ID=
EDGEX_STARK_PRIVATE_KEY=
```

edgeX order submission, private WebSocket confirmation, cleanup, and
neutralization use the same Stark key and account ID. Funding is not needed for
local payload-building tests, but a funded edgeX account is needed for proper
live submit, private-stream confirmation, cleanup, and neutralization tests.

Extended:

```bash
EXTENDED_VAULT=
EXTENDED_PRIVATE_KEY=
EXTENDED_PUBLIC_KEY=
EXTENDED_API_KEY=
EXTENDED_ENV=mainnet
```

Extended order submission, private WebSocket confirmation, cleanup, and
reconciliation all use the same Stark key/API key material. Funding is not
needed for local payload-building tests, but a funded account is needed for
proper live submit, confirmation, cleanup, and neutralization tests.

Pacifica:

```bash
PACIFICA_PRIVATE_KEY=
PACIFICA_ACCOUNT=
PACIFICA_AGENT_WALLET=
```

Pacifica order submission is websocket-first. The default builder uses
`wss://ws.pacifica.fi/ws`, Ed25519 signing, and `tif=ALO` to avoid Pacifica's
documented latency protection delay for GTC/IOC orders. Prefer a Pacifica API
Agent Key for benchmarks: set `PACIFICA_PRIVATE_KEY` to the agent private key,
`PACIFICA_ACCOUNT` to the original/main account, and optionally
`PACIFICA_AGENT_WALLET` to the agent wallet public key. If `PACIFICA_ACCOUNT`
differs from the signing key's public key and `PACIFICA_AGENT_WALLET` is empty,
the builder sends the signing public key as `agent_wallet`. Funding is not
needed for local payload-building tests, but a funded Pacifica account and a
bound agent wallet are needed for live submit, private WebSocket confirmation,
and cleanup tests.

Variational:

```bash
VARIATIONAL_API_KEY=
VARIATIONAL_API_SECRET=
VARIATIONAL_BASE_URL=
VARIATIONAL_TARGET_COMPANIES=
```

Variational request signing uses HMAC-SHA256 with `VARIATIONAL_API_KEY` and the
hex-encoded `VARIATIONAL_API_SECRET`. `VARIATIONAL_BASE_URL` is optional unless
Variational provides a private or environment-specific endpoint. The default
builder action is an authenticated `/status` smoke check; RFQ submit tests also
need target company IDs in `VARIATIONAL_TARGET_COMPANIES` or
`params.target_companies`.

Non-Secret Metadata
-------------------

Some builders need current venue metadata in the JSON config. Keep this separate
from credentials:

- edgeX: `params.metadata.contractList` and `params.metadata.coinList`.
- GRVT: `params.instruments`.
- Extended: `params.l2_config` or `params.market_model`.
- Variational: `params.target_companies` for RFQ submission.

These values are not secrets, but they should be refreshed before serious live
benchmarking so payload construction does not fetch metadata inside the timed
path.

Nado:

```bash
NADO_PRIVATE_KEY=
NADO_ADDRESS=
NADO_CHAIN_ID=57073
NADO_ENDPOINT_CONTRACT=
```

Nado order submission uses Gateway websocket executes at
`wss://gateway.prod.nado.xyz/v1/ws`. The default builder signs EIP-712
`place_order` payloads locally and uses `POST_ONLY` orders with a 50 ms nonce
discard window. `NADO_ADDRESS` is optional because it is derived from
`NADO_PRIVATE_KEY`; set it only as a guardrail. `NADO_ENDPOINT_CONTRACT` is
only needed when enabling authenticated subscription confirmation or cleanup. A
funded Nado subaccount is needed for live submit tests.

The direct-backend runner is configured as `nado_direct` in
`examples/nado-direct-builder.json`. It uses
`https://prod-mm.nado-backend.xyz/execute` for REST fallback/cleanup and
`wss://prod-mm.nado-backend.xyz/ws/v2` for Gateway WebSocket submission.
