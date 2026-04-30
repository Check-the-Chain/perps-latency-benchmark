#!/usr/bin/env python3
"""Build signed Lighter cancel-order payloads for benchmark cleanup."""

from __future__ import annotations

import asyncio
import json
import os
import sys
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
    metadata = dict(params.get("metadata") or {})
    builder_params = dict(params.get("builder_params") or {})
    orders = [order for order in metadata.get("cleanup_orders") or [] if order.get("venue") == "lighter"]
    if not orders:
        return {"metadata": {"cleanup": "skipped", "reason": "no lighter cleanup_orders"}}

    api_key_index = int(env_or_param(builder_params, "api_key_index", "LIGHTER_API_KEY_INDEX"))
    account_index = int(env_or_param(builder_params, "account_index", "LIGHTER_ACCOUNT_INDEX"))
    private_key = env_or_param(builder_params, "private_key", "LIGHTER_PRIVATE_KEY")

    client = lighter.SignerClient(
        url=builder_params.get("base_url", "https://mainnet.zklighter.elliot.ai"),
        api_private_keys={api_key_index: private_key},
        account_index=account_index,
    )
    try:
        tx_types, tx_infos = [], []
        for offset, order in enumerate(orders):
            tx_type, tx_info = sign_cancel(client, order, api_key_index, cleanup_nonce(builder_params, offset))
            tx_types.append(tx_type)
            tx_infos.append(tx_info)
    finally:
        await client.close()

    headers = {"Content-Type": "application/x-www-form-urlencoded"}
    if len(tx_types) == 1:
        return {
            "headers": headers,
            "body": urlencode({"tx_type": tx_types[0], "tx_info": tx_infos[0]}),
            "metadata": {"cleanup": "cancel_order", "orders": 1},
        }
    return {
        "url": builder_params.get("cancel_batch_url", "https://mainnet.zklighter.elliot.ai/api/v1/sendTxBatch"),
        "headers": headers,
        "body": urlencode({"tx_types": json.dumps(tx_types), "tx_infos": json.dumps(tx_infos)}),
        "metadata": {"cleanup": "cancel_order", "orders": len(tx_types)},
    }


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


def cleanup_nonce(params: dict[str, Any], offset: int) -> int:
    if params.get("cleanup_nonce_base") is not None:
        return int(params["cleanup_nonce_base"]) + offset
    if params.get("cleanup_nonce") is not None:
        return int(params["cleanup_nonce"])
    return -1


def env_or_param(params: dict[str, Any], key: str, env_key: str) -> str:
    value = params.get(key) or os.getenv(env_key)
    if value in (None, ""):
        raise SystemExit(f"missing {key}; set params.{key} or {env_key}")
    return str(value)


def compact_json(value: Any) -> str:
    return json.dumps(value, separators=(",", ":"), sort_keys=False)


if __name__ == "__main__":
    raise SystemExit(main())
