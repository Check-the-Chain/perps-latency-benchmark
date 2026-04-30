#!/usr/bin/env python3
"""Build signed Lighter cancel-order payloads for benchmark cleanup."""

from __future__ import annotations

import asyncio
import json
import os
import sys
from typing import Any
from urllib.parse import urlencode

from build_payload import order_index


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
            orders = await open_cleanup_orders(client, planned_orders(params, builder_params), builder_params, api_key_index, account_index)
            if not orders:
                return cleanup_result(False, True, "no stale lighter benchmark orders", metadata={"position": position})
        elif phase == "after_run":
            remaining = await open_cleanup_orders(client, result_orders(dict(params.get("result") or {})), builder_params, api_key_index, account_index)
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

        tx_types, tx_infos = [], []
        for offset, order in enumerate(orders):
            cancel_api_key_index, nonce = cleanup_nonce(client, builder_params, api_key_index, offset)
            tx_type, tx_info = sign_cancel(client, order, cancel_api_key_index, nonce)
            tx_types.append(tx_type)
            tx_infos.append(tx_info)
    finally:
        await client.close()

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
    return sorted(position_item(position) for position in (positions or []))


def position_item(position: Any) -> dict[str, str]:
    data = position.to_dict() if hasattr(position, "to_dict") else position
    if not isinstance(data, dict):
        data = {}
    market = data.get("market_index") or data.get("market_id") or data.get("symbol") or ""
    size = data.get("position") or data.get("position_size") or data.get("open_order_base_amount") or data.get("base_amount") or data.get("size") or ""
    return {"market": str(market), "size": str(size)}


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
