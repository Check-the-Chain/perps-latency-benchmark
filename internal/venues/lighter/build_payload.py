#!/usr/bin/env python3
"""Build signed Lighter sendTx/sendTxBatch payloads for perps-bench.

Run through the command builder, for example:

  uv run --with lighter-sdk python internal/venues/lighter/build_payload.py

The script signs transactions through the official Python SDK and emits HTTP
form bodies plus WebSocket JSON messages. It does not submit the transaction.
"""

from __future__ import annotations

import json
import os
import sys
import time
import asyncio
import hashlib
import fcntl
from typing import Any
from urllib.parse import urlencode


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
    api_key_index = int(env_or_param(params, "api_key_index", "LIGHTER_API_KEY_INDEX"))
    account_index = int(env_or_param(params, "account_index", "LIGHTER_ACCOUNT_INDEX"))
    private_key = env_or_param(params, "private_key", "LIGHTER_PRIVATE_KEY")

    client = lighter.SignerClient(
        url=params.get("base_url", "https://mainnet.zklighter.elliot.ai"),
        api_private_keys={api_key_index: private_key},
        account_index=account_index,
    )
    try:
        scenario = req["scenario"]
        batch_size = int(req.get("batch_size") or 1)
        if scenario == "batch":
            tx_types, tx_infos = [], []
            cleanup_orders = []
            for offset in range(batch_size):
                order_api_key_index, nonce = order_nonce(client, params, api_key_index, offset)
                tx_type, tx_info, cleanup_ref = sign_order(client, req, params, order_api_key_index, nonce, offset)
                tx_types.append(tx_type)
                tx_infos.append(tx_info)
                cleanup_orders.append(cleanup_ref)
            body = urlencode({"tx_types": json.dumps(tx_types), "tx_infos": json.dumps(tx_infos)})
            ws_body = compact_json({"type": "jsonapi/sendtxbatch", "data": {"tx_types": json.dumps(tx_types), "tx_infos": json.dumps(tx_infos)}})
            metadata = {"builder": "lighter-python-sdk", "orders": batch_size, "run_id": params.get("run_id"), "order_type": benchmark_order_type(client, params), "cleanup_orders": cleanup_orders}
        else:
            order_api_key_index, nonce = order_nonce(client, params, api_key_index, 0)
            tx_type, tx_info, cleanup_ref = sign_order(client, req, params, order_api_key_index, nonce, 0)
            body = urlencode({"tx_type": tx_type, "tx_info": tx_info})
            ws_body = compact_json({"type": "jsonapi/sendtx", "data": {"tx_type": tx_type, "tx_info": json.loads(tx_info)}})
            metadata = {"builder": "lighter-python-sdk", "orders": 1, "run_id": params.get("run_id"), "order_type": benchmark_order_type(client, params), "cleanup_orders": [cleanup_ref]}
    finally:
        await client.close()

    return {
        "headers": {"Content-Type": "application/x-www-form-urlencoded"},
        "body": body,
        "ws_body": ws_body,
        "metadata": metadata,
    }


def sign_order(client: Any, req: dict[str, Any], params: dict[str, Any], api_key_index: int, nonce: int, offset: int) -> tuple[int, str, dict[str, Any]]:
    client_order_index = order_index(req, params, offset)
    market_index = int(params["market_index"])
    tx_type, tx_info, _tx_hash, error = client.sign_create_order(
        market_index=market_index,
        client_order_index=client_order_index,
        base_amount=int(params["base_amount"]),
        price=int(params["price"]),
        is_ask=str(params.get("side", "buy")).lower() == "sell",
        order_type=int(params.get("order_type", client.ORDER_TYPE_LIMIT)),
        time_in_force=int(params.get("time_in_force", client.ORDER_TIME_IN_FORCE_POST_ONLY)),
        reduce_only=bool(params.get("reduce_only", False)),
        order_expiry=order_expiry(client, params),
        nonce=nonce,
        api_key_index=api_key_index,
    )
    if error:
        raise SystemExit(f"Lighter sign_create_order failed: {error}")
    return tx_type, tx_info, {
        "venue": "lighter",
        "market_index": market_index,
        "order_index": client_order_index,
    }


