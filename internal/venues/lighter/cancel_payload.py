#!/usr/bin/env python3
"""Build signed Lighter cancel-order payloads for benchmark cleanup."""

from __future__ import annotations

import asyncio
import json
import os
import sys
import time
from decimal import Decimal
from typing import Any
from urllib.parse import urlencode

from build_payload import next_nonce, order_index


def main() -> int:
    try:
        import lighter
    except ImportError as exc:
        raise SystemExit("missing Lighter SDK; run with `uv run --with lighter-sdk python ...`") from exc

    asyncio.run(serve(lighter))
    return 0


async def serve(lighter: Any) -> None:
    for line in sys.stdin:
        if not line.strip():
            continue
        built = await build(json.loads(line), lighter)
        print(compact_json(built), flush=True)


async def build(req: dict[str, Any], lighter: Any) -> dict[str, Any]:
    params = dict(req.get("params") or {})
    builder_params = dict(params.get("builder_params") or {})
    api_key_index = int(env_or_param(builder_params, "api_key_index", "LIGHTER_API_KEY_INDEX"))
    account_index = int(env_or_param(builder_params, "account_index", "LIGHTER_ACCOUNT_INDEX"))
    private_key = env_or_param(builder_params, "private_key", "LIGHTER_PRIVATE_KEY")

    client = lighter.SignerClient(
        url=builder_params.get("base_url", "https://mainnet.zklighter.elliot.ai"),
        api_private_keys={api_key_index: private_key},
        account_index=account_index,
    )
    client.account_api = lighter.AccountApi(client.api_client)
    try:
        phase = params.get("phase", "after_sample")
        reconciliation = {}
        if phase == "before_run":
            position = await position_snapshot(client, account_index)
            reconciliation = {"position": position}
            if bool(builder_params.get("cleanup_all_open_orders")):
                orders = await active_market_orders(client, builder_params, api_key_index, account_index)
            else:
                orders = await open_cleanup_orders(client, planned_orders(params, builder_params), builder_params, api_key_index, account_index)
            if not orders:
                return cleanup_result(False, True, "no stale lighter benchmark orders", metadata={"position": position})
        elif phase == "after_run":
            refs = result_orders(dict(params.get("result") or {}))
            remaining = await wait_no_open_cleanup_orders(client, refs, builder_params, api_key_index, account_index)
            if bool(builder_params.get("cleanup_all_open_orders")):
                remaining = await active_market_orders(client, builder_params, api_key_index, account_index)
                if remaining:
                    return await cancel_orders(client, remaining, builder_params, api_key_index, reconciliation)
            before_position = dict(params.get("run_metadata") or {}).get("position")
            after_position = await position_snapshot(client, account_index)
            problems = []
            if remaining:
                problems.append(f"{len(remaining)} lighter benchmark orders still open")
            if before_position is not None and before_position != after_position:
                problems.append("lighter position changed during run")
            metadata = {"position_before": before_position, "position_after": after_position}
            if problems:
                return cleanup_result(True, False, "; ".join(problems), metadata=metadata)
            return cleanup_result(True, True, "no lighter benchmark orders open after run and position unchanged", metadata=metadata)
        else:
            orders = cleanup_orders(dict(params.get("metadata") or {}))
            if not orders:
                return cleanup_result(False, True, "no lighter cleanup_orders")
            remaining = await wait_open_cleanup_orders(client, orders, builder_params, api_key_index, account_index)
            if not remaining:
                neutralize = await neutralize_payload(client, params, builder_params, api_key_index, account_index)
                if neutralize:
                    return neutralize
                return cleanup_result(False, True, "no lighter cleanup action needed")
            orders = remaining

        return await cancel_orders(client, orders, builder_params, api_key_index, reconciliation)
    finally:
        await client.close()

async def cancel_orders(client: Any, orders: list[dict[str, Any]], builder_params: dict[str, Any], api_key_index: int, reconciliation: dict[str, Any]) -> dict[str, Any]:
    tx_types, tx_infos = [], []
    for offset, order in enumerate(orders):
        cancel_api_key_index, nonce = cleanup_nonce(client, builder_params, api_key_index, offset)
        tx_type, tx_info = sign_cancel(client, order, cancel_api_key_index, nonce)
        tx_types.append(tx_type)
        tx_infos.append(tx_info)

    headers = {"Content-Type": "application/x-www-form-urlencoded"}
    if len(tx_types) == 1:
        return {
            "headers": headers,
            "body": urlencode({"tx_type": tx_types[0], "tx_info": tx_infos[0]}),
            "metadata": {"cleanup": "cancel_order", "orders": 1, "reconciliation": reconciliation},
        }
    return {
        "url": builder_params.get("cancel_batch_url", "https://mainnet.zklighter.elliot.ai/api/v1/sendTxBatch"),
        "headers": headers,
        "body": urlencode({"tx_types": json.dumps(tx_types), "tx_infos": json.dumps(tx_infos)}),
        "metadata": {"cleanup": "cancel_order", "orders": len(tx_types), "reconciliation": reconciliation},
    }


