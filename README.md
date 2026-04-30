# Perps Latency Benchmark

Benchmark crypto perps order submission latency.

## Prerequisites

- Go 1.25+
- `uv` for live Hyperliquid/Lighter runs
- Venue accounts funded and configured before using `--confirm-live`

## Quick Start: Hyperliquid and Lighter

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
  then adjust the BTC order params in `examples/lighter-builder.json` if
  needed.

Verify that required local environment is present:

```bash
go run ./cmd/perps-bench accounts check \
  --venues hyperliquid,lighter \
  --env-file .env.wallets.local
```

Run Hyperliquid:

```bash
go run ./cmd/perps-bench run \
  --config examples/hyperliquid-builder.json \
  --env-file .env.wallets.local \
  --confirm-live
```

Run Lighter:

```bash
go run ./cmd/perps-bench run \
  --config examples/lighter-builder.json \
  --env-file .env.wallets.local \
  --confirm-live
```

The starter configs use small post-only BTC orders. Keep them small until
account setup is confirmed. Hyperliquid and Lighter starter configs also cancel
benchmark orders after each measured submit, outside the latency window.

## Account Commands

```bash
go run ./cmd/perps-bench accounts plan --venues hyperliquid,lighter
go run ./cmd/perps-bench accounts generate --venues hyperliquid,lighter --out .env.wallets.local
go run ./cmd/perps-bench accounts checklist --venues hyperliquid,lighter --env-file .env.wallets.local
go run ./cmd/perps-bench accounts print --venues hyperliquid,lighter --env-file .env.wallets.local
go run ./cmd/perps-bench accounts check --venues hyperliquid,lighter --env-file .env.wallets.local
```

See `docs/credentials.md` for the full env-file layout.

## Output Files

Write JSON and CSV results:

```bash
go run ./cmd/perps-bench run \
  --config examples/hyperliquid-builder.json \
  --env-file .env.wallets.local \
  --confirm-live \
  --output results/hyperliquid.json \
  --csv results/hyperliquid.csv
```

Summaries default to full response latency. Use TTFB instead with:

```bash
--latency-mode ttfb
```

Compare saved result files:

```bash
go run ./cmd/perps-bench compare-results \
  results/hyperliquid.json \
  results/lighter.json
```

## HTTPS vs WebSocket

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
  --rate 0.2 \
  --chunk-iterations 12 \
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
PERPS_BENCH_API_URL=http://ec2-18-183-225-52.ap-northeast-1.compute.amazonaws.com:8080
PERPS_BENCH_API_USER=bench
PERPS_BENCH_API_PASSWORD=your-password
```

Deploy the dashboard:

```bash
cd frontend
npm run build
npx wrangler secret put PERPS_BENCH_API_PASSWORD --config wrangler.jsonc
npm run deploy
```

## Safety

Live runs require `--confirm-live`.

Fill-likely order profiles, including market, IOC, FOK, and explicit
non-post-only orders, require `risk.allow_fill=true`,
`risk.neutralize_on_fill=true`, and strict after-sample cleanup. Start with
post-only/maker-style orders while validating setup.

Hyperliquid and Lighter support cleanup of benchmark orders by client order
identifier. Cleanup runs outside the measured latency window. At startup, the
runner checks for stale orders from the same `run_id`; after the run, it
reconciles that no submitted benchmark orders remain open and the position did
not change. Each run gets a `run_id`, and Hyperliquid/Lighter client order IDs
are derived from it. Pass `--run-id` when you want a human-readable or
externally supplied run identifier. Use strict cleanup when you want cleanup
failures to fail the sample:

```bash
--cleanup --cleanup-mode strict
```

## Supported Venue Status

- Hyperliquid: HTTPS and WebSocket single/batch order submission.
- Lighter: HTTPS and WebSocket single/batch transaction submission.
- GRVT: HTTPS and WebSocket single/batch order submission.
- edgeX: HTTPS single order submission.
- Extended: HTTPS single order submission.
- Aster: HTTPS endpoints documented, live builder not included yet.
- Variational Omni: registered, but current public docs do not verify order
  submission.

Start with Hyperliquid and Lighter unless you have venue-specific credentials
and metadata prepared for another venue.

## Troubleshooting

- Missing env vars: run `accounts check` with the same `--env-file` flags you
  will use for the benchmark.
- Wrong wallet or key: run `accounts print` and compare the public identifiers
  with the venue UI.
- Lighter account errors: confirm `LIGHTER_ACCOUNT_INDEX`,
  `LIGHTER_API_KEY_INDEX`, and `LIGHTER_PRIVATE_KEY` match the active API key.
- Config rejected for transport: the venue does not support that
  transport/scenario pair in this tool.
- Config rejected for risk: keep the order post-only while validating setup.
