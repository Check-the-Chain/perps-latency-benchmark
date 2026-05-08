#!/usr/bin/env python3
"""Build signed Nado cancel-order payloads for benchmark cleanup."""

from __future__ import annotations

import json
import os
import sys
import time
from typing import Any

from build_payload import (
    DEFAULT_CHAIN_ID,
    DEFAULT_SUBSCRIPTION_WS,
    compact_json,
    env_or_param,
    sign_typed_data,
    stream_domain,
    stream_auth_types,
)


def main() -> int:
    try:
        from eth_account import Account
        from eth_account.messages import encode_typed_data
    except ImportError as exc:
        raise SystemExit("missing Nado cancel dependency; run with `uv run --with eth-account python ...`") from exc

    for line in sys.stdin:
        if not line.strip():
            continue
        built = build(json.loads(line), Account, encode_typed_data)
        print(compact_json(built), flush=True)
    return 0


def build(req: dict[str, Any], Account: Any, encode_typed_data: Any) -> dict[str, Any]:
    params = dict(req.get("params") or {})
    builder_params = dict(params.get("builder_params") or {})
    private_key = env_or_param(builder_params, "private_key", "NADO_PRIVATE_KEY")
    orders = cleanup_orders(params)
    if not orders:
        return {"cleanup": {"attempted": False, "ok": True, "description": "no Nado cleanup_orders"}}

    sender = sender_for_orders(orders, builder_params)
    product_ids = [int(order.get("asset") or order.get("product_id") or builder_params.get("product_id", 1)) for order in orders]
    digests = [digest_for_order(order) for order in orders]
    tx = {
        "sender": sender,
        "productIds": product_ids,
        "digests": digests,
        "nonce": cancel_nonce(builder_params),
    }
    signature, _ = sign_typed_data(
        Account,
        encode_typed_data,
        private_key,
        cancel_domain(builder_params),
        cancellation_types(),
        tx,
    )
    request_id = int(time.time_ns() & 0x7fffffff)
    payload = {
        "cancel_orders": {
            "tx": {
                "sender": tx["sender"],
                "productIds": tx["productIds"],
                "digests": tx["digests"],
                "nonce": str(tx["nonce"]),
            },
            "signature": signature,
            "id": request_id,
        }
    }
    body = compact_json(payload)
    product_id = product_ids[0] if len(set(product_ids)) == 1 else None
    return {
        "headers": {
            "Content-Type": "application/json",
            "Accept-Encoding": "gzip, br, deflate",
        },
        "body": body,
        "ws_body": body,
        "metadata": {
            "cleanup": "cancel_orders",
            "orders": len(orders),
            "digests": digests,
            "cancel_confirmation": cancel_confirmation_metadata(
                Account,
                encode_typed_data,
                private_key,
                builder_params,
                sender,
                product_id,
                digests,
            ),
        },
    }


def cleanup_orders(params: dict[str, Any]) -> list[dict[str, Any]]:
    raw = params.get("order_refs") or []
    orders = [dict(order) for order in raw if dict(order).get("venue") == "nado"]
    if orders:
        return orders
    metadata = dict(params.get("metadata") or {})
    return [dict(order) for order in metadata.get("cleanup_orders") or [] if dict(order).get("venue") == "nado"]


def sender_for_orders(orders: list[dict[str, Any]], params: dict[str, Any]) -> str:
    for order in orders:
        sender = order.get("client_order_id") or order.get("sender")
        if sender:
            return str(sender)
    return env_or_param(params, "sender", "NADO_SENDER")


def digest_for_order(order: dict[str, Any]) -> str:
    digest = order.get("cloid") or order.get("digest")
    if not digest:
        raise SystemExit("Nado cleanup order missing digest/cloid")
    return str(digest)


def cancel_domain(params: dict[str, Any]) -> dict[str, Any]:
    return {
        "name": "Nado",
        "version": "0.0.1",
        "chainId": int(params.get("chain_id", os.getenv("NADO_CHAIN_ID", DEFAULT_CHAIN_ID))),
        "verifyingContract": env_or_param(params, "endpoint_contract", "NADO_ENDPOINT_CONTRACT"),
    }


def cancellation_types() -> dict[str, Any]:
    return {
        "Cancellation": [
            {"name": "sender", "type": "bytes32"},
            {"name": "productIds", "type": "uint32[]"},
            {"name": "digests", "type": "bytes32[]"},
            {"name": "nonce", "type": "uint64"},
        ]
    }


def cancel_nonce(params: dict[str, Any]) -> int:
    if params.get("cancel_nonce") is not None:
        return int(str(params["cancel_nonce"]), 0)
    recv_window_ms = int(params.get("cancel_recv_window_ms", params.get("recv_window_ms", 5000)))
    return ((int(time.time() * 1000) + recv_window_ms) << 20) + (time.time_ns() & ((1 << 20) - 1))


def cancel_confirmation_metadata(
    Account: Any,
    encode_typed_data: Any,
    private_key: str,
    params: dict[str, Any],
    sender: str,
    product_id: int | None,
    digests: list[str],
) -> dict[str, Any]:
    if params.get("cancel_confirmation") is not True:
        return {}
    expiration = int(time.time() * 1000) + int(params.get("stream_auth_ttl_ms", 90_000))
    tx = {"sender": sender, "expiration": expiration}
    signature, _ = sign_typed_data(Account, encode_typed_data, private_key, stream_domain(params), stream_auth_types(), tx)
    return {
        "venue": "nado",
        "ws_url": params.get("subscription_ws", DEFAULT_SUBSCRIPTION_WS),
        "subaccount": sender,
        "product_id": product_id,
        "digests": digests,
        "auth": {
            "method": "authenticate",
            "id": int(time.time_ns() & 0x7fffffff),
            "tx": {"sender": sender, "expiration": str(expiration)},
            "signature": signature,
        },
    }


if __name__ == "__main__":
    raise SystemExit(main())
