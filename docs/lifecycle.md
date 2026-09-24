# Benchmark Lifecycle

Live runs follow this order:

1. Validate venue support, builder params, and account env.
2. Build and sign payloads outside the measured path.
3. Submit the prepared request.
4. Optionally wait for the matching WebSocket order/trade update.
5. Classify the result.
6. Run cleanup outside the measured path.
7. Write samples and cleanup status.

Measurement modes:

- `ack`: measure submit response latency.
- `ws_confirmation`: measure from submit to the matching WebSocket confirmation.

For cross-venue runs, prefer low-cadence post-only `ws_confirmation` samples.
That measures accepted resting-order updates without depending on taker fills.

Fill-likely profiles need strict cleanup. Hyperliquid, Lighter, Aster, edgeX,
and Extended can cancel benchmark orders, reconcile open orders, check position
drift, and neutralize small fills after each sample.

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

- `best_effort`: record cleanup status.
- `strict`: fail the sample if cleanup fails.

Cleanup is not included in request latency. Results include a `run_id`; generated
order IDs are derived from that ID unless the config supplies explicit IDs.
