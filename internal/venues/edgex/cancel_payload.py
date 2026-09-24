#!/usr/bin/env python3
"""Cleanup edgeX benchmark orders outside the measured latency window."""

from __future__ import annotations

import asyncio
import json
import os
import sys
import time
from decimal import Decimal
from pathlib import Path
from typing import Any

sys.path.append(str(Path(__file__).resolve().parents[1]))
from cleanup_common import cleanup_orders_for_venue, cleanup_result, result_orders_for_venue
from build_payload import client_order_id, env_or_param, strip_0x


def main() -> int:
    try:
        from edgex_sdk import CancelOrderParams, Client, CreateOrderParams, GetActiveOrderParams, OrderSide, OrderType, StarkExSigningAdapter, TimeInForce
    except ImportError as exc:
        raise SystemExit("missing edgeX SDK; run with `uv run --with edgex-python-sdk --with requests python ...`") from exc

    asyncio.run(serve(CancelOrderParams, Client, CreateOrderParams, GetActiveOrderParams, OrderSide, OrderType, StarkExSigningAdapter, TimeInForce))
    return 0


async def serve(
    CancelOrderParams: Any,
    Client: Any,
    CreateOrderParams: Any,
    GetActiveOrderParams: Any,
    OrderSide: Any,
    OrderType: Any,
    StarkExSigningAdapter: Any,
    TimeInForce: Any,
) -> None:
    for line in sys.stdin:
        if not line.strip():
            continue
        built = await build(json.loads(line), CancelOrderParams, Client, CreateOrderParams, GetActiveOrderParams, OrderSide, OrderType, StarkExSigningAdapter, TimeInForce)
        print(compact_json(built), flush=True)


async def build(
    req: dict[str, Any],
    CancelOrderParams: Any,
    Client: Any,
    CreateOrderParams: Any,
    GetActiveOrderParams: Any,
    OrderSide: Any,
    OrderType: Any,
    StarkExSigningAdapter: Any,
    TimeInForce: Any,
) -> dict[str, Any]:
    params = dict(req.get("params") or {})
    builder_params = dict(params.get("builder_params") or {})
    client = sdk_client(builder_params, Client, StarkExSigningAdapter)
    try:
        phase = params.get("phase", "after_sample")
        if phase == "before_run":
            return await before_run(client, params, builder_params, CancelOrderParams, GetActiveOrderParams)
        if phase == "after_run":
            return await after_run(client, params, builder_params, GetActiveOrderParams)
        return await after_sample(client, params, builder_params, CancelOrderParams, GetActiveOrderParams, CreateOrderParams, OrderSide, OrderType, TimeInForce)
    finally:
        await client.close()


def sdk_client(params: dict[str, Any], Client: Any, StarkExSigningAdapter: Any) -> Any:
    return Client(
        base_url=str(params.get("base_url", "https://pro.edgex.exchange")),
        account_id=int(env_or_param(params, "account_id", "EDGEX_ACCOUNT_ID")),
        stark_private_key=strip_0x(env_or_param(params, "stark_private_key", "EDGEX_STARK_PRIVATE_KEY")),
        signing_adapter=StarkExSigningAdapter(),
        timeout=float(params.get("cleanup_request_timeout_seconds", 15)),
    )


async def before_run(client: Any, params: dict[str, Any], builder_params: dict[str, Any], CancelOrderParams: Any, GetActiveOrderParams: Any) -> dict[str, Any]:
    position = await position_snapshot(client, builder_params)
    if bool(builder_params.get("cleanup_all_open_orders")):
        orders = await active_market_orders(client, builder_params, GetActiveOrderParams)
    else:
        orders = await open_cleanup_orders(client, planned_orders(params, builder_params), builder_params, GetActiveOrderParams)
    if not orders:
        return cleanup_result(False, True, "no stale edgeX benchmark orders", metadata={"position": position})
    return await cancel_orders(client, orders, CancelOrderParams, {"position": position})


async def after_run(client: Any, params: dict[str, Any], builder_params: dict[str, Any], GetActiveOrderParams: Any) -> dict[str, Any]:
    refs = result_orders(dict(params.get("result") or {}))
    remaining = await wait_no_open_cleanup_orders(client, refs, builder_params, GetActiveOrderParams)
    before_position = dict(params.get("run_metadata") or {}).get("position")
    after_position = await position_snapshot(client, builder_params)
    problems = []
    if remaining:
        problems.append(f"{len(remaining)} edgeX benchmark orders still open")
    if before_position is not None and before_position != after_position:
        problems.append("edgeX position changed during run")
    metadata = {"position_before": before_position, "position_after": after_position}
    if problems:
        return cleanup_result(True, False, "; ".join(problems), metadata=metadata)
    return cleanup_result(True, True, "no edgeX benchmark orders open after run and position unchanged", metadata=metadata)


