# Perps Latency Benchmark

Go benchmark scaffold for measuring network-only latency to crypto perps venue
order submission endpoints.

The measured region is deliberately narrow:

1. Build and sign the venue payload outside the timer.
2. Start timer immediately before `http.Client.Do`.
3. Send the already-built request over Go's `net/http` transport.
4. Read and close the response body.
5. Stop timer.

Samples also include `httptrace` phase timings such as connection reuse, DNS, TCP,
TLS, request write, time to first response byte, response read, and total request
time.

The runner has two modes:

- Closed-loop sequential mode, the default, for one-at-a-time latency probes.
- Open-loop fixed-rate mode with `--rate`, `--max-in-flight`, `scheduled_at`,
  `start_delay_ns`, and `corrected_ns` fields so client stalls are visible instead
  of silently biasing the latency distribution.

## Why Go

Go gives us the pieces we need without writing a custom networking stack:

- `net/http` for a mature HTTP client and connection pooling.
- `net/http/httptrace` for request phase timings.
- `hdrhistogram-go` for latency summaries.
- `cobra` for a small CLI.

## Quick Start: Hyperliquid and Lighter

```bash
go test ./...
```

JSON configs should stay non-secret. Local credentials live in dotenv files that
are ignored by git. Start by generating the wallets used by the two starter
venues:

```bash
go run ./cmd/perps-bench accounts generate \
  --venues hyperliquid,lighter \
  --out .env.wallets.local
```

Then print the setup checklist. This shows the public wallet identifiers and the
manual venue setup still required:

```bash
go run ./cmd/perps-bench accounts checklist \
  --venues hyperliquid,lighter \
  --env-file .env.wallets.local
```

Complete the checklist before benchmarking:

- Hyperliquid: register/approve the printed EVM wallet if needed, fund the
  Hyperliquid account or agent wallet, then confirm `symbol`, `asset`, `size`,
  and `price` in [examples/hyperliquid-builder.json](examples/hyperliquid-builder.json).
- Lighter: use the printed Ethereum address for account creation/deposits,
  register or create the Lighter API key, fill `LIGHTER_ACCOUNT_INDEX` and
  `LIGHTER_API_KEY_INDEX` in `.env.wallets.local`, then confirm `market_index`,
  `base_amount`, and `price` in [examples/lighter-builder.json](examples/lighter-builder.json).

If Lighter generates API key material for you instead of registering the
generated `LIGHTER_PRIVATE_KEY`, replace `LIGHTER_PRIVATE_KEY` in
`.env.wallets.local` with the active key.

Verify that the required local env is present:

```bash
go run ./cmd/perps-bench accounts check \
  --venues hyperliquid,lighter \
  --env-file .env.wallets.local
```

Run a small Hyperliquid benchmark:

```bash
go run ./cmd/perps-bench run \
  --config examples/hyperliquid-builder.json \
  --env-file .env.wallets.local \
  --confirm-live
```

Run a small Lighter benchmark:

```bash
go run ./cmd/perps-bench run \
  --config examples/lighter-builder.json \
  --env-file .env.wallets.local \
  --confirm-live
```

Both example configs use post-only orders and build/sign payloads before the
measured network call. Keep them small while validating account setup. See
[docs/credentials.md](docs/credentials.md) for the full env-file layout and
per-venue variable list.

Useful account commands:

```bash
go run ./cmd/perps-bench accounts plan --venues hyperliquid,lighter
go run ./cmd/perps-bench accounts generate --venues hyperliquid,lighter --out .env.wallets.local
go run ./cmd/perps-bench accounts checklist --venues hyperliquid,lighter --env-file .env.wallets.local
go run ./cmd/perps-bench accounts print --venues hyperliquid,lighter --env-file .env.wallets.local
go run ./cmd/perps-bench accounts check --venues hyperliquid,lighter --env-file .env.wallets.local
```

## Run

Mock loopback benchmark:

```bash
go run ./cmd/perps-bench run \
  --venue mock \
  --iterations 10 \
  --warmups 2 \
  --mock-latency-ms 1
```

Fixed-rate/open-loop benchmark:

```bash
go run ./cmd/perps-bench run \
  --venue mock \
  --iterations 100 \
  --rate 20 \
  --max-in-flight 16
```

Risk settings are explicit. Fill-likely profiles such as market, IOC, FOK, or
non-post-only orders are rejected until a venue cleanup/neutralization adapter is
wired in. Maker-style validation can be enforced with:

```json
{
  "risk": {
    "require_post_only": true
  }
}
```

HTTPS vs WebSocket comparison:

```bash
go run ./cmd/perps-bench compare-transports \
  --venue hyperliquid \
  --body-file ./hyperliquid-http-order.json \
  --ws-body-file ./hyperliquid-ws-order.json \
  --iterations 50 \
  --warmups 5 \
  --rate 10 \
  --max-in-flight 4 \
  --confirm-live \
  --output results/hyperliquid-transports.json
```

`compare-transports` runs each transport as a separate benchmark variant using
the same scenario, rate, warmups, iterations, timeout, and connection settings.
That matches the way mature tools separate HTTP and WebSocket metrics: compare
tagged result streams after the run instead of mixing protocols in one histogram.

