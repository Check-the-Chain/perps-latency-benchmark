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
import inspect
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
    order_type = benchmark_order_type(lighter.SignerClient, params)
    api_key_index, private_key, api_key_role = api_key_material(params, order_type)
    account_index = int(env_or_param(params, "account_index", "LIGHTER_ACCOUNT_INDEX"))

    client = lighter.SignerClient(
        url=params.get("base_url", "https://mainnet.zklighter.elliot.ai"),
        api_private_keys={api_key_index: private_key},
        account_index=account_index,
    )
    try:
        speed_bump = await speed_bump_metadata(lighter, client, params, api_key_index, account_index, order_type)
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
            metadata = {"builder": "lighter-python-sdk", "orders": batch_size, "run_id": params.get("run_id"), "order_type": order_type, "api_key_role": api_key_role, "cleanup_orders": cleanup_orders, **speed_bump}
        else:
            order_api_key_index, nonce = order_nonce(client, params, api_key_index, 0)
            tx_type, tx_info, cleanup_ref = sign_order(client, req, params, order_api_key_index, nonce, 0)
            body = urlencode({"tx_type": tx_type, "tx_info": tx_info})
            ws_body = compact_json({"type": "jsonapi/sendtx", "data": {"tx_type": tx_type, "tx_info": json.loads(tx_info)}})
            metadata = {"builder": "lighter-python-sdk", "orders": 1, "run_id": params.get("run_id"), "order_type": order_type, "api_key_role": api_key_role, "cleanup_orders": [cleanup_ref], **speed_bump}
        metadata["confirmation"] = confirmation_metadata(client, params, api_key_index, account_index, metadata["order_type"], cleanup_orders if scenario == "batch" else [cleanup_ref])
    finally:
        await client.close()

    return {
        "headers": {"Content-Type": "application/x-www-form-urlencoded"},
        "body": body,
        "ws_body": ws_body,
        "metadata": metadata,
    }


def api_key_material(params: dict[str, Any], order_type: str) -> tuple[int, str, str]:
    explicit_key = params.get("api_key_index") is not None or params.get("private_key")
    role = str(params.get("api_key_role") or ("default" if explicit_key else "auto")).lower()
    if role == "auto":
        role = "maker" if order_type == "post_only" else "taker"
    if role == "maker" and order_type != "post_only":
        raise SystemExit("Lighter maker API keys can only be used for post-only orders")
    if role not in {"maker", "taker", "default"}:
        raise SystemExit(f"unsupported Lighter api_key_role {role!r}")

    if explicit_key:
        return (
            int(env_or_param(params, "api_key_index", "LIGHTER_API_KEY_INDEX")),
            env_or_param(params, "private_key", "LIGHTER_PRIVATE_KEY"),
            role,
        )

    if role == "maker":
        api_key_index = first_value(params, "maker_api_key_index", "LIGHTER_MAKER_API_KEY_INDEX", "LIGHTER_API_KEY_INDEX")
        private_key = first_value(params, "maker_private_key", "LIGHTER_MAKER_PRIVATE_KEY", "LIGHTER_PRIVATE_KEY")
    elif role == "taker":
        api_key_index = first_value(params, "taker_api_key_index", "LIGHTER_TAKER_API_KEY_INDEX", "LIGHTER_API_KEY_INDEX")
        private_key = first_value(params, "taker_private_key", "LIGHTER_TAKER_PRIVATE_KEY", "LIGHTER_PRIVATE_KEY")
    else:
        api_key_index = first_value(params, "api_key_index", "LIGHTER_API_KEY_INDEX")
        private_key = first_value(params, "private_key", "LIGHTER_PRIVATE_KEY")

    return int(api_key_index), private_key, role