async def after_sample(
    client: Any,
    params: dict[str, Any],
    builder_params: dict[str, Any],
    CancelOrderParams: Any,
    GetActiveOrderParams: Any,
    CreateOrderParams: Any,
    OrderSide: Any,
    OrderType: Any,
    TimeInForce: Any,
) -> dict[str, Any]:
    orders = cleanup_orders(dict(params.get("metadata") or {}))
    if not orders:
        return cleanup_result(False, True, "no edgeX cleanup_orders")
    remaining = await wait_open_cleanup_orders(client, orders, builder_params, GetActiveOrderParams)
    if remaining:
        return await cancel_orders(client, remaining, CancelOrderParams, {})
    neutralize = await neutralize_position(client, params, builder_params, CreateOrderParams, OrderSide, OrderType, TimeInForce)
    if neutralize:
        return neutralize
    return cleanup_result(False, True, "no edgeX cleanup action needed")


async def cancel_orders(client: Any, orders: list[dict[str, Any]], CancelOrderParams: Any, reconciliation: dict[str, Any]) -> dict[str, Any]:
    if not orders:
        return cleanup_result(False, True, "no edgeX orders to cancel", metadata=reconciliation)
    failed = []
    for order in orders:
        client_id = str(order.get("client_order_id") or "")
        if not client_id:
            continue
        response = await client.cancel_order(CancelOrderParams(client_id=client_id))
        if not response_ok(response):
            failed.append(f"{client_id}: {response_reason(response)}")
    if failed:
        return cleanup_result(True, False, "; ".join(failed), metadata=reconciliation)
    return cleanup_result(True, True, "cancel edgeX benchmark orders by client order ID", metadata=reconciliation)


async def active_market_orders(client: Any, params: dict[str, Any], GetActiveOrderParams: Any) -> list[dict[str, Any]]:
    contract_id = str(params["contract_id"])
    response = await client.get_active_orders(GetActiveOrderParams(filter_contract_id_list=[contract_id], size=str(params.get("active_order_page_size", "100"))))
    require_ok(response)
    return [
        {"venue": "edgex", "contract_id": contract_id, "client_order_id": str(first_present(order, "clientOrderId", "client_order_id", "clientId"))}
        for order in order_items(response)
        if first_present(order, "clientOrderId", "client_order_id", "clientId") not in (None, "")
    ]


async def open_cleanup_orders(client: Any, refs: list[dict[str, Any]], params: dict[str, Any], GetActiveOrderParams: Any) -> list[dict[str, Any]]:
    if not refs:
        return []
    open_ids: set[str] = set()
    for contract_id in sorted({str(ref.get("contract_id") or params["contract_id"]) for ref in refs}):
        response = await client.get_active_orders(GetActiveOrderParams(filter_contract_id_list=[contract_id], size=str(params.get("active_order_page_size", "100"))))
        require_ok(response)
        open_ids.update(
            str(first_present(order, "clientOrderId", "client_order_id", "clientId"))
            for order in order_items(response)
            if first_present(order, "clientOrderId", "client_order_id", "clientId") not in (None, "")
        )
    return [ref for ref in refs if str(ref.get("client_order_id")) in open_ids]


async def wait_open_cleanup_orders(client: Any, refs: list[dict[str, Any]], params: dict[str, Any], GetActiveOrderParams: Any) -> list[dict[str, Any]]:
    attempts = max(1, int(params.get("cleanup_poll_attempts", 5)))
    interval = max(0, int(params.get("cleanup_poll_interval_ms", 250))) / 1000
    for attempt in range(attempts):
        remaining = await open_cleanup_orders(client, refs, params, GetActiveOrderParams)
        if remaining or attempt == attempts - 1:
            return remaining
        await asyncio.sleep(interval)
    return []


async def wait_no_open_cleanup_orders(client: Any, refs: list[dict[str, Any]], params: dict[str, Any], GetActiveOrderParams: Any) -> list[dict[str, Any]]:
    attempts = max(1, int(params.get("reconciliation_poll_attempts", 5)))
    interval = max(0, int(params.get("reconciliation_poll_interval_ms", 250))) / 1000
    remaining = []
    for attempt in range(attempts):
        remaining = await open_cleanup_orders(client, refs, params, GetActiveOrderParams)
        if not remaining or attempt == attempts - 1:
            return remaining
        await asyncio.sleep(interval)
    return remaining


def planned_orders(params: dict[str, Any], builder_params: dict[str, Any]) -> list[dict[str, Any]]:
    run = dict(params.get("run") or {})
    run_id = run.get("run_id")
    if not run_id:
        return []
    total = int(run.get("iterations") or 0) + int(run.get("warmups") or 0)
    warmups = int(run.get("warmups") or 0)
    order_params = dict(builder_params)
    order_params["run_id"] = run_id
    refs = []
    for index in range(total):
        req = {"iteration": index - warmups}
        refs.append({
            "venue": "edgex",
            "contract_id": str(order_params["contract_id"]),
            "client_order_id": client_order_id(order_params, req, 0),
        })
    return refs


