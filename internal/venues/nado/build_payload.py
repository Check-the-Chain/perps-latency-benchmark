#!/usr/bin/env python3
"""Build signed Nado order payloads for perps-bench.

The script reads payload.Request JSON lines from stdin and writes payload.Built
JSON lines to stdout. It signs locally and never sends a network request.
"""

from __future__ import annotations

import hashlib
import json
import os
import random
import sys
import time
from decimal import Decimal, ROUND_DOWN, ROUND_UP
from typing import Any

X18 = Decimal("1000000000000000000")
DEFAULT_SUBSCRIPTION_WS = "wss://gateway.prod.nado.xyz/v1/subscribe"
DEFAULT_CHAIN_ID = 57073
DEFAULT_EXPIRATION = "4294967295"


def main() -> int:
    try:
        from eth_account import Account
        from eth_account.messages import encode_typed_data
    except ImportError as exc:
        raise SystemExit("missing Nado builder dependency; run with `uv run --with eth-account python ...`") from exc

    for line in sys.stdin:
        if not line.strip():
            continue
        built = build(json.loads(line), Account, encode_typed_data)
        print(compact_json(built), flush=True)
    return 0


def build(req: dict[str, Any], Account: Any, encode_typed_data: Any) -> dict[str, Any]:
    params = dict(req.get("params") or {})
    private_key = env_or_param(params, "private_key", "NADO_PRIVATE_KEY")
    wallet = Account.from_key(private_key)
    sender = sender_bytes32(params, wallet.address)

    scenario = req.get("scenario", "single")
    count = int(req.get("batch_size") or 1) if scenario == "batch" else 1
    product_id = int(params.get("product_id", 1))
    chain_id = int(params.get("chain_id", os.getenv("NADO_CHAIN_ID", DEFAULT_CHAIN_ID)))

    built_orders = [
        signed_place_order(Account, encode_typed_data, private_key, params, req, sender, product_id, chain_id, offset)
        for offset in range(count)
    ]
    digests = [item["digest"] for item in built_orders]
    cleanup_orders = [
        {
            "venue": "nado",
            "asset": product_id,
            "client_order_id": sender,
            "cloid": item["digest"],
        }
        for item in built_orders
    ]

    metadata = {
        "builder": "nado-eip712",
        "product_id": product_id,
        "symbol": params.get("symbol"),
        "digest": digests[0],
        "digests": digests,
        "order_type": normalized_order_type(params),
        "side": str(params.get("side", "buy")).lower(),
        "nonce": str(built_orders[0]["order"]["nonce"]),
        "nonces": [str(item["order"]["nonce"]) for item in built_orders],
        "recv_window_ms": int(params.get("recv_window_ms", 5000)),
        "cleanup_orders": cleanup_orders,
        "confirmation": confirmation_metadata(
            Account,
            encode_typed_data,
            private_key,
            params,
            sender,
            product_id,
            digests,
        ),
    }
    headers = {
        "Content-Type": "application/json",
        "Accept-Encoding": "gzip, br, deflate",
    }
    if scenario == "batch":
        metadata["orders"] = count
        metadata["submission_model"] = "parallel_http_single_orders"
        metadata["native_batch_endpoint"] = False
        metadata["batch_note"] = "Nado has no documented native multi-place-order endpoint; benchmark sends signed single place_order executes concurrently."
        return {
            "headers": headers,
            "parallel_requests": [
                {
                    "headers": headers,
                    "body": item["body"],
                }
                for item in built_orders
            ],
            "metadata": metadata,
        }

    body = built_orders[0]["body"]
    return {
        "headers": headers,
        "body": body,
        "ws_body": body,
        "metadata": metadata,
    }


def signed_place_order(
    Account: Any,
    encode_typed_data: Any,
    private_key: str,
    params: dict[str, Any],
    req: dict[str, Any],
    sender: str,
    product_id: int,
    chain_id: int,
    offset: int,
) -> dict[str, Any]:
    order = order_from_params(params, req, offset, sender)
    signature, digest = sign_typed_data(
        Account,
        encode_typed_data,
        private_key,
        order_domain(chain_id, product_id),
        order_types(),
        order,
    )
    request_id = int(params["request_id"]) + offset if params.get("request_id") is not None else request_id_from_digest(digest)
    payload = {
        "place_order": {
            "product_id": product_id,
            "order": stringified_order(order),
            "signature": signature,
            "id": request_id,
        }
    }
    if "spot_leverage" in params:
        payload["place_order"]["spot_leverage"] = bool_param(params, "spot_leverage", True)
    return {"order": order, "digest": digest, "body": compact_json(payload)}


