# Aster Payload Builder Status

No SDK-backed Aster payload builder is included yet.

Official sources checked:

- https://github.com/asterdex/api-docs
- https://github.com/asterdex/api-docs/blob/master/V3(Recommended)/EN/aster-finance-futures-api-v3.md
- https://github.com/asterdex/api-docs/blob/master/Aster%20API%20Overview.md
- https://github.com/asterdex/aster-connector-python
- https://github.com/asterdex/aster-broker-pro-sdk

The current official API docs mark V3 as recommended for new futures integrations.
V3 order authentication requires signer, nonce, and signature parameters, with
API Wallet signing examples shown in the docs. The official `aster-connector-python`
package is a lightweight REST connector for the public API, but its order methods
target `/fapi/v1/order` and `/fapi/v1/batchOrders`, use API-key HMAC signing, and
submit requests through the client rather than exposing a V3 signed-payload builder.
The official broker SDK is a hosted web trading UI integration, not a local order
payload signing SDK.

Because there is no official SDK/package suitable for building V3 signed order
payloads without sending them, this venue intentionally does not hand-roll Aster
signing crypto in `build_payload.py`.
