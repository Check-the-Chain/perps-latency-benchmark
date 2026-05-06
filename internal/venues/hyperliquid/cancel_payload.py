#!/usr/bin/env python3
"""Build signed Hyperliquid cancelByCloid payloads for benchmark cleanup."""

from __future__ import annotations

import json
import os
import sys
import time
import urllib.request
from decimal import Decimal
from pathlib import Path
from typing import Any

sys.path.append(str(Path(__file__).resolve().parents[1]))
from cleanup_common import cleanup_orders_for_venue, cleanup_result, result_orders_for_venue
from build_payload import cleanup_ref, order_cloid, valid_price


def main() -> int:
    try:
        from eth_account import Account
        from hyperliquid.utils.constants import MAINNET_API_URL
        from hyperliquid.info import Info
        from hyperliquid.utils.signing import order_request_to_order_wire, order_wires_to_order_action, sign_l1_action
        from hyperliquid.utils.types import Cloid
    except ImportError as exc:
        raise SystemExit(
            "missing Hyperliquid SDK dependencies; run with "
            "`uv run --with hyperliquid-python-sdk --with eth-account python ...`"
        ) from exc

    for line in sys.stdin:
        if not line.strip():
            continue
        built = build(json.loads(line), Account, Info, MAINNET_API_URL, order_request_to_order_wire, order_wires_to_order_action, sign_l1_action, Cloid)
        print(compact_json(built), flush=True)
    return 0


def build(req: dict[str, Any], Account: Any, Info: Any, MAINNET_API_URL: str, order_request_to_order_wire: Any, order_wires_to_order_action: Any, sign_l1_action: Any, Cloid: Any) -> dict[str, Any]:
    params = dict(req.get("params") or {})
    builder_params = dict(params.get("builder_params") or {})
    phase = params.get("phase", "after_sample")
    if phase == "before_run":
        return before_run(params, builder_params, Account, Info, MAINNET_API_URL, sign_l1_action, Cloid)
    if phase == "after_run":
        return after_run(params, builder_params, Account, Info, MAINNET_API_URL)

    orders = cleanup_orders(dict(params.get("metadata") or {}))
    if not orders:
        return skipped("no hyperliquid cleanup_orders")
    remaining = open_cleanup_orders(orders, builder_params, Account, Info, MAINNET_API_URL)
    if remaining:
        return cancel_payload(remaining, builder_params, Account, MAINNET_API_URL, sign_l1_action)
    neutralize = neutralize_payload(params, builder_params, Account, Info, MAINNET_API_URL, order_request_to_order_wire, order_wires_to_order_action, sign_l1_action, Cloid)
    if neutralize:
        return neutralize
    return skipped("no hyperliquid cleanup action needed")


def before_run(params: dict[str, Any], builder_params: dict[str, Any], Account: Any, Info: Any, MAINNET_API_URL: str, sign_l1_action: Any, Cloid: Any) -> dict[str, Any]:
    position = position_snapshot(builder_params, Account, Info, MAINNET_API_URL)
    planned = planned_orders(params, builder_params, Cloid)
    open_orders = open_cleanup_orders(planned, builder_params, Account, Info, MAINNET_API_URL)
    if not open_orders:
        return cleanup_result(False, True, "no stale hyperliquid benchmark orders", metadata={"position": position})
    return cancel_payload(open_orders, builder_params, Account, MAINNET_API_URL, sign_l1_action, metadata={"position": position})


def after_run(params: dict[str, Any], builder_params: dict[str, Any], Account: Any, Info: Any, MAINNET_API_URL: str) -> dict[str, Any]:
    refs = result_orders(dict(params.get("result") or {}))
    remaining = open_cleanup_orders(refs, builder_params, Account, Info, MAINNET_API_URL)
    before_position = dict(params.get("run_metadata") or {}).get("position")
    after_position = position_snapshot(builder_params, Account, Info, MAINNET_API_URL)
    problems = []
    if remaining:
        problems.append(f"{len(remaining)} hyperliquid benchmark orders still open")
    if before_position is not None and before_position != after_position:
        problems.append("hyperliquid position changed during run")
    metadata = {"position_before": before_position, "position_after": after_position}
    if problems:
        return cleanup_result(True, False, "; ".join(problems), metadata=metadata)
    return cleanup_result(True, True, "no hyperliquid benchmark orders open after run and position unchanged", metadata=metadata)