def order_from_params(params: dict[str, Any], req: dict[str, Any], offset: int, sender: str) -> dict[str, Any]:
    side = str(params.get("side", "buy")).lower()
    amount = scale_x18(params.get("amount", params.get("size", "0")), signed=True)
    if side in ("sell", "ask", "short"):
        amount = -abs(amount)
    else:
        amount = abs(amount)
    return {
        "sender": sender,
        "priceX18": scale_x18(params["price"], signed=True),
        "amount": amount,
        "expiration": int(params.get("expiration", DEFAULT_EXPIRATION)),
        "nonce": order_nonce(params, req, offset),
        "appendix": appendix(params),
    }


def stringified_order(order: dict[str, Any]) -> dict[str, Any]:
    return {
        "sender": order["sender"],
        "priceX18": str(order["priceX18"]),
        "amount": str(order["amount"]),
        "expiration": str(order["expiration"]),
        "nonce": str(order["nonce"]),
        "appendix": str(order["appendix"]),
    }


def sign_typed_data(
    Account: Any,
    encode_typed_data: Any,
    private_key: str,
    domain: dict[str, Any],
    types: dict[str, Any],
    message: dict[str, Any],
) -> tuple[str, str]:
    signable = encode_typed_data(domain_data=domain, message_types=types, message_data=message)
    signed = Account.sign_message(signable, private_key)
    return hex0x(signed.signature.hex()), hex0x(signed.message_hash.hex())


def order_domain(chain_id: int, product_id: int) -> dict[str, Any]:
    return {
        "name": "Nado",
        "version": "0.0.1",
        "chainId": chain_id,
        "verifyingContract": product_verifying_contract(product_id),
    }


def stream_domain(params: dict[str, Any]) -> dict[str, Any]:
    contract = env_or_param(params, "endpoint_contract", "NADO_ENDPOINT_CONTRACT")
    return {
        "name": "Nado",
        "version": "0.0.1",
        "chainId": int(params.get("chain_id", os.getenv("NADO_CHAIN_ID", DEFAULT_CHAIN_ID))),
        "verifyingContract": contract,
    }


def order_types() -> dict[str, Any]:
    return {
        "Order": [
            {"name": "sender", "type": "bytes32"},
            {"name": "priceX18", "type": "int128"},
            {"name": "amount", "type": "int128"},
            {"name": "expiration", "type": "uint64"},
            {"name": "nonce", "type": "uint64"},
            {"name": "appendix", "type": "uint128"},
        ]
    }


def stream_auth_types() -> dict[str, Any]:
    return {
        "StreamAuthentication": [
            {"name": "sender", "type": "bytes32"},
            {"name": "expiration", "type": "uint64"},
        ]
    }


def confirmation_metadata(
    Account: Any,
    encode_typed_data: Any,
    private_key: str,
    params: dict[str, Any],
    sender: str,
    product_id: int,
    digests: list[str],
) -> dict[str, Any]:
    if params.get("confirmation") is not True:
        return {}
    expiration = int(time.time() * 1000) + int(params.get("stream_auth_ttl_ms", 90_000))
    tx = {"sender": sender, "expiration": expiration}
    signature, _ = sign_typed_data(Account, encode_typed_data, private_key, stream_domain(params), stream_auth_types(), tx)
    auth_id = int(params.get("auth_request_id") or (time.time_ns() & 0x7fffffff))
    return {
        "venue": "nado",
        "ws_url": params.get("subscription_ws", DEFAULT_SUBSCRIPTION_WS),
        "subaccount": sender,
        "product_id": product_id,
        "digests": digests,
        "order_type": normalized_order_type(params),
        "auth": {
            "method": "authenticate",
            "id": auth_id,
            "tx": {"sender": sender, "expiration": str(expiration)},
            "signature": signature,
        },
    }


