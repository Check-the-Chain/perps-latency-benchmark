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

## Safety

Live runs require `--confirm-live`.

Fill-likely order profiles, including market, IOC, FOK, and explicit
non-post-only orders, are blocked. Use post-only/maker-style orders while
validating repeatability.

Hyperliquid and Lighter support best-effort cleanup of benchmark orders by
client order identifier. Cleanup is recorded in JSON samples and is not included
in latency summaries. The summary prints cleanup attempted/ok/failed/skipped
counts. Each run also gets a `run_id`, and Hyperliquid/Lighter client order IDs
are derived from it. Pass `--run-id` when you want a human-readable or
externally supplied run identifier. Use strict cleanup only when you want
cleanup failures to fail the sample:

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
