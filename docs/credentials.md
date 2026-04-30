# Credentials

Keep JSON benchmark configs non-secret and commit-safe. Real credentials should
live in local dotenv files or the shell environment.

Recommended layout:

- `.env.local`: optional shared local settings.
- `.env.hyperliquid.local`: Hyperliquid credentials.
- `.env.lighter.local`: Lighter credentials.
- `.env.grvt.local`: GRVT credentials.
- `.env.edgex.local`: edgeX credentials.
- `.env.extended.local`: Extended credentials.

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

edgeX:

```bash
EDGEX_ACCOUNT_ID=
EDGEX_STARK_PRIVATE_KEY=
```

Extended:

```bash
EXTENDED_VAULT=
EXTENDED_PRIVATE_KEY=
EXTENDED_PUBLIC_KEY=
EXTENDED_API_KEY=
EXTENDED_ENV=mainnet
```

Non-Secret Metadata
-------------------

Some builders need current venue metadata in the JSON config. Keep this separate
from credentials:

- edgeX: `params.metadata.contractList` and `params.metadata.coinList`.
- GRVT: `params.instruments`.
- Extended: `params.l2_config` or `params.market_model`.

These values are not secrets, but they should be refreshed before serious live
benchmarking so payload construction does not fetch metadata inside the timed
path.