def order_expiry(client: Any, params: dict[str, Any]) -> int:
    if params.get("order_expiry") is not None:
        return int(params["order_expiry"])
    order_type = int(params.get("order_type", client.ORDER_TYPE_LIMIT))
    time_in_force = int(params.get("time_in_force", client.ORDER_TIME_IN_FORCE_POST_ONLY))
    if order_type == client.ORDER_TYPE_MARKET or time_in_force == client.ORDER_TIME_IN_FORCE_IMMEDIATE_OR_CANCEL:
        return client.DEFAULT_IOC_EXPIRY
    return client.DEFAULT_28_DAY_ORDER_EXPIRY


def benchmark_order_type(client: Any, params: dict[str, Any]) -> str:
    order_type = int(params.get("order_type", client.ORDER_TYPE_LIMIT))
    time_in_force = int(params.get("time_in_force", client.ORDER_TIME_IN_FORCE_POST_ONLY))
    if order_type == client.ORDER_TYPE_MARKET:
        return "market"
    if time_in_force == client.ORDER_TIME_IN_FORCE_POST_ONLY:
        return "post_only"
    if time_in_force == client.ORDER_TIME_IN_FORCE_IMMEDIATE_OR_CANCEL:
        return "ioc"
    return "limit"


def order_index(req: dict[str, Any], params: dict[str, Any], offset: int) -> int:
    if params.get("client_order_index") is not None:
        return int(params["client_order_index"]) + offset
    run_id = params.get("run_id")
    if run_id:
        seed = f"{run_id}:{req.get('iteration', 0)}:{params.get('market_index')}:{params.get('side', 'buy')}:{offset}".encode()
        return int.from_bytes(hashlib.blake2b(seed, digest_size=8).digest(), "big") % 1_900_000_000
    return int(time.time_ns() % 2_000_000_000) + offset


def order_nonce(client: Any, params: dict[str, Any], api_key_index: int, offset: int) -> tuple[int, int]:
    if params.get("nonce_base") is not None:
        return api_key_index, int(params["nonce_base"]) + offset
    if params.get("nonce") is not None:
        return api_key_index, int(params["nonce"])
    state_file = params.get("nonce_state_file") or os.getenv("LIGHTER_NONCE_STATE_FILE")
    if state_file:
        return api_key_index, next_nonce(client, api_key_index, str(state_file))
    return client.get_api_key_nonce(api_key_index, -1)


def next_nonce(client: Any, api_key_index: int, state_file: str) -> int:
    directory = os.path.dirname(state_file)
    if directory:
        os.makedirs(directory, exist_ok=True)
    with open(state_file, "a+", encoding="utf-8") as handle:
        fcntl.flock(handle, fcntl.LOCK_EX)
        handle.seek(0)
        raw = handle.read().strip()
        state = json.loads(raw) if raw else {}
        key = str(api_key_index)
        _remote_api_key_index, remote_nonce = client.get_api_key_nonce(api_key_index, -1)
        remote_nonce = int(remote_nonce)
        last_nonce = int(state.get(key, remote_nonce - 1))
        max_drift = int(os.getenv("LIGHTER_NONCE_MAX_DRIFT", "0"))
        if max_drift > 0 and remote_nonce <= last_nonce < remote_nonce + max_drift:
            nonce = last_nonce + 1
        else:
            nonce = remote_nonce
        state[key] = nonce
        handle.seek(0)
        handle.truncate()
        json.dump(state, handle, separators=(",", ":"))
        handle.flush()
        os.fsync(handle.fileno())
        return nonce


def env_or_param(params: dict[str, Any], key: str, env_key: str) -> str:
    value = params.get(key) or os.getenv(env_key)
    if value in (None, ""):
        raise SystemExit(f"missing {key}; set params.{key} or {env_key}")
    return str(value)


def compact_json(value: Any) -> str:
    return json.dumps(value, separators=(",", ":"), sort_keys=False)


if __name__ == "__main__":
    raise SystemExit(main())