async def active_market_orders(client: Any, params: dict[str, Any], api_key_index: int, account_index: int) -> list[dict[str, Any]]:
    market_index = int(params["market_index"])
    auth, error = client.create_auth_token_with_expiry(api_key_index=api_key_index)
    if error:
        raise SystemExit(f"Lighter auth token failed: {error}")
    response = await client.order_api.account_active_orders(account_index=account_index, market_id=market_index, auth=auth)
    return [
        {"venue": "lighter", "market_index": market_index, "order_index": order_index_value(order)}
        for order in orders_list(response)
    ]


def planned_orders(params: dict[str, Any], builder_params: dict[str, Any]) -> list[dict[str, Any]]:
    run = dict(params.get("run") or {})
    run_id = run.get("run_id")
    if not run_id:
        return []
    scenario = run.get("scenario", "single")
    total = int(run.get("iterations") or 0) + int(run.get("warmups") or 0)
    warmups = int(run.get("warmups") or 0)
    batch_size = int(run.get("batch_size") or 1)
    count = batch_size if scenario == "batch" else 1
    order_params = dict(builder_params)
    order_params["run_id"] = run_id
    refs = []
    for index in range(total):
        req = {"iteration": index - warmups}
        for offset in range(count):
            refs.append({
                "venue": "lighter",
                "market_index": int(order_params["market_index"]),
                "order_index": order_index(req, order_params, offset),
            })
    return refs


def result_orders(result: dict[str, Any]) -> list[dict[str, Any]]:
    refs = []
    for sample in result.get("samples") or []:
        refs.extend(cleanup_orders(dict(sample.get("metadata") or {})))
    return refs


def cleanup_orders(metadata: dict[str, Any]) -> list[dict[str, Any]]:
    return [order for order in metadata.get("cleanup_orders") or [] if order.get("venue") == "lighter"]


async def open_cleanup_orders(client: Any, refs: list[dict[str, Any]], params: dict[str, Any], api_key_index: int, account_index: int) -> list[dict[str, Any]]:
    if not refs:
        return []
    auth, error = client.create_auth_token_with_expiry(api_key_index=api_key_index)
    if error:
        raise SystemExit(f"Lighter auth token failed: {error}")
    open_by_market: dict[int, set[int]] = {}
    for market_index in sorted({int(ref["market_index"]) for ref in refs}):
        response = await client.order_api.account_active_orders(account_index=account_index, market_id=market_index, auth=auth)
        open_by_market[market_index] = {order_index_value(order) for order in orders_list(response)}
    return [ref for ref in refs if int(ref["order_index"]) in open_by_market.get(int(ref["market_index"]), set())]


async def wait_open_cleanup_orders(client: Any, refs: list[dict[str, Any]], params: dict[str, Any], api_key_index: int, account_index: int) -> list[dict[str, Any]]:
    attempts = max(1, int(params.get("cleanup_poll_attempts", 5)))
    interval = max(0, int(params.get("cleanup_poll_interval_ms", 250))) / 1000
    for attempt in range(attempts):
        remaining = await open_cleanup_orders(client, refs, params, api_key_index, account_index)
        if remaining or attempt == attempts - 1:
            return remaining
        await asyncio.sleep(interval)
    return []


async def wait_no_open_cleanup_orders(client: Any, refs: list[dict[str, Any]], params: dict[str, Any], api_key_index: int, account_index: int) -> list[dict[str, Any]]:
    attempts = max(1, int(params.get("reconciliation_poll_attempts", 5)))
    interval = max(0, int(params.get("reconciliation_poll_interval_ms", 250))) / 1000
    remaining = []
    for attempt in range(attempts):
        remaining = await open_cleanup_orders(client, refs, params, api_key_index, account_index)
        if not remaining or attempt == attempts - 1:
            return remaining
        await asyncio.sleep(interval)
    return remaining


def orders_list(response: Any) -> list[Any]:
    if hasattr(response, "orders"):
        return response.orders or []
    if isinstance(response, dict):
        return response.get("orders") or []
    return []


def order_index_value(order: Any) -> int:
    if hasattr(order, "client_order_index"):
        return int(order.client_order_index)
    if hasattr(order, "client_order_id"):
        return int(order.client_order_id)
    if hasattr(order, "order_index"):
        return int(order.order_index)
    if hasattr(order, "orderIndex"):
        return int(order.orderIndex)
    if isinstance(order, dict):
        for key in ("client_order_index", "client_order_id", "order_index", "orderIndex", "index"):
            if key in order:
                return int(order[key])
    return -1


async def position_snapshot(client: Any, account_index: int) -> list[dict[str, str]]:
    response = await client.account_api.account(by="index", value=str(account_index))
    accounts = getattr(response, "accounts", None)
    if accounts is None and isinstance(response, dict):
        accounts = response.get("accounts")
    if not accounts:
        return []
    positions = getattr(accounts[0], "positions", None)
    if positions is None and isinstance(accounts[0], dict):
        positions = accounts[0].get("positions")
    return sorted(item for item in (position_item(position) for position in (positions or [])) if Decimal(item["size"]) != 0)


