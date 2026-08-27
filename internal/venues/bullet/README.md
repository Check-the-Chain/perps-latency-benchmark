# Bullet

Bullet is a Sovereign-SDK rollup. Orders are borsh-serialized, ed25519-signed
transactions submitted as an opaque base64 `tx` blob, either over WebSocket
`order.place` or as `POST /tx/submit`.

## Files

- `build_payload.mjs` — order payload construction and confirmation metadata.
- `cancel_payload.mjs` — cleanup cancel payloads.
- `bullet.go` — venue definition and response classifier.
- `confirmation.go` — submit and cancel confirmation over `ADDRESS@user.orders`.

## Signing

Signing is delegated to the official Bullet WASM SDK
([`@bulletxyz/sdk-wasm`](https://www.npmjs.com/package/@bulletxyz/sdk-wasm)),
pinned in `package.json`. The wire encoding is therefore the exchange's own
code rather than a transcription of it, which removes an entire class of
silent-mismatch bug. Payload building runs outside the measured latency window.

Setup:

```bash
cd internal/venues/bullet && npm ci
```

`node_modules/` is git-ignored; `package-lock.json` is committed. Node 20+.

Two SDK details worth knowing before editing these files:

- **wasm-bindgen objects have move semantics.** Passing a `Keypair` to both
  `Client.builder().keypair()` and `TransactionBuilder.signer()` frees it on the
  first use and the second throws `null pointer passed to rust`. Set the keypair
  once on the client and let it act as the default signer.
- **Building the `Client` fetches exchange metadata** and takes roughly 2-4
  seconds. It happens once per process and is cached, but the builder
  `timeout_ms` in the example configs is raised to 20000 to accommodate it.

## Mainnet only

This venue ships mainnet configs only. `@bulletxyz/sdk-wasm` releases have at times failed with
`Schema outdated - recompile the binary to update bullet-exchange-interface`
against testnet, which has been upgraded ahead of mainnet and ahead of the
pinned SDK. Mainnet is unaffected. If you want a testnet config later, take a
mainnet one, set `"network": "testnet"` and point `request.ws_url` at the
testnet host — and expect to need an SDK at least as new as the one pinned here.

Worth noting the failure mode: the SDK raises at client construction rather
than emitting a silently malformed transaction, which is the behaviour you
want from a signing library.

## Network selection

The network is a single builder param — `"network": "mainnet"` or
`"testnet"` — which the SDK resolves to the correct REST host, WebSocket host,
chain id, and chain hash together, so the *signing* side can never be half
configured. There is no environment variable for the host.

The Go runner still needs its own endpoint — `request.ws_url` for WebSocket,
`request.url` for HTTP — so every shipped config sets that alongside `network`
in the same file. A hand-written config that changes only one of the two can
still point the socket and the signed chain parameters at different networks;
change them together.

## Transaction uniqueness

The builders do not call `.window()`. It is already the SDK's default mode,
seeded with a **microsecond** timestamp; supplying a millisecond value instead
cuts resolution 1000x, and two transactions signed by the same delegate inside
one millisecond collide, the second being reported `dropped`. Order submission
and cleanup cancellation share the credential, so an order and its own cancel
can land in the same millisecond. Do not pin a uniqueness value in a config:
every transaction after the first would be a guaranteed drop.

## Client order ids

Generated ids are kept below 2^52. They are u64 on the wire and the SDK accepts
an unbounded `bigint`, but the Go confirmation path
(`internal/venues/confirmutil.Text`) round-trips JSON numbers through float64.
Any id at or above 2^53 loses precision, which silently breaks confirmation
matching and surfaces as phantom timeouts rather than an obvious error. An
explicit out-of-range `client_order_id` is rejected rather than clamped.

## Batch comparability

Bullet's batch places `Vec<NewOrderArgs>` in **one transaction under a single
signature**. Venues such as Pacifica sign each batch action individually. Bullet
batch latency is therefore structurally more favourable and is not measuring the
same work. Samples carry `batch_signing_model: "single_signature_tx"` so this
stays visible in the data.

## Transports

Only WebSocket configs ship. HTTP `POST /tx/submit` is supported by the venue
definition and handled by the classifier — it carries the identical signed
transaction — but no example config uses it, because WebSocket is roughly twice
as fast and is the path every comparable venue is benchmarked on. To measure the
HTTP path, copy a WebSocket config, set `"transport": "https"`, and add
`request.url` pointing at `<host>/tx/submit`.

## Other constraints

- Bullet timestamps are microseconds since epoch, not milliseconds.
- Reads and the `user.orders` topic key on the **main account address**, never
  the delegate address.
- `submitted` is the normal happy-path acknowledgement, not `processed`; it is
  classified accepted, with book entry verified separately by the confirmation
  match.
- The delegate public key is printed base58 by `accounts checklist`, which is
  the encoding Bullet accepts; `/api/v1/delegateOf` rejects the hex form of the
  same bytes.

## No taker config

Taker configs are not shipped. `immediate_or_cancel` is fill-likely, which
`lifecycle.FillLikely` correctly gates behind `risk.allow_fill` and in turn
`risk.neutralize_on_fill` — and Bullet implements no position neutralization,
so a filled order could not be unwound. `bullet.go` deliberately does not claim
`Capabilities.Neutralization`. Add neutralization support to the builders
before adding a taker config.

## Operational note: `PERPS_BENCH_PYTHON` does not apply here

`internal/payload/command.go` rewrites `uv run … python script.py` invocations
into a direct interpreter call when `PERPS_BENCH_PYTHON` is set, so long-running
services can skip the `uv` wrapper. That rewrite is gated on `argv[0]` being
`uv`, so it silently no-ops for this venue's `node` command. Nothing breaks —
Bullet never used `uv` — but an operator setting that variable expecting it to
speed up every persistent builder will not see any effect here.
