# Benchmark Lifecycle

The benchmark currently implements the safe parts of the lifecycle:

1. Validate transport/scenario capabilities, builder params, and account env
   before live SDK-builder runs.
2. Build/sign payloads outside the measured network window.
3. Submit the already-built request in the measured region.
4. Classify the venue response as accepted, rejected, rate limited, auth error,
   nonce error, transport error, or unknown.
5. Run optional cleanup outside the measured network window.
6. Record classification and cleanup status in JSON results.

Fill-likely orders are intentionally blocked for now. Market, IOC, FOK, and
explicit non-post-only profiles require venue-specific cleanup and inventory
reconciliation before they should be benchmarked repeatedly.

Hyperliquid and Lighter have cleanup adapters for:

- cancel by client order ID

The next lifecycle layer should add concrete venue adapters for:

- list open benchmark orders by run ID/client ID prefix
- get current position
- neutralize unexpected or intentional fills

Until position adapters exist, use post-only/maker-style profiles for live
repeated runs.

Risk config:

```json
{
  "risk": {
    "require_post_only": true,
    "allow_fill": false,
    "neutralize_on_fill": false
  },
  "cleanup": {
    "enabled": true,
    "mode": "best_effort",
    "scope": "after_sample",
    "timeout_ms": 5000
  }
}
```

Cleanup modes:

- `best_effort`: record cleanup status without failing the benchmark sample.
- `strict`: fail the sample if cleanup fails.

Cleanup is not included in `network_ns`; it is stored separately under
`sample.cleanup`.
