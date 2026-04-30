# Benchmark Lifecycle

The benchmark currently implements the safe parts of the lifecycle:

1. Validate transport/scenario capabilities, builder params, and account env
   before live SDK-builder runs.
2. Build/sign payloads outside the measured network window.
3. Submit the already-built request in the measured region.
4. Classify the venue response as accepted, rejected, rate limited, auth error,
   nonce error, transport error, or unknown.
5. Record classification in JSON/CSV results.

Fill-likely orders are intentionally blocked for now. Market, IOC, FOK, and
explicit non-post-only profiles require venue-specific cleanup and inventory
reconciliation before they should be benchmarked repeatedly.

The next lifecycle layer should add concrete venue adapters for:

- cancel by client order ID
- list open benchmark orders by run ID/client ID prefix
- get current position
- neutralize unexpected or intentional fills

Until those adapters exist, use post-only/maker-style profiles for live repeated
runs.

Risk config:

```json
{
  "risk": {
    "require_post_only": true,
    "allow_fill": false,
    "neutralize_on_fill": false
  }
}
```

Cleanup policy and max-notional enforcement are not exposed yet because concrete
venue cleanup/reconciliation adapters are still intentionally unwired.
