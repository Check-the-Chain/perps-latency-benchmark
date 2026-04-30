#!/usr/bin/env python3
"""Build signed Hyperliquid cancelByCloid payloads for benchmark cleanup."""

from __future__ import annotations

import json
import os
import sys
import time
from typing import Any


def main() -> int:
    try:
        from eth_account import Account
        from hyperliquid.utils.constants import MAINNET_API_URL
        from hyperliquid.utils.signing import sign_l1_action
    except ImportError as exc:
        raise SystemExit(
            "missing Hyperliquid SDK dependencies; run with "
            "`uv run --with hyperliquid-python-sdk --with eth-account python ...`"
        ) from exc

    for line in sys.stdin:
        if not line.strip():
            continue
        built = build(json.loads(line), Account, MAINNET_API_URL, sign_l1_action)
        print(compact_json(built), flush=True)
    return 0


def build(req: dict[str, Any], Account: Any, MAINNET_API_URL: str, sign_l1_action: Any) -> dict[str, Any]:
    params = dict(req.get("params") or {})
    metadata = dict(params.get("metadata") or {})
    builder_params = dict(params.get("builder_params") or {})
    orders = [order for order in metadata.get("cleanup_orders") or [] if order.get("venue") == "hyperliquid"]
    if not orders:
        return {"metadata": {"cleanup": "skipped", "reason": "no hyperliquid cleanup_orders"}}

    wallet = Account.from_key(env_or_param(builder_params, "secret_key", "HYPERLIQUID_SECRET_KEY"))
    action = {
        "type": "cancelByCloid",
        "cancels": [{"asset": int(order["asset"]), "cloid": str(order["cloid"])} for order in orders],
    }
    nonce = int(builder_params.get("cleanup_nonce") or (time.time_ns() // 1_000_000))
    signature = sign_l1_action(
        wallet,
        action,
        builder_params.get("vault_address"),
        nonce,
        builder_params.get("expires_after"),
        builder_params.get("base_url", MAINNET_API_URL) == MAINNET_API_URL,
    )
    payload = {
        "action": action,
        "nonce": nonce,
        "signature": signature,
        "vaultAddress": builder_params.get("vault_address"),
        "expiresAfter": builder_params.get("expires_after"),
    }
    return {
        "headers": {"Content-Type": "application/json"},
        "body": compact_json(payload),
        "metadata": {"cleanup": "cancelByCloid", "orders": len(orders), "nonce": nonce},
    }


def env_or_param(params: dict[str, Any], key: str, env_key: str) -> str:
    value = params.get(key) or os.getenv(env_key)
    if not value:
        raise SystemExit(f"missing {key}; set params.{key} or {env_key}")
    return str(value)


def compact_json(value: Any) -> str:
    return json.dumps(value, separators=(",", ":"), sort_keys=False)


if __name__ == "__main__":
    raise SystemExit(main())
