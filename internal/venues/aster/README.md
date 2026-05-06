# Aster Payload Builder

Aster is integrated through the current official futures V3 REST API:

- `POST /fapi/v3/order` for single orders.
- `POST /fapi/v3/batchOrders` for batch orders.
- `POST /fapi/v3/listenKey` plus `wss://fstream.asterdex.com/ws/<listenKey>`
  for private account stream confirmation.

Signed REST requests are `application/x-www-form-urlencoded` payloads using the
Aster API Wallet model. The signed string is the final encoded form/query string
without `signature`; it includes `user`, `signer`, and a microsecond `nonce`,
then signs that string as an EIP-712 `Message.msg` using
`ASTER_API_PRIVATE_KEY`.

The builder signs payloads without submitting them. For `ws_confirmation`, it
creates or reuses a `listenKey` outside the measured submit window so the Go
runner can subscribe before placing the order and measure to the matching
`ORDER_TRADE_UPDATE` event.
