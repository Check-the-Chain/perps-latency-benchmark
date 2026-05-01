# Benchmark Lifecycle

The benchmark currently implements the safe parts of the lifecycle:

1. Validate transport/scenario capabilities, builder params, and account env
   before live SDK-builder runs.
2. Build/sign payloads outside the measured network window.
3. Submit the already-built request in the measured region.
4. Optionally wait for a venue WebSocket order/trade confirmation and measure
   send-to-confirmation latency instead of submit response latency.
5. Classify the venue response as accepted, rejected, rate limited, auth error,
   nonce error, transport error, or unknown.
6. Run optional cleanup outside the measured network window.
7. Record classification and cleanup status in JSON results.

Measurement modes:

- `ack`: submit signed payload and measure until the venue returns a response.
- `ws_confirmation`: subscribe before submission and measure until the matching
  WebSocket order update or trade confirmation arrives.

For cross-venue comparison, prefer post-only `ws_confirmation` runs at a low
cadence, such as one sample per minute. This compares the time to an accepted
resting order update without relying on taker execution paths or venue-specific
market-order handling.

Fill-likely orders require venue-specific cleanup and inventory reconciliation
before they can be benchmarked repeatedly. Hyperliquid and Lighter support
strict after-sample neutralization for small fillable profiles.

Hyperliquid and Lighter have cleanup adapters for:

- cancel by client order ID
- startup stale-order cleanup for the same `run_id`
- after-run open-order reconciliation for submitted benchmark refs
- after-run position reconciliation against the startup position snapshot
- after-sample reduce-only neutralization when `risk.neutralize_on_fill` is set

Use post-only/maker-style profiles while validating setup. Only enable fillable
profiles with strict cleanup.

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
`sample.cleanup`. Result output includes a `run_id`; for Hyperliquid and
Lighter, generated client order identifiers are derived from that run ID,
iteration, market, side, and order offset unless the config explicitly provides
its own client ID fields.
