# RISEx Venue Notes

RISEx is a fully onchain perpetuals exchange on RISE Chain (an EVM L2,
mainnet chain ID 4153). Its docs are inconsistent in places (see the
mainnet host caveat below); verify details against `GET /v1/system/config`
before relying on them.

Endpoints:

- Mainnet REST: `https://api.rise.trade`
- Mainnet WS: `wss://ws.rise.trade/ws`
- Testnet REST: `https://api.testnet.rise.trade`
- Testnet WS: `wss://ws.testnet.rise.trade`

**Mainnet host caveat**: the docs disagree with themselves about mainnet
hostnames — `ws-connection.md` says `wss://ws.risex.trade` (does not
resolve), `endpoints.md` says `wss://ws.rise.trade/ws` (resolves), and no
page states a mainnet REST base URL (every REST example uses
`api.testnet.rise.trade`). `https://api.rise.trade` and
`wss://ws.rise.trade/ws` are the verified live values — `GET
/v1/system/config` on that host returns `chain_id: 4153` and router/auth
addresses matching the docs' published contracts. Re-verify against `GET
/v1/system/config` before a live run in case this changes.

## Auth model: EIP-712 permits via a registered session signer

Unlike venues that sign each order with the main wallet key directly, RISEx
uses session-key delegation:

1. **One-time setup** (not part of the benchmark's timed path): the main
   account deposits collateral (first deposit registers the account), then
   registers a session signer key via `POST /v1/auth/register-signer` —
   two EIP-712 signatures (`RegisterSigner` from the account, `VerifySigner`
   from the signer) consuming a `(nonceAnchor, nonceBitmap)` pair from `GET
   /v1/nonce-state/{account}`. Run `register_signer.py` for this
   (`--dry-run` first to review, then `--confirm-live` to submit — it's a
   real mainnet action).
2. **Per-order signing** (fully local, no network): every place/cancel
   request is authorized by a `VerifyWitness` EIP-712 permit signed by the
   *session key* over a locally-computed action hash:
   `VerifyWitness{account, target: router, hash: action_hash, nonceAnchor,
   nonceBitmap, deadline}`. The signature is **base64-encoded, not hex**, in
   EIP-2098 compact form (64 bytes: `r || s` with the top bit of `s` set when
   `v == 28`).
3. **WebSocket auth** for the private `orders` channel is a *third*, separate
   EIP-712 flow (`auth_v2`): fetch a one-time server nonce from `GET
   /v1/auth/nonce` (bare 32-byte hex, no `0x` prefix), sign `RegisterV2{signer,
   message, nonce}` with the session key, send `{"method":"auth_v2","params":
   {...}}` over the socket. This nonce is single-use and short-lived, so
   `build_payload.py` caches the signed frame and only refreshes it every
   `WS_AUTH_REFRESH_SECS` (240s) rather than on every request.

## Nonce scheme

`nonce_anchor` must be **exactly** `(current nonce_anchor from GET
/v1/nonce-state/{account}) + 1` — a strict sequence position to *open* an
anchor, not a free choice of any unused value; any other value reverts with
`InvalidNonceAnchor(account, given, expected)`.

`build_payload.py` fetches a fresh anchor at the start of every `Build()`
call (`open_next_nonce_anchor()`) rather than caching one per process: a
cached anchor goes stale the moment `cancel_payload.py` (a separate process
that always opens a fresh anchor per cleanup call) opens the next one,
causing every subsequent order in the run to revert with
`InvalidNonceAnchor`. This fetch is free in the reported latency —
`Build()` time is tracked separately as `prepared_ns` and is never counted
in `network_ns`/`latency_ms` (see `internal/bench/runner.go`); only the
HTTP round trip after `Build()` returns is measured. `cancel_payload.py`
also always fetches a fresh anchor, same reasoning.

## order_data bit-packing and header_flags

The action hash for `place_order` is
`keccak(keccak(b"RISE_PERPS_PLACE_ORDER_V1") + word(headerFlags) +
word(order_data) + word(builderId) + [word(builderFeeBps) only if > 0] +
word(clientOrderId) + word(ttlUnits))`, per RISEx's published spec
(`https://developer.rise.trade/reference/integration.md`). `order_data`
packs `market_id`, `size_steps`, `price_ticks`, and a `flags` nibble into a
single integer. `header_flags` bit `0x01` is always set (permit present);
`0x02` when `builder_id != 0`; `0x04` when `client_order_id != 0`; `0x10`
when `ttl_units != 0` — and the corresponding word must carry its real
value, not `0`, whenever its bit is set, since RISEx's router recomputes
the hash from the request body it actually receives and rejects any
mismatch as `SignerNotAuthorized`.

Verified end-to-end against live mainnet: 10/10 real post-only/GTC/limit
buy orders placed, confirmed over the private WS channel, and cancelled
(with WS cancel confirmation) in a single benchmark run with cleanup
enabled, leaving zero open orders.

`pack_order_data()` only supports `order_type=limit` and
`time_in_force=GTC`, and raises rather than guess at bit positions for
market orders, IOC/FOK, GTT, or reduce_only — those combinations are
unverified. Before relaxing those checks, confirm the same way: a signed
order that reaches a balance/business-logic error, or succeeds outright,
rather than a signature error, is proof the hash matched.