Generic prebuilt HTTP payload:

```bash
go run ./cmd/perps-bench run \
  --venue http \
  --url https://example.com/api \
  --body-file ./signed-order.json \
  --iterations 10 \
  --confirm-live
```

Initial venue adapters are prebuilt-payload adapters with venue defaults:

- Hyperliquid defaults to `https://api.hyperliquid.xyz/exchange`.
- Aster defaults to `https://fapi.asterdex.com/fapi/v3/order` and
  `https://fapi.asterdex.com/fapi/v3/batchOrders`.
- edgeX defaults to `https://pro.edgex.exchange/api/v1/private/order/createOrder`.
- Lighter defaults to `https://mainnet.zklighter.elliot.ai/api/v1/sendTx` and
  `sendTxBatch` for batch tests.
- Variational Omni is registered, but official docs do not currently verify an
  order submission API, so endpoints are intentionally empty.
- GRVT defaults to `https://trades.grvt.io/full/v1/create_order`,
  `https://trades.grvt.io/full/v2/bulk_orders`, and
  `wss://trades.grvt.io/ws/full`.
- Extended defaults to `https://api.starknet.extended.exchange/api/v1/user/order`.

Venues can use `--transport websocket` only when the venue definition has a
verified order-submission WebSocket URL and the body is already wrapped in the
venue's WebSocket submission format. Several venues document WebSockets only for
market-data or account-update streams, not order entry; those definitions leave
the WebSocket order URL empty on purpose.

That keeps signing out of the benchmark. For accepted live orders, prefer a
builder so the SDK prepares fresh signing material before each timed request.

## Payload Builders

For live runs, use a builder instead of replaying fixed JSON. Builders run before
the measured network call and return a fresh body, headers, and optional
WebSocket body for that iteration.

The preferred SDK builder type is `persistent_command`: perps-bench starts one
long-lived process, sends one JSON request per stdin line, and expects one JSON
response per stdout line. This keeps SDK imports and Python process startup out
of repeated request preparation. The simpler `command` type is still available
for one-shot/debug builders; it sends one JSON request on stdin and reads one
JSON response from stdout.

Builder request fields include:

- `venue`
- `transport`
- `scenario`
- `iteration`
- `batch_size`
- `requested_at`
- `params`

Builder response fields include:

- `headers`
- `body` or `body_base64`
- `batch_body` or `batch_body_base64`
- `ws_body` or `ws_body_base64`
- `ws_batch_body` or `ws_batch_body_base64`
- `metadata`

SDK-backed examples are included for:

- Hyperliquid: [internal/venues/hyperliquid/build_payload.py](internal/venues/hyperliquid/build_payload.py)
- Lighter: [internal/venues/lighter/build_payload.py](internal/venues/lighter/build_payload.py)
- edgeX: [internal/venues/edgex/build_payload.py](internal/venues/edgex/build_payload.py)
- GRVT: [internal/venues/grvt/build_payload.py](internal/venues/grvt/build_payload.py)
- Extended: [internal/venues/extended/build_payload.py](internal/venues/extended/build_payload.py)

Aster and Variational Omni are documented as gaps for now: Aster does not expose
a suitable official V3 SDK payload-builder path, and current public Variational
Omni docs do not verify a trading/order submission API.

Example configs:

```bash
go run ./cmd/perps-bench run --config examples/hyperliquid-builder.json --confirm-live
go run ./cmd/perps-bench run --config examples/lighter-builder.json --confirm-live
go run ./cmd/perps-bench run --config examples/edgex-builder.json --confirm-live
go run ./cmd/perps-bench run --config examples/grvt-builder.json --confirm-live
go run ./cmd/perps-bench run --config examples/extended-builder.json --confirm-live
```

The Python builders are intended to be run through `uv run --with ...`, support
both one-shot and line-oriented persistent modes, and keep signing outside the
timed path.

Venue definitions also declare verified capabilities for HTTPS single, HTTPS
batch, WebSocket single, and WebSocket batch submission. The CLI rejects
transport/scenario combinations that are not verified for that venue instead of
silently falling back to a slower or undocumented path.

WebSocket body formats:

- Hyperliquid: `{ "method": "post", "id": ..., "request": { "type": "action", "payload": ... } }`
- Lighter: `{ "type": "jsonapi/sendtx", "data": ... }` or
  `{ "type": "jsonapi/sendtxbatch", "data": ... }`
- GRVT: JSON-RPC messages using `v1/create_order` or `v2/bulk_orders`.

Use `body_file` / `batch_body_file` for HTTPS payloads and `ws_body_file` /
`ws_batch_body_file` for WebSocket payloads. The payloads usually differ because
WebSocket order submission often wraps the signed action in a protocol message.

## Output

```bash
go run ./cmd/perps-bench run \
  --config examples/benchmark.json \
  --output results/run.json \
  --csv results/run.csv
```

Use `--latency-mode total` to summarize full response-read latency, or
`--latency-mode ttfb` to summarize time to first response byte.
