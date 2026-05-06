#!/usr/bin/env python3
"""Build signed Aster futures V3 order payloads for perps-bench.

The script emits a signed application/x-www-form-urlencoded request body. It
does not submit the order. When confirmation metadata is enabled, it creates or
reuses a signed user data stream listenKey outside the measured submit window.
"""

from __future__ import annotations

import hashlib
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from typing import Any


DEFAULT_BASE_URL = "https://fapi.asterdex.com"
DEFAULT_WS_BASE_URL = "wss://fstream.asterdex.com"
DEFAULT_ORDER_PATH = "/fapi/v3/order"
DEFAULT_BATCH_ORDER_PATH = "/fapi/v3/batchOrders"
DEFAULT_LISTEN_KEY_PATH = "/fapi/v3/listenKey"
ASTER_CHAIN_ID = 1666
ASTER_VERIFYING_CONTRACT = "0x0000000000000000000000000000000000000000"
_LISTEN_KEYS: dict[tuple[str, str, str], tuple[str, float]] = {}


def main() -> int:
    for line in sys.stdin:
        if not line.strip():
            continue
        print(compact_json(build(json.loads(line))), flush=True)
    return 0


def build(req: dict[str, Any]) -> dict[str, Any]:
    params = dict(req.get("params") or {})
    scenario = str(req.get("scenario") or "single")
    if str(req.get("transport") or "http") == "websocket":
        raise SystemExit("Aster builder supports HTTP order submission; official WebSocket docs expose market and user data streams, not order submission")

    signer = load_signer(params)
    batch_size = int(req.get("batch_size") or 1)
    orders = params.get("orders")
    if orders is None:
        orders = [order_params(params, req, offset) for offset in range(batch_size if scenario == "batch" else 1)]
    if scenario != "batch":
        orders = list(orders)[:1]

    if scenario == "batch":
        body = signed_batch_body(orders, signer)
        path = str(params.get("batch_path", DEFAULT_BATCH_ORDER_PATH))
    else:
        body = signed_order_body(dict(orders[0]), signer)
        path = str(params.get("path", DEFAULT_ORDER_PATH))

    cleanup_orders = [cleanup_ref(order) for order in orders]
    built: dict[str, Any] = {
        "method": "POST",
        "headers": {
            "Content-Type": "application/x-www-form-urlencoded",
        },
        "body": body,
        "metadata": {
            "builder": "aster-api-wallet-v3",
            "orders": len(orders),
            "client_order_ids": [order["newClientOrderId"] for order in orders],
            "request_nonce": body_value(body, "nonce"),
            "run_id": params.get("run_id"),
            "order_type": benchmark_order_type(orders[0]),
            "time_in_force": str(orders[0].get("timeInForce") or ""),
            "speed_bump_ns": 0,
            "speed_bump_ms": 0,
            "speed_bump_source": "aster official API docs do not document a fixed order-entry speed bump",
            "cleanup_orders": cleanup_orders,
            "confirmation": confirmation_metadata(params, signer, cleanup_orders, benchmark_order_type(orders[0])),
        },
    }
    if path != DEFAULT_ORDER_PATH and scenario != "batch":
        built["url"] = str(params.get("base_url", DEFAULT_BASE_URL)).rstrip("/") + path
    if path != DEFAULT_BATCH_ORDER_PATH and scenario == "batch":
        built["batch_url"] = str(params.get("base_url", DEFAULT_BASE_URL)).rstrip("/") + path
    return built


def order_params(params: dict[str, Any], req: dict[str, Any], offset: int) -> dict[str, str]:
    order_type = str(params.get("type") or params.get("order_type") or "LIMIT").upper()
    order: dict[str, str] = {
        "symbol": str(params.get("symbol", "BTCUSDT")).upper(),
        "side": str(params.get("side", "BUY")).upper(),
        "type": order_type,
        "newClientOrderId": client_order_id(params, req, offset),
        "newOrderRespType": str(params.get("new_order_resp_type", "ACK")).upper(),
    }
    if params.get("position_side") is not None:
        order["positionSide"] = str(params["position_side"]).upper()
    if params.get("reduce_only") is not None:
        order["reduceOnly"] = "true" if bool(params["reduce_only"]) else "false"
    if order_type != "MARKET":
        order["timeInForce"] = str(params.get("time_in_force") or params.get("tif") or "GTX").upper()
    if params.get("quantity") is not None or params.get("size") is not None:
        order["quantity"] = str(params.get("quantity", params.get("size")))
    if params.get("price") is not None and order_type != "MARKET":
        order["price"] = str(price_for_offset(params, offset))
    for source, target in (
        ("stop_price", "stopPrice"),
        ("activation_price", "activationPrice"),
        ("callback_rate", "callbackRate"),
        ("working_type", "workingType"),
        ("price_protect", "priceProtect"),
        ("close_position", "closePosition"),
    ):
        if params.get(source) is not None:
            order[target] = str(params[source])
    return order


