# Perps Latency Benchmark

Benchmark crypto perps order submission latency.

## Prerequisites

- Go 1.25+
- `uv` for live Hyperliquid/Lighter/Extended runs; Python 3 for Aster
- Venue accounts funded and configured before using `--confirm-live`

## Quick Start

Generate local wallet material:

```bash
go run ./cmd/perps-bench accounts generate \
  --venues hyperliquid,lighter \
  --out .env.wallets.local
```

Print the setup checklist. This shows the public wallet identifiers you need for
venue setup.

```bash
go run ./cmd/perps-bench accounts checklist \
  --venues hyperliquid,lighter \
  --env-file .env.wallets.local
```

Complete the venue-side setup:

- Hyperliquid: register or approve the printed EVM wallet if needed, fund the
  Hyperliquid account or agent wallet, then adjust the BTC order params in
  `examples/hyperliquid-builder.json` if needed.
- Lighter: use the printed Ethereum address for account creation/deposits,
  generate an API key in Lighter, fill `LIGHTER_PRIVATE_KEY`,
  `LIGHTER_ACCOUNT_INDEX`, and `LIGHTER_API_KEY_INDEX` in `.env.wallets.local`,
  optionally set `LIGHTER_MAKER_PRIVATE_KEY` and
  `LIGHTER_MAKER_API_KEY_INDEX` for a maker-only key,
  then adjust the BTC order params in `examples/lighter-builder.json` if
  needed.

Verify that required local environment is present:

```bash
go run ./cmd/perps-bench accounts check \
  --venues hyperliquid,lighter \
  --env-file .env.wallets.local
```

Run a benchmark:

```bash
go run ./cmd/perps-bench run \
  --config examples/hyperliquid-builder.json \
  --env-file .env.wallets.local \
  --confirm-live
```

Swap the config file for another configured venue, for example:

```bash
examples/lighter-builder.json
examples/aster-builder.json
examples/extended-builder.json
```

The starter configs use small post-only orders. Keep them small until account
setup is confirmed. Cleanup runs after each measured submit, outside the
latency window.

For long-running taker sampling, use the venue-specific `*-taker-builder.json`
config and strict cleanup.

## Account Commands

```bash
go run ./cmd/perps-bench accounts plan --venues hyperliquid,lighter
go run ./cmd/perps-bench accounts generate --venues hyperliquid,lighter --out .env.wallets.local
go run ./cmd/perps-bench accounts checklist --venues hyperliquid,lighter --env-file .env.wallets.local
go run ./cmd/perps-bench accounts check --venues hyperliquid,lighter --env-file .env.wallets.local
```

See `docs/credentials.md` for env-file details.

## Output Files

Write JSON/CSV results:

```bash
go run ./cmd/perps-bench run \
  --config examples/hyperliquid-builder.json \
  --env-file .env.wallets.local \
  --confirm-live \
  --output results/hyperliquid.json \
  --csv results/hyperliquid.csv
```

Summaries default to full response latency. To use TTFB:

```bash
--latency-mode ttfb
```

To measure private-stream confirmation instead of submit response:

```bash
--measurement-mode ws_confirmation
```

Compare saved result files:

```bash
go run ./cmd/perps-bench compare-results \
  results/hyperliquid.json \
  results/lighter.json
```

## Transports

Some venues support order submission over both HTTPS and WebSocket. Compare them
with:

```bash
go run ./cmd/perps-bench compare-transports \
  --config examples/hyperliquid-builder.json \
  --env-file .env.wallets.local \
  --transports https,websocket \
  --iterations 50 \
  --warmups 5 \
  --confirm-live \
  --output results/hyperliquid-transports.json
```

Unsupported transport/scenario combinations fail before the run starts.

## Continuous API

Run a benchmark continuously into a local SQLite store:

```bash
go run ./cmd/perps-bench run-continuous \
  --config examples/lighter-builder.json \
  --env-file .env.wallets.local \
  --transport websocket \
  --rate 0.0166667 \
  --chunk-iterations 1 \
  --cleanup-mode strict \
  --confirm-live \
  --store data/bench.db
```