def position_item(position: Any) -> dict[str, str]:
    data = position.to_dict() if hasattr(position, "to_dict") else position
    if not isinstance(data, dict):
        data = {}
    market = first_present(data, "market_index", "market_id", "symbol")
    size = first_present(data, "position", "position_size", "open_order_base_amount", "base_amount", "size")
    return {"market": str(market), "size": str(size)}


async def neutralize_payload(client: Any, params: dict[str, Any], builder_params: dict[str, Any], api_key_index: int, account_index: int) -> dict[str, Any] | None:
    if not bool(builder_params.get("neutralize_on_fill")):
        return None
    before_position = dict(params.get("run_metadata") or {}).get("position")
    if before_position is None:
        return None
    after_position = await position_snapshot(client, account_index)
    market_index = int(builder_params["market_index"])
    delta = position_size(after_position, market_index) - position_size(before_position, market_index)
    if delta == 0:
        return None

    order_api_key_index, nonce = cleanup_nonce(client, builder_params, api_key_index, 0)
    tx_type, tx_info, _tx_hash, error = client.sign_create_order(
        market_index=market_index,
        client_order_index=int(time.time_ns() % 1_900_000_000),
        base_amount=abs(int(delta)),
        price=neutralize_price(builder_params, delta < 0),
        is_ask=delta > 0,
        order_type=int(builder_params.get("neutralize_order_type", client.ORDER_TYPE_MARKET)),
        time_in_force=int(builder_params.get("neutralize_time_in_force", client.ORDER_TIME_IN_FORCE_IMMEDIATE_OR_CANCEL)),
        reduce_only=True,
        order_expiry=int(builder_params.get("neutralize_order_expiry", client.DEFAULT_IOC_EXPIRY)),
        nonce=nonce,
        api_key_index=order_api_key_index,
    )
    if error:
        raise SystemExit(f"Lighter sign neutralize order failed: {error}")
    return {
        "headers": {"Content-Type": "application/x-www-form-urlencoded"},
        "body": urlencode({"tx_type": tx_type, "tx_info": tx_info}),
        "metadata": {
            "cleanup": "neutralize_position",
            "orders": 1,
            "reconciliation": {"position_before": before_position, "position_after": after_position, "delta": str(delta)},
        },
    }


def position_size(positions: list[dict[str, Any]], market_index: int) -> Decimal:
    for position in positions or []:
        if str(position.get("market", "")) == str(market_index):
            return Decimal(str(position.get("size", "0") or "0"))
    return Decimal("0")


def neutralize_price(builder_params: dict[str, Any], is_buy: bool) -> int:
    if builder_params.get("neutralize_price") is not None:
        return int(builder_params["neutralize_price"])
    price = Decimal(str(builder_params["price"]))
    slippage_bps = Decimal(str(builder_params.get("neutralize_slippage_bps", "500")))
    multiplier = Decimal("1") + slippage_bps / Decimal("10000")
    if is_buy:
        return int(price * multiplier)
    return int(price / multiplier)


def first_present(data: dict[str, Any], *keys: str) -> Any:
    for key in keys:
        if key in data and data[key] is not None:
            return data[key]
    return ""


def cleanup_result(attempted: bool, ok: bool, description: str, metadata: dict[str, Any] | None = None) -> dict[str, Any]:
    result = {"attempted": attempted, "ok": ok, "description": description}
    if not ok:
        result["error"] = description
    if metadata:
        result["metadata"] = metadata
    return {"cleanup": result}


def sign_cancel(client: Any, order: dict[str, Any], api_key_index: int, nonce: int) -> tuple[int, str]:
    tx_type, tx_info, _tx_hash, error = client.sign_cancel_order(
        market_index=int(order["market_index"]),
        order_index=int(order["order_index"]),
        nonce=nonce,
        api_key_index=api_key_index,
    )
    if error:
        raise SystemExit(f"Lighter sign_cancel_order failed: {error}")
    return tx_type, tx_info


def cleanup_nonce(client: Any, params: dict[str, Any], api_key_index: int, offset: int) -> tuple[int, int]:
    if params.get("cleanup_nonce_base") is not None:
        return api_key_index, int(params["cleanup_nonce_base"]) + offset
    if params.get("cleanup_nonce") is not None:
        return api_key_index, int(params["cleanup_nonce"])
    state_file = params.get("nonce_state_file") or os.getenv("LIGHTER_NONCE_STATE_FILE")
    if state_file:
        return api_key_index, next_nonce(client, api_key_index, str(state_file))
    return client.get_api_key_nonce(api_key_index, -1)


def env_or_param(params: dict[str, Any], key: str, env_key: str) -> str:
    value = params.get(key) or os.getenv(env_key)
    if value in (None, ""):
        raise SystemExit(f"missing {key}; set params.{key} or {env_key}")
    return str(value)


def compact_json(value: Any) -> str:
    return json.dumps(value, separators=(",", ":"), sort_keys=False)


if __name__ == "__main__":
    raise SystemExit(main())