def sign_order(client: Any, req: dict[str, Any], params: dict[str, Any], api_key_index: int, nonce: int, offset: int) -> tuple[int, str, dict[str, Any]]:
    client_order_index = order_index(req, params, offset)
    market_index = int(params["market_index"])
    tx_type, tx_info, _tx_hash, error = client.sign_create_order(
        market_index=market_index,
        client_order_index=client_order_index,
        base_amount=int(params["base_amount"]),
        price=price_for_offset(params, offset),
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


def price_for_offset(params: dict[str, Any], offset: int) -> int:
    price = int(params["price"])
    if offset <= 0:
        return price
    if params.get("price_step_bps") is not None:
        step = max(int(round(price * float(params["price_step_bps"]) / 10000)), 1)
    else:
        step = int(params.get("price_step", 0))
    if step == 0:
        return price
    return price - (step * offset) if str(params.get("side", "buy")).lower() == "buy" else price + (step * offset)


def confirmation_metadata(client: Any, params: dict[str, Any], api_key_index: int, account_index: int, order_type: str, orders: list[dict[str, Any]]) -> dict[str, Any]:
    auth, error = client.create_auth_token_with_expiry(api_key_index=api_key_index)
    if error:
        raise SystemExit(f"Lighter auth token failed: {error}")
    return {
        "venue": "lighter",
        "ws_url": params.get("ws_url", "wss://mainnet.zklighter.elliot.ai/stream"),
        "auth_token": auth,
        "account_index": account_index,
        "api_key_index": api_key_index,
        "market_index": int(params["market_index"]),
        "order_type": order_type,
        "order_indices": [order["order_index"] for order in orders],
    }


async def speed_bump_metadata(lighter: Any, client: Any, params: dict[str, Any], api_key_index: int, account_index: int, order_type: str) -> dict[str, Any]:
    if params.get("speed_bump_ms") is not None:
        speed_bump_ms = int(float(params["speed_bump_ms"]))
        return {
            "speed_bump_ns": speed_bump_ms * 1_000_000,
            "speed_bump_ms": speed_bump_ms,
            "speed_bump_source": "lighter params.speed_bump_ms override",
        }

    limits = await account_limits(lighter, client, api_key_index, account_index)
    if limits is None:
        return {
            "speed_bump_ns": 0,
            "speed_bump_ms": 0,
            "speed_bump_source": "lighter account tier unavailable; no speed bump subtracted",
        }

    user_tier = str(first_deep(limits, "user_tier_name", "user_tier") or "standard").lower()
    effective_lit = int(float(first_deep(limits, "effective_lit_stakes", "leased_lit") or 0))
    if order_type in {"market", "ioc"}:
        speed_bump_ms = lighter_taker_latency_ms(user_tier, effective_lit)
        source = f"lighter {user_tier} taker latency tier; effective_lit_stakes={effective_lit}"
    else:
        speed_bump_ms = 0 if user_tier == "premium" else 200
        source = f"lighter {user_tier} maker/cancel latency tier"
    return {
        "speed_bump_ns": speed_bump_ms * 1_000_000,
        "speed_bump_ms": speed_bump_ms,
        "speed_bump_source": source,
    }


async def account_limits(lighter: Any, client: Any, api_key_index: int, account_index: int) -> dict[str, Any] | None:
    account_api = getattr(lighter, "AccountApi", None)
    api_client = getattr(client, "api_client", None)
    if account_api is None or api_client is None:
        return None
    auth, error = client.create_auth_token_with_expiry(api_key_index=api_key_index)
    if error:
        raise SystemExit(f"Lighter auth token failed for account limits: {error}")
    api = account_api(api_client)
    response = api.account_limits(account_index=account_index, auth=auth)
    if inspect.isawaitable(response):
        response = await response
    return to_plain(response)


def lighter_taker_latency_ms(user_tier: str, effective_lit: int) -> int:
    if user_tier != "premium":
        return 300
    tiers = (
        (500_000, 140),
        (300_000, 150),
        (100_000, 160),
        (30_000, 170),
        (10_000, 180),
        (3_000, 190),
        (1_000, 195),
        (0, 200),
    )
    for minimum, latency_ms in tiers:
        if effective_lit >= minimum:
            return latency_ms
    return 200


def first_deep(value: Any, *keys: str) -> Any:
    plain = to_plain(value)
    if isinstance(plain, dict):
        for key in keys:
            if key in plain and plain[key] not in (None, ""):
                return plain[key]
        for nested in plain.values():
            found = first_deep(nested, *keys)
            if found not in (None, ""):
                return found
    if isinstance(plain, list):
        for nested in plain:
            found = first_deep(nested, *keys)
            if found not in (None, ""):
                return found
    return None


def to_plain(value: Any) -> Any:
    if hasattr(value, "to_dict"):
        return value.to_dict()
    if isinstance(value, dict):
        return {key: to_plain(item) for key, item in value.items()}
    if isinstance(value, list):
        return [to_plain(item) for item in value]
    if hasattr(value, "__dict__") and not isinstance(value, (str, bytes)):
        return {key: to_plain(item) for key, item in vars(value).items() if not key.startswith("_")}
    return value


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
    if run_id and is_fill_likely(params):
        seed = f"{run_id}:{req.get('iteration', 0)}:{params.get('market_index')}:{params.get('side', 'buy')}:{offset}:{time.time_ns()}".encode()
        return int.from_bytes(hashlib.blake2b(seed, digest_size=8).digest(), "big") % 1_900_000_000
    if run_id:
        seed = f"{run_id}:{req.get('iteration', 0)}:{params.get('market_index')}:{params.get('side', 'buy')}:{offset}".encode()
        return int.from_bytes(hashlib.blake2b(seed, digest_size=8).digest(), "big") % 1_900_000_000
    return int(time.time_ns() % 2_000_000_000) + offset


def is_fill_likely(params: dict[str, Any]) -> bool:
    order_type = str(params.get("order_type", "")).lower()
    tif = str(params.get("time_in_force", "")).lower()
    return order_type in ("market", "ioc", "fok") or tif in ("immediate_or_cancel", "fill_or_kill")


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
        account_index = getattr(client, "account_index", "")
        key = f"{account_index}:{api_key_index}" if account_index != "" else str(api_key_index)
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
    value = params.get(key) or prefixed_env(params, key) or os.getenv(env_key)
    if value in (None, ""):
        extra = prefixed_env_name(params, key)
        env_hint = f"{extra} or {env_key}" if extra else env_key
        raise SystemExit(f"missing {key}; set params.{key} or {env_hint}")
    return str(value)


def first_value(params: dict[str, Any], key: str, *env_keys: str) -> str:
    value = params.get(key)
    if value not in (None, ""):
        return str(value)
    value = prefixed_env(params, key)
    if value not in (None, ""):
        return str(value)
    for env_key in env_keys:
        value = os.getenv(env_key)
        if value not in (None, ""):
            return str(value)
    prefixed = prefixed_env_name(params, key)
    candidates = [item for item in [prefixed, *env_keys] if item]
    raise SystemExit(f"missing {key}; set params.{key} or one of {', '.join(candidates)}")


def prefixed_env(params: dict[str, Any], key: str) -> str | None:
    env_name = prefixed_env_name(params, key)
    if not env_name:
        return None
    return os.getenv(env_name)


def prefixed_env_name(params: dict[str, Any], key: str) -> str:
    prefix = str(params.get("env_prefix") or params.get("account_env_prefix") or "").strip()
    if not prefix:
        return ""
    return f"{prefix}_{key.upper()}"


def compact_json(value: Any) -> str:
    return json.dumps(value, separators=(",", ":"), sort_keys=False)


if __name__ == "__main__":
    raise SystemExit(main())
