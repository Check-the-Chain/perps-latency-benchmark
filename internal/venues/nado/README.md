# Nado Venue Notes

Nado's fastest verified submit path is the gateway execute WebSocket:

- Mainnet gateway WS: `wss://gateway.prod.nado.xyz/v1/ws`
- Mainnet gateway REST: `https://gateway.prod.nado.xyz/v1/execute`
- Mainnet subscription WS: `wss://gateway.prod.nado.xyz/v1/subscribe`

Order submit payloads are the same top-level `place_order` JSON over WebSocket and REST. The builder signs the EIP-712 `Order` locally, emits both `body` and `ws_body`, and does not perform network I/O. Batch benchmarking is manual fanout: Nado has no documented native multi-place-order endpoint, so batch scenarios emit concurrent single-order REST executes.

Speed notes:

- Prefer WebSocket order entry to avoid HTTP setup and the documented HTTP `Accept-Encoding` requirement.
- Batch scenarios use concurrent HTTPS `place_order` executes, matching the
  Extended manual-fanout model. Label these as manual/fanout batch, not native
  batch.
- Keep the gateway WebSocket warm. Nado requires ping frames every 30 seconds; the benchmark sends WebSocket protocol ping frames after 25 seconds of idle time.
- `nonce` includes a discard/recv timestamp in the high 44 bits. The example uses `recv_window_ms=5000` for safe local testing; lower it only when prepare-to-send delay is known and tight.
- Post-only appendix is `1537`: version 1 plus POST_ONLY in bits 9-10.
- Post-only avoids Nado's documented 50ms non-post-only speed bump, but it does
  not get a separate higher rate limit. Place-order limits are 600/minute or
  100/10s per wallet with spot leverage, and 30/minute or 5/10s without spot
  leverage. `spot_leverage` is only valid on spot products; omit it for perps
  like `BTC-PERP`.
- Confirmation uses the authenticated subscription WebSocket and matches
  `order_update` events by signed order digest. Nado's subscription endpoint
  requires permessage-deflate with context takeover, so this path uses the
  compression-capable confirmation WebSocket client rather than Gorilla.
- Public docs do not state that the gateway or sequencer is AWS Tokyo based.
  Public DNS currently exposes `gateway.prod.nado.xyz` through Cloudflare, so
  edge DNS does not prove backend or sequencer region.

Credential requirements:

- `NADO_PRIVATE_KEY`: EVM wallet or linked signer private key.
- `NADO_SENDER`: 32-byte subaccount sender. If omitted, the builder derives `wallet address + subaccount name padded to 12 bytes`.
- `NADO_ENDPOINT_CONTRACT`: endpoint verifying contract from Nado's contracts query. Required for subscription authentication and cancellation signing.
- `NADO_CHAIN_ID`: defaults to Ink mainnet `57073`; set `763373` for Ink Sepolia.

References:

- https://docs.nado.xyz/developer-resources/api/endpoints
- https://docs.nado.xyz/developer-resources/api/gateway
- https://docs.nado.xyz/developer-resources/api/gateway/executes/place-order
- https://docs.nado.xyz/developer-resources/api/gateway/executes/cancel-orders
- https://docs.nado.xyz/developer-resources/api/gateway/signing
- https://docs.nado.xyz/developer-resources/api/subscriptions
- https://docs.nado.xyz/developer-resources/api/subscriptions/streams
- https://docs.nado.xyz/developer-resources/api/subscriptions/events
- https://docs.inkonchain.com/general/network-information