def result_orders(result: dict[str, Any]) -> list[dict[str, Any]]:
    return result_orders_for_venue(result, "edgex")


def cleanup_orders(metadata: dict[str, Any]) -> list[dict[str, Any]]:
    return cleanup_orders_for_venue(metadata, "edgex")


async def position_snapshot(client: Any, params: dict[str, Any]) -> list[dict[str, str]]:
    response = await client.get_account_positions()
    require_ok(response)
    positions = []
    for position in position_items(response):
        size = Decimal(str(first_present(position, "size", "positionSize", "openSize", "availableSize") or "0"))
        if size == 0:
            continue
        positions.append({
            "contract_id": str(first_present(position, "contractId", "contract_id") or ""),
            "side": str(first_present(position, "side", "positionSide") or ""),
            "size": str(size),
        })
    return sorted(positions, key=lambda item: (item["contract_id"], item["side"]))


async def neutralize_position(client: Any, params: dict[str, Any], builder_params: dict[str, Any], CreateOrderParams: Any, OrderSide: Any, OrderType: Any, TimeInForce: Any) -> dict[str, Any] | None:
    if not bool(builder_params.get("neutralize_on_fill")):
        return None
    before_position = dict(params.get("run_metadata") or {}).get("position")
    if before_position is None:
        return None
    after_position = await position_snapshot(client, builder_params)
    contract_id = str(builder_params["contract_id"])
    delta = signed_position_size(after_position, contract_id) - signed_position_size(before_position, contract_id)
    if delta == 0:
        return None

    is_buy = delta < 0
    order = CreateOrderParams(
        contract_id=contract_id,
        price=str(neutralize_price(builder_params, is_buy)),
        size=str(abs(delta)),
        type=OrderType.MARKET,
        side=OrderSide.BUY.value if is_buy else OrderSide.SELL.value,
        client_order_id=str(int(time.time_ns())),
        time_in_force=TimeInForce.IMMEDIATE_OR_CANCEL.value,
        reduce_only=True,
    )
    response = await client.create_order(order)
    ok = response_ok(response)
    return cleanup_result(
        True,
        ok,
        response_reason(response) if not ok else "neutralize edgeX position",
        metadata={"position_before": before_position, "position_after": after_position, "delta": str(delta)},
    )


def order_items(response: dict[str, Any]) -> list[Any]:
    return nested_items(response, "orderList", "orders", "dataList", "list", "items")


def position_items(response: dict[str, Any]) -> list[Any]:
    return nested_items(response, "positionAssetList", "positions", "positionList", "dataList", "list", "items")


def nested_items(value: Any, *keys: str) -> list[Any]:
    if isinstance(value, list):
        return value
    if not isinstance(value, dict):
        return []
    for key in keys:
        child = value.get(key)
        if isinstance(child, list):
            return child
    for child in value.values():
        found = nested_items(child, *keys)
        if found:
            return found
    return []


def signed_position_size(positions: list[dict[str, Any]], contract_id: str) -> Decimal:
    total = Decimal("0")
    for position in positions or []:
        if str(position.get("contract_id", "")) != contract_id:
            continue
        size = Decimal(str(position.get("size", "0") or "0"))
        side = str(position.get("side", "")).upper()
        total += -size if side == "SHORT" or side == "SELL" else size
    return total


def neutralize_price(builder_params: dict[str, Any], is_buy: bool) -> Decimal:
    if builder_params.get("neutralize_price") is not None:
        return Decimal(str(builder_params["neutralize_price"]))
    price = Decimal(str(builder_params["price"]))
    slippage_bps = Decimal(str(builder_params.get("neutralize_slippage_bps", "500")))
    multiplier = Decimal("1") + slippage_bps / Decimal("10000")
    if is_buy:
        return price * multiplier
    return price / multiplier


def require_ok(response: dict[str, Any]) -> None:
    if not response_ok(response):
        raise SystemExit(response_reason(response))


def response_ok(response: dict[str, Any]) -> bool:
    code = str(response.get("code") or response.get("status") or "").upper()
    if code == "SUCCESS" or code == "OK":
        return True
    if response.get("success") is True:
        return True
    return False


def response_reason(response: dict[str, Any]) -> str:
    for key in ("msg", "message", "error", "reason", "code", "status"):
        value = response.get(key)
        if value not in (None, ""):
            return str(value)
    return "edgeX API response was not successful"


def first_present(data: Any, *keys: str) -> Any:
    if not isinstance(data, dict):
        return None
    for key in keys:
        if data.get(key) not in (None, ""):
            return data[key]
    return None


def compact_json(value: Any) -> str:
    return json.dumps(value, separators=(",", ":"), sort_keys=False)


if __name__ == "__main__":
    raise SystemExit(main())