def price_for_offset(params: dict[str, Any], offset: int) -> str:
    from decimal import Decimal

    base = Decimal(str(params["price"]))
    if offset <= 0:
        return str(base)
    if params.get("price_step_bps") is not None:
        step = base * Decimal(str(params["price_step_bps"])) / Decimal("10000")
    else:
        step = Decimal(str(params.get("price_step", "0")))
    if step == 0:
        return decimal_text(base)
    side = str(params.get("side", "BUY")).lower()
    value = base - (step * offset) if side == "buy" else base + (step * offset)
    return decimal_text(value)


def decimal_text(value: Any) -> str:
    text = format(value.normalize(), "f")
    if "." in text:
        text = text.rstrip("0").rstrip(".")
    return text


def signed_order_body(order: dict[str, str], signer: "AsterSigner") -> str:
    return signer.sign(dict(order))


def signed_batch_body(orders: list[dict[str, str]], signer: "AsterSigner") -> str:
    return signer.sign({"batchOrders": compact_json(orders)})


def body_value(body: str, key: str) -> str:
    parsed = urllib.parse.parse_qs(body)
    values = parsed.get(key) or [""]
    return values[0]


class AsterSigner:
    def __init__(self, user: str, signer: str, private_key: str):
        self.user = user
        self.signer = signer
        self.private_key = private_key

    def sign(self, values: dict[str, str]) -> str:
        payload = dict(values)
        payload["nonce"] = str(microsecond_nonce())
        payload["user"] = self.user
        payload["signer"] = self.signer
        body = urllib.parse.urlencode(payload)
        return body + "&signature=" + sign_message(body, self.private_key)


def load_signer(params: dict[str, Any]) -> AsterSigner:
    user = env_or_param(params, "user", "ASTER_USER_ADDRESS")
    signer = env_or_param(params, "signer", "ASTER_API_WALLET_ADDRESS")
    private_key = env_or_param(params, "private_key", "ASTER_API_PRIVATE_KEY")
    derived = signer_address(private_key)
    if derived.lower() != signer.lower():
        raise SystemExit(f"ASTER_API_PRIVATE_KEY derives {derived}, not signer {signer}")
    return AsterSigner(user, signer, private_key)


def microsecond_nonce() -> int:
    return time.time_ns() // 1_000


def sign_values(values: dict[str, str], private_key: str) -> str:
    """Compatibility helper used by tests and cleanup."""
    body = urllib.parse.urlencode(values)
    return body + "&signature=" + sign_message(body, private_key)


def sign_message(message: str, private_key: str) -> str:
    try:
        from eth_account import Account
        from eth_account.messages import encode_typed_data
    except ImportError as exc:
        raise SystemExit("Aster V3 signing requires eth-account; run with `uv run --with eth-account python ...`") from exc

    signable = encode_typed_data(full_message=typed_data(message))
    signature = Account.sign_message(signable, private_key=private_key).signature.hex()
    if not signature.startswith("0x"):
        signature = "0x" + signature
    return signature


def signer_address(private_key: str) -> str:
    try:
        from eth_account import Account
    except ImportError as exc:
        raise SystemExit("Aster V3 signing requires eth-account; run with `uv run --with eth-account python ...`") from exc
    return str(Account.from_key(private_key).address)


def typed_data(message: str) -> dict[str, Any]:
    return {
        "types": {
            "EIP712Domain": [
                {"name": "name", "type": "string"},
                {"name": "version", "type": "string"},
                {"name": "chainId", "type": "uint256"},
                {"name": "verifyingContract", "type": "address"},
            ],
            "Message": [{"name": "msg", "type": "string"}],
        },
        "primaryType": "Message",
        "domain": {
            "name": "AsterSignTransaction",
            "version": "1",
            "chainId": ASTER_CHAIN_ID,
            "verifyingContract": ASTER_VERIFYING_CONTRACT,
        },
        "message": {"msg": message},
    }


