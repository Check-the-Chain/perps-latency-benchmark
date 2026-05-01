#!/usr/bin/env python3
"""Build signed Hyperliquid order payloads for perps-bench.

Run through the command builder, for example:

  uv run --with hyperliquid-python-sdk --with eth-account \
    python internal/venues/hyperliquid/build_payload.py

The script reads the perps-bench builder request JSON from stdin and writes a
payload.Built JSON object to stdout. It does not send any network request.
"""

from __future__ import annotations

import json
import os
import sys
import time
import hashlib
from decimal import Decimal
from typing import Any


def main() -> int:
    try:
        from eth_account import Account
        from hyperliquid.info import Info
        from hyperliquid.utils.constants import MAINNET_API_URL
        from hyperliquid.utils.signing import (
            order_request_to_order_wire,
            order_wires_to_order_action,
            sign_l1_action,
        )
        from hyperliquid.utils.types import Cloid
    except ImportError as exc:
        raise SystemExit(
            "missing Hyperliquid SDK dependencies; run with "
            "`uv run --with hyperliquid-python-sdk --with eth-account python ...`"
        ) from exc

    for line in sys.stdin:
        if not line.strip():
            continue
        built = build(
            json.loads(line),
            Account,
            Info,
            MAINNET_API_URL,
            order_request_to_order_wire,
            order_wires_to_order_action,
            sign_l1_action,
            Cloid,
        )
        print(compact_json(built), flush=True)
    return 0


def build(
    req: dict[str, Any],
    Account: Any,
    Info: Any,
    MAINNET_API_URL: str,
    order_request_to_order_wire: Any,
    order_wires_to_order_action: Any,
    sign_l1_action: Any,
    Cloid: Any,
) -> dict[str, Any]:
    params = dict(req.get("params") or {})
    wallet = Account.from_key(env_or_param(params, "secret_key", "HYPERLIQUID_SECRET_KEY"))

    scenario = req["scenario"]
    batch_size = int(req.get("batch_size") or 1)
    orders = params.get("orders")
    if orders is None:
        price = dynamic_limit_price(params, Info, MAINNET_API_URL)
        orders = [order_from_params(params, req, offset, Cloid, price) for offset in range(batch_size if scenario == "batch" else 1)]
    if scenario != "batch":
        orders = orders[:1]

    order_wires = []
    cleanup_orders = []
    for order in orders:
        cleanup_orders.append(cleanup_ref(order))
        asset = int(order.pop("asset"))
        order_wires.append(order_request_to_order_wire(order, asset))

    nonce = int(params.get("nonce_base") or (time.time_ns() // 1_000_000)) + int(req["iteration"])
    builder = params.get("builder")
    grouping = params.get("grouping", "na")
    action = order_wires_to_order_action(order_wires, builder, grouping)
    signature = sign_l1_action(
        wallet,
        action,
        params.get("vault_address"),
        nonce,
        params.get("expires_after"),
        params.get("base_url", MAINNET_API_URL) == MAINNET_API_URL,
    )
    payload = {
        "action": action,
        "nonce": nonce,
        "signature": signature,
        "vaultAddress": params.get("vault_address"),
        "expiresAfter": params.get("expires_after"),
    }

    built: dict[str, Any] = {
        "headers": {"Content-Type": "application/json"},
        "body": compact_json(payload),
        "metadata": {
            "builder": "hyperliquid-python-sdk",
            "nonce": nonce,
            "run_id": params.get("run_id"),
            "order_type": benchmark_order_type(params),
            "time_in_force": order_tif(params),
            "cleanup_orders": cleanup_orders,
        },
    }
    if req.get("transport") == "websocket":
        built["ws_body"] = compact_json(
            {
                "method": "post",
                "id": params.get("request_id", nonce),
                "request": {"type": "action", "payload": payload},
            }
        )
    return built


def order_from_params(params: dict[str, Any], req: dict[str, Any], offset: int, Cloid: Any, price: Decimal | None = None) -> dict[str, Any]:
    asset = params.get("asset")
    if asset is None:
        raise SystemExit("Hyperliquid builder requires params.asset for symbol-to-asset mapping")
    cloid = order_cloid(params, req, offset, Cloid)
    is_buy = str(params.get("side", "buy")).lower() == "buy"
    order: dict[str, Any] = {
        "coin": params.get("symbol", "BTC"),
        "is_buy": is_buy,
        "sz": float(params["size"]),
        "limit_px": float(price or Decimal(str(params["price"]))),
        "order_type": {"limit": {"tif": order_tif(params)}},
        "reduce_only": bool(params.get("reduce_only", False)),
        "asset": asset,
    }
    if cloid:
        order["cloid"] = cloid
    return order


def dynamic_limit_price(params: dict[str, Any], Info: Any, MAINNET_API_URL: str) -> Decimal | None:
    if not bool(params.get("price_from_mid")) and params.get("price_offset_bps") is None:
        return None
    symbol = str(params.get("symbol", "BTC"))
    info = Info(params.get("base_url", MAINNET_API_URL), skip_ws=True)
    mid = Decimal(str(info.all_mids()[symbol]))
    offset = Decimal(str(params.get("price_offset_bps", "0"))) / Decimal("10000")
    if str(params.get("side", "buy")).lower() == "buy":
        return mid * (Decimal("1") + offset)
    return mid * (Decimal("1") - offset)


def order_tif(params: dict[str, Any]) -> str:
    if str(params.get("order_type", "")).lower() == "market":
        return params.get("tif", "Ioc")
    return params.get("tif", "Alo")


def benchmark_order_type(params: dict[str, Any]) -> str:
    order_type = str(params.get("order_type", "")).lower()
    tif = str(order_tif(params)).lower()
    if order_type == "market":
        return "market"
    if tif == "alo":
        return "post_only"
    if tif == "ioc":
        return "ioc"
    return tif or order_type or "limit"


def order_cloid(params: dict[str, Any], req: dict[str, Any], offset: int, Cloid: Any) -> Any:
    if params.get("cloid_base") is not None:
        return Cloid.from_int((int(str(params["cloid_base"]), 0) + offset) & ((1 << 128) - 1))
    if params.get("cloid") is not None:
        raw = str(params["cloid"])
        if offset:
            return Cloid.from_int((int(raw, 16) + offset) & ((1 << 128) - 1))
        return Cloid.from_str(raw)
    run_id = params.get("run_id")
    if run_id:
        seed = f"{run_id}:{req.get('iteration', 0)}:{params.get('symbol', 'BTC')}:{params.get('side', 'buy')}:{offset}".encode()
        return Cloid.from_int(int.from_bytes(hashlib.blake2b(seed, digest_size=16).digest(), "big"))
    return Cloid.from_int(((time.time_ns() << 16) + offset) & ((1 << 128) - 1))


def cleanup_ref(order: dict[str, Any]) -> dict[str, Any]:
    cloid = order.get("cloid")
    return {
        "venue": "hyperliquid",
        "asset": int(order["asset"]),
        "symbol": order["coin"],
        "cloid": cloid.to_raw() if hasattr(cloid, "to_raw") else str(cloid),
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
