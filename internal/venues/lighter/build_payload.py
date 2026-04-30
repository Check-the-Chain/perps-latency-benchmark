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
            nonce_base = int(params.get("nonce_base") or time.time_ns())
            for offset in range(batch_size):
                tx_type, tx_info, cleanup_ref = sign_order(client, params, api_key_index, nonce_base + offset, offset)
                tx_types.append(tx_type)
                tx_infos.append(tx_info)
                cleanup_orders.append(cleanup_ref)
            body = urlencode({"tx_types": json.dumps(tx_types), "tx_infos": json.dumps(tx_infos)})
            ws_body = compact_json({"type": "jsonapi/sendtxbatch", "data": {"tx_types": tx_types, "tx_infos": tx_infos}})
            metadata = {"builder": "lighter-python-sdk", "orders": batch_size, "cleanup_orders": cleanup_orders}
        else:
            tx_type, tx_info, cleanup_ref = sign_order(client, params, api_key_index, int(params.get("nonce") or -1), 0)
            body = urlencode({"tx_type": tx_type, "tx_info": tx_info})
            ws_body = compact_json({"type": "jsonapi/sendtx", "data": {"tx_type": tx_type, "tx_info": tx_info}})
            metadata = {"builder": "lighter-python-sdk", "orders": 1, "cleanup_orders": [cleanup_ref]}
    finally:
        await client.close()

    return {
        "headers": {"Content-Type": "application/x-www-form-urlencoded"},
        "body": body,
        "ws_body": ws_body,
        "metadata": metadata,
    }


def sign_order(client: Any, params: dict[str, Any], api_key_index: int, nonce: int, offset: int) -> tuple[int, str, dict[str, Any]]:
    client_order_index = int(params.get("client_order_index") or (time.time_ns() % 2_000_000_000)) + offset
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
        order_expiry=int(params.get("order_expiry", client.DEFAULT_28_DAY_ORDER_EXPIRY)),
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


def env_or_param(params: dict[str, Any], key: str, env_key: str) -> str:
    value = params.get(key) or os.getenv(env_key)
    if value in (None, ""):
        raise SystemExit(f"missing {key}; set params.{key} or {env_key}")
    return str(value)


def compact_json(value: Any) -> str:
    return json.dumps(value, separators=(",", ":"), sort_keys=False)


if __name__ == "__main__":
    raise SystemExit(main())