def client_order_id(params: dict[str, Any], req: dict[str, Any], offset: int) -> str:
    explicit = params.get("new_client_order_id") or params.get("client_order_id")
    if explicit:
        raw = str(explicit)
        if offset == 0:
            return raw
        return (raw[:30] + f"_{offset}")[:36]
    if params.get("client_order_id_base") is not None:
        return str(int(params["client_order_id_base"]) + int(req.get("iteration") or 0) + offset)
    run_id = params.get("run_id")
    if run_id and is_fill_likely(params):
        seed = f"{run_id}:{req.get('iteration', 0)}:{params.get('symbol', 'BTCUSDT')}:{params.get('side', 'BUY')}:{offset}:{time.time_ns()}".encode()
        return "pb_" + hashlib.blake2b(seed, digest_size=12).hexdigest()
    if run_id:
        seed = f"{run_id}:{req.get('iteration', 0)}:{params.get('symbol', 'BTCUSDT')}:{params.get('side', 'BUY')}:{offset}".encode()
        return "pb_" + hashlib.blake2b(seed, digest_size=12).hexdigest()
    return "pb_" + str(time.time_ns())[-24:]


def is_fill_likely(params: dict[str, Any]) -> bool:
    order_type = str(params.get("type", params.get("order_type", "LIMIT"))).upper()
    tif = str(params.get("time_in_force", params.get("timeInForce", "GTC"))).upper()
    return order_type == "MARKET" or tif in ("IOC", "FOK")


def cleanup_ref(order: dict[str, str]) -> dict[str, str]:
    return {
        "venue": "aster",
        "symbol": str(order["symbol"]),
        "client_order_id": str(order["newClientOrderId"]),
    }


def benchmark_order_type(order: dict[str, str]) -> str:
    order_type = str(order.get("type") or "").lower()
    tif = str(order.get("timeInForce") or "").lower()
    if str(order.get("reduceOnly") or "").lower() == "true":
        return "reduce_only"
    if order_type == "market":
        return "market"
    if tif == "gtx":
        return "post_only"
    if tif in ("ioc", "fok"):
        return tif
    return order_type or "limit"


def confirmation_metadata(params: dict[str, Any], signer: AsterSigner, orders: list[dict[str, str]], order_type: str) -> dict[str, Any]:
    if params.get("confirmation") is False or params.get("prepare_confirmation") is False:
        return {}
    listen_key = str(params.get("listen_key") or cached_listen_key(params, signer))
    ws_base = str(params.get("ws_url") or params.get("ws_base_url") or DEFAULT_WS_BASE_URL).rstrip("/")
    if ws_base.endswith("/ws"):
        ws_url = ws_base + "/" + listen_key
    else:
        ws_url = ws_base + "/ws/" + listen_key
    return {
        "venue": "aster",
        "ws_url": ws_url,
        "listen_key": listen_key,
        "client_order_ids": [order["client_order_id"] for order in orders],
        "order_type": order_type,
    }


def cached_listen_key(params: dict[str, Any], signer: AsterSigner) -> str:
    base_url = str(params.get("base_url", DEFAULT_BASE_URL)).rstrip("/")
    key = (base_url, signer.user, signer.signer)
    cached = _LISTEN_KEYS.get(key)
    now = time.time()
    if cached and cached[1] > now:
        return cached[0]
    listen_key = start_listen_key(base_url, signer, float(params.get("listen_key_timeout_seconds", 10)))
    _LISTEN_KEYS[key] = (listen_key, now + 45 * 60)
    return listen_key


def start_listen_key(base_url: str, signer: AsterSigner, timeout: float) -> str:
    body = signer.sign({})
    request = urllib.request.Request(
        base_url + DEFAULT_LISTEN_KEY_PATH,
        data=body.encode(),
        method="POST",
        headers={"Content-Type": "application/x-www-form-urlencoded"},
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            payload = json.loads(response.read().decode())
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode(errors="replace")
        raise SystemExit(f"failed to start Aster listenKey: http {exc.code}: {detail}") from exc
    if not payload.get("listenKey"):
        raise SystemExit(f"failed to start Aster listenKey: {payload}")
    return str(payload["listenKey"])


def env_or_param(params: dict[str, Any], key: str, env_key: str) -> str:
    value = params.get(key) or os.getenv(env_key)
    if value in (None, ""):
        raise SystemExit(f"missing {key}; set params.{key} or {env_key}")
    return str(value)


def compact_json(value: Any) -> str:
    return json.dumps(value, separators=(",", ":"), sort_keys=False)


if __name__ == "__main__":
    raise SystemExit(main())