def sender_bytes32(params: dict[str, Any], wallet_address: str) -> str:
    sender = params.get("sender") or os.getenv("NADO_SENDER")
    if sender:
        text = str(sender)
        if len(text) != 66 or not text.startswith("0x"):
            raise SystemExit("NADO_SENDER/params.sender must be a 32-byte hex string")
        return text.lower()
    address = str(params.get("address") or os.getenv("NADO_ADDRESS") or wallet_address)
    subaccount = str(params.get("subaccount") or os.getenv("NADO_SUBACCOUNT") or "default")
    if not address.startswith("0x") or len(address) != 42:
        raise SystemExit("NADO_ADDRESS/params.address must be a 20-byte hex address")
    suffix = subaccount.encode("ascii")
    if len(suffix) > 12:
        raise SystemExit("NADO_SUBACCOUNT/params.subaccount must be at most 12 ASCII bytes")
    return "0x" + bytes.fromhex(address[2:]).hex() + suffix.ljust(12, b"\x00").hex()


def product_verifying_contract(product_id: int) -> str:
    return "0x" + int(product_id).to_bytes(20, "big", signed=False).hex()


def appendix(params: dict[str, Any]) -> int:
    if params.get("appendix") is not None:
        return int(str(params["appendix"]), 0)
    order_type = normalized_order_type(params)
    order_type_bits = {
        "default": 0,
        "limit": 0,
        "ioc": 1,
        "market": 1,
        "fok": 2,
        "post_only": 3,
    }.get(order_type)
    if order_type_bits is None:
        raise SystemExit(f"unsupported Nado order_type {order_type!r}")
    value = 1 | (order_type_bits << 9)
    if bool_param(params, "isolated", False):
        value |= 1 << 8
    if bool_param(params, "reduce_only", False):
        value |= 1 << 11
    return value


def normalized_order_type(params: dict[str, Any]) -> str:
    text = str(params.get("order_type", "post_only")).lower().replace("-", "_")
    if text in ("alo", "maker"):
        return "post_only"
    return text


def order_nonce(params: dict[str, Any], req: dict[str, Any], offset: int) -> int:
    if params.get("nonce") is not None:
        return int(str(params["nonce"]), 0) + offset
    recv_window_ms = int(params.get("recv_window_ms", 5000))
    discard_ms = int(time.time() * 1000) + recv_window_ms
    random_bits = nonce_random_bits(params, req, offset)
    return (discard_ms << 20) + random_bits


def nonce_random_bits(params: dict[str, Any], req: dict[str, Any], offset: int) -> int:
    if params.get("nonce_random") is not None:
        return (int(str(params["nonce_random"]), 0) + offset) & ((1 << 20) - 1)
    run_id = params.get("run_id")
    if run_id:
        seed = f"{run_id}:{req.get('iteration', 0)}:{offset}:{time.time_ns()}".encode()
        return int.from_bytes(hashlib.blake2b(seed, digest_size=8).digest(), "big") & ((1 << 20) - 1)
    return random.SystemRandom().randrange(1 << 20)


def scale_x18(value: Any, signed: bool = False) -> int:
    decimal = Decimal(str(value))
    scaled = decimal * X18
    rounding = ROUND_UP if signed and scaled < 0 else ROUND_DOWN
    return int(scaled.to_integral_value(rounding=rounding))


def request_id_from_digest(digest: str) -> int:
    return int(digest[-12:], 16) & 0x7fffffff


def hex0x(value: str) -> str:
    return value if value.startswith("0x") else "0x" + value


def bool_param(params: dict[str, Any], key: str, default: bool) -> bool:
    value = params.get(key, default)
    if isinstance(value, bool):
        return value
    return str(value).lower() in ("1", "true", "yes", "on")


def env_or_param(params: dict[str, Any], key: str, env_key: str) -> str:
    value = params.get(key) or os.getenv(env_key)
    if not value:
        raise SystemExit(f"missing {key}; set params.{key} or {env_key}")
    return str(value)


def compact_json(value: Any) -> str:
    return json.dumps(value, separators=(",", ":"), sort_keys=False)


if __name__ == "__main__":
    raise SystemExit(main())