Cancel's action hash is simpler:
`keccak(keccak(b"RISE_PERPS_CANCEL_ORDER_V1") + word(1) + word(resting_order_id))`
— no optional-field complication, since cancel only ever carries
`resting_order_id`.

**`order_id` wire format**: `POST /v1/orders/cancel`'s `order_id` field
must be a **24-byte, `0x`-prefixed hex string**, not decimal (the API
rejects other lengths with `invalid order_id length: expected 24, got N`).
`cancel_payload.py` passes through whatever `GET /v1/orders/open` returns
verbatim in its `order_id`/`id` field, which is already in this format.

## Batch scenario is not supported (refuses outright, does not fake it)

`build_payload.py`'s `build()` raises `SystemExit` immediately for
`benchmark.scenario: "batch"`, and `Capabilities.HTTPBatch` is `false` in
`risex.go`. There is no `risex-batch5-builder.json` example, matching
edgeX's precedent for venues with no real batch endpoint.

Concurrent single-order fanout (this repo's usual workaround for venues
without a native multi-order endpoint, used by Nado/Extended) does not work
for RISEx: `nonce_anchor` is a strict per-account sequence, so concurrent
submissions can't guarantee RISEx processes them in the order the client
assigned anchors for — unlike Nado/Extended, whose nonces are time-based
and tolerate concurrent submission. In a live `batch_size=5` test, only 1
of 5 concurrently-submitted orders actually succeeded (the other 4 were
silently rejected, most likely `InvalidNonceAnchor`), yet the sample still
reported `ok`, because `DoParallelFastest` only measures the first of the N
requests to complete, not whether all N succeeded. Refusing outright avoids
reporting a misleading number for a near-total failure rate.

If RISEx ever offers a real batch endpoint, or their backend is confirmed
to process same-account concurrent submissions in nonce order, batch
support could be added the way Aster/GRVT/Hyperliquid/Lighter/Pacifica use
their venues' real batch endpoints. Single-scenario numbers are unaffected.

## Order identity: client_order_id, not a client-known digest

Unlike Nado (whose EIP-712 order digest is deterministic and known before
sending, so it can be used to match WS confirmation events), RISEx's
`order_id`/`resting_order_id` are server-generated and only known after the
REST response returns. `build_payload.py` instead sets the REST request's
`client_order_id` field (a client-chosen uint64) and uses that as the
identity threaded through `cleanup_orders` metadata and WS confirmation
matching (verified live as part of the 10/10 run described above).
`cancel_payload.py` resolves `resting_order_id` by querying `GET
/v1/orders/open?account=...&market_id=...` and matching on
`client_order_id` — one HTTP GET per cleanup call, acceptable since cleanup
runs outside the timed benchmark path.

## Rate limits and other notes

- REST: 500 requests/10s/IP. WebSocket: 10 requests/s/IP.
- RISEx has no documented WebSocket order-entry endpoint — order submission
  is REST-only (`POST /v1/orders/place`); the WS connection is only used for
  market data and the private `orders`/`fills`/`positions` confirmation
  channels.
- `price`/`amount` in benchmark params are human units (e.g. `"60000"`,
  `"0.001"`); the builder converts to `price_ticks`/`size_steps` using each
  market's `step_price`/`step_size` from `GET /v1/markets`, cached per
  process after the first lookup for a given `market_id`.

Credential requirements:

- `RISEX_PRIVATE_KEY`: EVM private key for the main account (holds
  collateral, registers the signer). Alternative: `RISEX_ACCOUNT_ADDRESS`
  (address only, no private key) if you don't have or don't want to export
  the main account's raw key — e.g. a hardware wallet, or a signer already
  registered through RISEx's website UI.
- `RISEX_SIGNER_PRIVATE_KEY`: EVM private key for the registered session
  signer. Must differ from `RISEX_PRIVATE_KEY`; cannot be auto-generated by
  `accounts generate` the way a plain wallet key can, since registration
  requires two live signatures posted to the API (see `accounts plan
  --venues risex` for the manual steps).
- `RISEX_BASE_URL` (optional): override the REST base URL (defaults to
  `https://api.rise.trade`; see the mainnet host caveat above).
- `RISEX_NONCE_ANCHOR` (optional): pin a starting nonce anchor instead of
  fetching one from `GET /v1/nonce-state/{account}`.

References:

- https://docs.risechain.com/docs/risex
- https://developer.rise.trade/reference/general-information
- https://developer.rise.trade/reference/integration
- https://developer.rise.trade/reference/orderservice_placeorder
- https://developer.rise.trade/reference/orderservice_cancelorder
- https://developer.rise.trade/reference/authservice_geteip712domain
- https://developer.rise.trade/reference/authservice_registersigner
- https://developer.rise.trade/reference/apiservice_getsystemconfig
- https://developer.rise.trade/reference/marketservice_getmarkets
- https://developer.rise.trade/reference/ws-connection
- https://developer.rise.trade/reference/orders-channel
- https://developer.rise.trade/reference/authentication-3