Serve the read-only API:

```bash
go run ./cmd/perps-bench serve \
  --store data/bench.db \
  --listen 127.0.0.1:8080
```

Expose it publicly with a password:

```bash
export PERPS_BENCH_API_PASSWORD='choose-a-long-password'

go run ./cmd/perps-bench serve \
  --store data/bench.db \
  --listen 0.0.0.0:8080 \
  --cors-origin ""
```

Check the API with:

```bash
curl -u "bench:$PERPS_BENCH_API_PASSWORD" \
  "http://YOUR_SERVER:8080/api/latest?window=5m"
```

## Dashboard

Start the read-only API:

```bash
go run ./cmd/perps-bench serve \
  --store data/bench.db \
  --listen 127.0.0.1:8080
```

Start the dashboard:

```bash
cd frontend
npm install
npm run dev
```

Open the URL printed by Vite, normally `http://127.0.0.1:3000`. The dashboard
keeps API credentials on the server side; they are not exposed to the browser.

For local development against an authenticated API, create `frontend/.dev.vars`:

```bash
PERPS_BENCH_API_URL=https://your-benchmark-api.example.com
PERPS_BENCH_API_USER=bench
PERPS_BENCH_API_PASSWORD=your-password
```

Deploy the dashboard:

```bash
cd frontend
npm run build
npx wrangler secret put PERPS_BENCH_API_URL --config wrangler.jsonc
npx wrangler secret put PERPS_BENCH_API_PASSWORD --config wrangler.jsonc
npm run deploy
```

The deployment flow uses `staging` as the local/default integration branch and
`main` as production. Pushes to `staging` deploy the separate Cloudflare Worker
`perps-latency-dashboard-staging`; merges to `main` deploy production
`perps-latency-dashboard`.

```bash
cd frontend
npm run deploy:staging
npm run deploy:production
```

Autodeploy can call the same npm scripts from the relevant branch. The Worker
runtime secrets `PERPS_BENCH_API_URL` and `PERPS_BENCH_API_PASSWORD` are set
directly in Cloudflare for each Worker environment.

## Safety

Live runs require `--confirm-live`.

Fill-likely order profiles, including market, IOC, FOK, and explicit
non-post-only orders, require `risk.allow_fill=true`,
`risk.neutralize_on_fill=true`, and strict after-sample cleanup. Start with
post-only/maker-style orders while validating setup.

Hyperliquid, Lighter, Aster, and Extended support cleanup of benchmark orders.
Cleanup runs outside the measured latency window. At startup, the runner checks
for stale orders from the same `run_id`; after the run, it reconciles that no
submitted benchmark orders remain open and the position did not change. Use
strict cleanup when cleanup failures should fail the sample:

```bash
--cleanup --cleanup-mode strict
```

## Supported Venue Status

- Hyperliquid: HTTPS/WebSocket order submission, confirmation tracking, and
  cleanup.
- Lighter: HTTPS/WebSocket transaction submission, confirmation tracking, and
  cleanup.
- Aster: HTTPS order/batch submission, private WebSocket confirmation, and cleanup.
- Extended: HTTPS order submission, parallel single-order batch benchmark,
  private WebSocket confirmation, and cleanup.

## Troubleshooting

- Missing env vars: run `accounts check` with the same `--env-file` flags you
  will use for the benchmark.
- Wrong wallet or key: run `accounts print` and compare the public identifiers
  with the venue UI.
- Lighter account errors: confirm `LIGHTER_ACCOUNT_INDEX`,
  `LIGHTER_API_KEY_INDEX`, and `LIGHTER_PRIVATE_KEY` match the active API key.
  If using maker-only mode, confirm `LIGHTER_MAKER_API_KEY_INDEX` is marked
  maker-only in Lighter.
- Lighter runner already active: use a separate Lighter API key for each
  concurrent runner, or stop the existing process using the same key.
- Config rejected for transport: the venue does not support that
  transport/scenario pair in this tool.
- Config rejected for risk: keep the order post-only while validating setup.