def cancel_payload(orders: list[dict[str, Any]], builder_params: dict[str, Any], Account: Any, MAINNET_API_URL: str, sign_l1_action: Any, metadata: dict[str, Any] | None = None) -> dict[str, Any]:
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
        "metadata": {"cleanup": "cancelByCloid", "orders": len(orders), "nonce": nonce, "reconciliation": metadata or {}},
    }


def neutralize_payload(params: dict[str, Any], builder_params: dict[str, Any], Account: Any, Info: Any, MAINNET_API_URL: str, order_request_to_order_wire: Any, order_wires_to_order_action: Any, sign_l1_action: Any, Cloid: Any) -> dict[str, Any] | None:
    if not bool(builder_params.get("neutralize_on_fill")):
        return None
    before_position = dict(params.get("run_metadata") or {}).get("position")
    if before_position is None:
        return None
    after_position = position_snapshot(builder_params, Account, Info, MAINNET_API_URL)
    symbol = str(builder_params.get("symbol", "BTC"))
    delta = position_size(after_position, "coin", symbol, "szi") - position_size(before_position, "coin", symbol, "szi")
    if delta == 0:
        return None

    wallet = Account.from_key(env_or_param(builder_params, "secret_key", "HYPERLIQUID_SECRET_KEY"))
    pause_for_cumulative_request_debt(builder_params, wallet.address, MAINNET_API_URL)
    is_buy = delta < 0
    size = abs(delta)
    asset = int(builder_params["asset"])
    cloid = Cloid.from_int(time.time_ns() & ((1 << 128) - 1))
    order = {
        "coin": symbol,
        "is_buy": is_buy,
        "sz": float(size),
        "limit_px": float(neutralize_price(builder_params, is_buy)),
        "order_type": {"limit": {"tif": builder_params.get("neutralize_tif", "Ioc")}},
        "reduce_only": True,
        "cloid": cloid,
    }
    action = order_wires_to_order_action(
        [order_request_to_order_wire(order, asset)],
        builder_params.get("builder"),
        builder_params.get("grouping", "na"),
    )
    nonce = int(time.time_ns() // 1_000_000)
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
        "metadata": {
            "cleanup": "neutralize_position",
            "orders": 1,
            "nonce": nonce,
            "cleanup_orders": [{"venue": "hyperliquid", "asset": asset, "symbol": symbol, "cloid": cloid.to_raw()}],
            "reconciliation": {"position_before": before_position, "position_after": after_position, "delta": str(delta)},
        },
    }


def planned_orders(params: dict[str, Any], builder_params: dict[str, Any], Cloid: Any) -> list[dict[str, Any]]:
    run = dict(params.get("run") or {})
    run_id = run.get("run_id")
    if not run_id:
        return []
    scenario = run.get("scenario", "single")
    total = int(run.get("iterations") or 0) + int(run.get("warmups") or 0)
    warmups = int(run.get("warmups") or 0)
    batch_size = int(run.get("batch_size") or 1)
    count = batch_size if scenario == "batch" else 1
    refs = []
    order_params = dict(builder_params)
    order_params["run_id"] = run_id
    for index in range(total):
        req = {"iteration": index - warmups}
        for offset in range(count):
            order = {
                "asset": int(order_params["asset"]),
                "coin": order_params.get("symbol", "BTC"),
                "cloid": order_cloid(order_params, req, offset, Cloid),
            }
            refs.append(cleanup_ref(order))
    return refs


