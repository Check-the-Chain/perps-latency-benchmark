# Pacifica Venue Notes

Pacifica's fastest verified submit path is the native trading WebSocket:

- Mainnet WS: `wss://ws.pacifica.fi/ws`
- Mainnet REST base: `https://api.pacifica.fi/api/v1`

The builder signs Ed25519 messages locally using Pacifica's deterministic JSON
format and emits websocket payloads only. It does not perform network I/O.

Speed notes:

- Prefer WebSocket order entry to avoid HTTP setup.
- Use `tif=ALO` or `tif=TOB` for maker latency. Pacifica documents roughly
  200 ms latency protection for market orders and GTC/IOC limit orders.
- Post-only orders still consume normal action credits. Pacifica's public rate
  limits are credit-based over a 60-second rolling window: unidentified IPs get
  125 credits, valid API config keys start at 300 credits, and fee tiers raise
  account quota. Standard actions cost 1 credit; cancels cost 0.5 credits.
- Native websocket batch orders support up to 10 actions. Batches containing
  market or GTC/IOC limit actions have a randomized 50-100 ms delay; all-ALO,
  all-TOB, cancel-only, or TP/SL-only batches avoid that delay.
- API agent wallets can sign for the main account; set `PACIFICA_ACCOUNT` and
  `PACIFICA_AGENT_WALLET` when using an agent key.
- Public docs do not state the matching-engine or order-entry region. Public
  DNS currently exposes `ws.pacifica.fi` through AWS Global Accelerator and
  `api.pacifica.fi` through CloudFront, so edge DNS alone does not prove AWS
  Tokyo placement.

Credential requirements:

- `PACIFICA_PRIVATE_KEY`: base58 Solana private key for the account or agent.
- `PACIFICA_ACCOUNT`: main Pacifica account public key. Optional when the
  private key is the account key.
- `PACIFICA_AGENT_WALLET`: registered agent wallet public key. Optional when
  the private key is the account key.

References:

- https://docs.pacifica.fi/api-documentation/api
- https://docs.pacifica.fi/api-documentation/api/websocket
- https://docs.pacifica.fi/api-documentation/api/websocket/trading-operations/create-limit-order
- https://docs.pacifica.fi/api-documentation/api/websocket/trading-operations/create-order
- https://docs.pacifica.fi/api-documentation/api/websocket/trading-operations/batch-order
- https://docs.pacifica.fi/api-documentation/api/signing
- https://github.com/pacifica-fi/python-sdk