def result_orders(result: dict[str, Any]) -> list[dict[str, Any]]:
    return result_orders_for_venue(result, "hyperliquid")


def cleanup_orders(metadata: dict[str, Any]) -> list[dict[str, Any]]:
    return cleanup_orders_for_venue(metadata, "hyperliquid")


def open_cleanup_orders(refs: list[dict[str, Any]], builder_params: dict[str, Any], Account: Any, Info: Any, MAINNET_API_URL: str) -> list[dict[str, Any]]:
    if not refs:
        return []
    wallet = Account.from_key(env_or_param(builder_params, "secret_key", "HYPERLIQUID_SECRET_KEY"))
    info = Info(builder_params.get("base_url", MAINNET_API_URL), skip_ws=True)
    open_cloids = {str(order.get("cloid")) for order in info.open_orders(wallet.address) if order.get("cloid")}
    return [ref for ref in refs if str(ref.get("cloid")) in open_cloids]


def position_snapshot(builder_params: dict[str, Any], Account: Any, Info: Any, MAINNET_API_URL: str) -> list[dict[str, str]]:
    wallet = Account.from_key(env_or_param(builder_params, "secret_key", "HYPERLIQUID_SECRET_KEY"))
    info = Info(builder_params.get("base_url", MAINNET_API_URL), skip_ws=True)
    positions = []
    for wrapped in info.user_state(wallet.address).get("assetPositions") or []:
        pos = wrapped.get("position") or {}
        size = str(pos.get("szi", "0"))
        if Decimal(size) == 0:
            continue
        positions.append({
            "coin": str(pos.get("coin", "")),
            "szi": size,
        })
    return sorted(positions, key=lambda item: item["coin"])


def position_size(positions: list[dict[str, Any]], key: str, value: str, size_key: str) -> Decimal:
    for position in positions or []:
        if str(position.get(key, "")) == value:
            return Decimal(str(position.get(size_key, "0") or "0"))
    return Decimal("0")


def pause_for_cumulative_request_debt(builder_params: dict[str, Any], user: str, MAINNET_API_URL: str) -> None:
    pause_ms = int(builder_params.get("debt_neutralize_pause_ms") or 0)
    if pause_ms <= 0:
        return
    base_url = builder_params.get("base_url", MAINNET_API_URL).rstrip("/")
    try:
        status = post_json(f"{base_url}/info", {"type": "userRateLimit", "user": user})
        used = int(status.get("nRequestsUsed") or 0)
        cap = int(status.get("nRequestsCap") or 0)
        surplus = int(status.get("nRequestsSurplus") or 0)
    except Exception:
        return
    if cap + surplus - used < 1:
        time.sleep(pause_ms / 1000)


def post_json(url: str, payload: dict[str, Any]) -> dict[str, Any]:
    request = urllib.request.Request(
        url,
        data=compact_json(payload).encode(),
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(request, timeout=10) as response:
        return json.load(response)


def neutralize_price(builder_params: dict[str, Any], is_buy: bool) -> Decimal:
    if builder_params.get("neutralize_price") is not None:
        return Decimal(str(builder_params["neutralize_price"]))
    price = Decimal(str(builder_params["price"]))
    slippage_bps = Decimal(str(builder_params.get("neutralize_slippage_bps", "500")))
    multiplier = Decimal("1") + slippage_bps / Decimal("10000")
    if is_buy:
        return valid_price(price * multiplier, True)
    return valid_price(price / multiplier, False)


def skipped(reason: str) -> dict[str, Any]:
    return cleanup_result(False, True, reason)


def env_or_param(params: dict[str, Any], key: str, env_key: str) -> str:
    value = params.get(key) or os.getenv(env_key)
    if not value:
        raise SystemExit(f"missing {key}; set params.{key} or {env_key}")
    return str(value)


def compact_json(value: Any) -> str:
    return json.dumps(value, separators=(",", ":"), sort_keys=False)


if __name__ == "__main__":
    raise SystemExit(main())
