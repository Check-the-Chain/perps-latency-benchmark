#!/usr/bin/env python3
"""Build signed Pacifica websocket order payloads for perps-bench.

The script follows Pacifica's public Python SDK signing format and emits
websocket JSON messages. It does not connect or submit orders.
"""

from __future__ import annotations

import hashlib
import json
import os
import sys
import time
import uuid
from decimal import Decimal
from typing import Any


DEFAULT_WS_URL = "wss://ws.pacifica.fi/ws"


def main() -> int:
    try:
        import base58
        from solders.keypair import Keypair
    except ImportError as exc:
        raise SystemExit("Pacifica signing requires solders and base58; run with `uv run --with solders --with base58 python ...`") from exc

    for line in sys.stdin:
        if not line.strip():
            continue
        print(compact_json(build(json.loads(line), Keypair, base58)), flush=True)
    return 0


def build(req: dict[str, Any], keypair_type: Any | None = None, base58: Any | None = None) -> dict[str, Any]:
    if keypair_type is None or base58 is None:
        try:
            import base58 as base58_module
            from solders.keypair import Keypair
        except ImportError as exc:
            raise SystemExit("Pacifica signing requires solders and base58; run with `uv run --with solders --with base58 python ...`") from exc
        keypair_type = Keypair
        base58 = base58_module

    params = dict(req.get("params") or {})
    if str(req.get("transport") or "websocket") != "websocket":
        raise SystemExit("Pacifica builder is websocket-first; use request.transport=websocket")

    keypair = keypair_type.from_base58_string(env_or_param(params, "private_key", "PACIFICA_PRIVATE_KEY"))
    signer_public_key = str(keypair.pubkey())
    account = str(params.get("account") or os.getenv("PACIFICA_ACCOUNT") or signer_public_key)
    agent_wallet = str(params.get("agent_wallet") or os.getenv("PACIFICA_AGENT_WALLET") or "")
    if account != signer_public_key and not agent_wallet:
        agent_wallet = signer_public_key
    if agent_wallet and agent_wallet != signer_public_key:
        raise SystemExit(f"PACIFICA_PRIVATE_KEY derives {signer_public_key}, not agent wallet {agent_wallet}")

    scenario = str(req.get("scenario") or "single")
    batch_size = int(req.get("batch_size") or 1)
    if scenario == "batch":
        actions = [signed_create_action(params, req, keypair, base58, account, agent_wallet, offset) for offset in range(batch_size)]
        message = {"id": str(uuid.uuid4()), "params": {"batch_orders": {"actions": actions}}}
        cleanup_orders = [cleanup_ref(action["data"]) for action in actions]
        orders = len(actions)
    else:
        order = signed_order(params, req, keypair, base58, account, agent_wallet, 0)
        operation = operation_type(params)
        message = {"id": str(uuid.uuid4()), "params": {operation: order}}
        cleanup_orders = [cleanup_ref(order)]
        orders = 1

    ws_body = compact_json(message)
    order_type = benchmark_order_type(params)
    tif = str(params.get("tif") or "ALO").upper()
    return {
        "ws_body": ws_body,
        "ws_batch_body": ws_body,
        "metadata": {
            "builder": "pacifica-ed25519-ws",
            "orders": orders,
            "run_id": params.get("run_id"),
            "order_type": order_type,
            "time_in_force": tif,
            "client_order_ids": [ref["client_order_id"] for ref in cleanup_orders],
            "speed_bump_ns": speed_bump_ns(scenario, order_type, tif),
            "speed_bump_ms": speed_bump_ns(scenario, order_type, tif) // 1_000_000,
            "speed_bump_source": speed_bump_source(scenario, order_type, tif),
            "cleanup_orders": cleanup_orders,
            "confirmation": confirmation_metadata(params, account, cleanup_orders, order_type),
        },
    }


def signed_create_action(params: dict[str, Any], req: dict[str, Any], keypair: Any, base58: Any, account: str, agent_wallet: str, offset: int) -> dict[str, Any]:
    return {
        "type": "CreateMarket" if operation_type(params) == "create_market_order" else "Create",
        "data": signed_order(params, req, keypair, base58, account, agent_wallet, offset),
    }


def signed_order(params: dict[str, Any], req: dict[str, Any], keypair: Any, base58: Any, account: str, agent_wallet: str, offset: int) -> dict[str, Any]:
    timestamp = int(params.get("timestamp") or time.time() * 1000)
    window = expiry_window(params)
    header = {
        "timestamp": timestamp,
        "expiry_window": window,
        "type": operation_type(params),
    }
    payload = order_payload(params, req, offset)
    signature = sign_message(header, payload, keypair, base58)
    order = {
        "account": account,
        "signature": signature,
        "timestamp": timestamp,
        "expiry_window": window,
        **payload,
    }
    if agent_wallet:
        order["agent_wallet"] = agent_wallet
    return order


def sign_message(header: dict[str, Any], payload: dict[str, Any], keypair: Any, base58: Any) -> str:
    message = compact_json(sort_json_keys({**header, "data": payload}))
    signature = keypair.sign_message(message.encode("utf-8"))
    return base58.b58encode(bytes(signature)).decode("ascii")


class PacificaSigner:
    def __init__(self, account: str, agent_wallet: str, keypair: Any, base58: Any):
        self.account = account
        self.agent_wallet = agent_wallet
        self.keypair = keypair
        self.base58 = base58

    def sign(self, operation: str, payload: dict[str, Any], timestamp: int, window: int) -> str:
        return sign_message({"timestamp": timestamp, "expiry_window": window, "type": operation}, payload, self.keypair, self.base58)

    def request(self, operation: str, payload: dict[str, Any], timestamp: int, signature: str, window: int) -> dict[str, Any]:
        request = {
            "account": self.account,
            "signature": signature,
            "timestamp": timestamp,
            "expiry_window": window,
            **payload,
        }
        if self.agent_wallet:
            request["agent_wallet"] = self.agent_wallet
        return request


def load_signer(params: dict[str, Any]) -> PacificaSigner:
    try:
        import base58
        from solders.keypair import Keypair
    except ImportError as exc:
        raise SystemExit("Pacifica signing requires solders and base58; run with `uv run --with solders --with base58 python ...`") from exc
    keypair = Keypair.from_base58_string(env_or_param(params, "private_key", "PACIFICA_PRIVATE_KEY"))
    signer_public_key = str(keypair.pubkey())
    account = str(params.get("account") or os.getenv("PACIFICA_ACCOUNT") or signer_public_key)
    agent_wallet = str(params.get("agent_wallet") or os.getenv("PACIFICA_AGENT_WALLET") or "")
    if account != signer_public_key and not agent_wallet:
        agent_wallet = signer_public_key
    if agent_wallet and agent_wallet != signer_public_key:
        raise SystemExit(f"PACIFICA_PRIVATE_KEY derives {signer_public_key}, not agent wallet {agent_wallet}")
    return PacificaSigner(account, agent_wallet, keypair, base58)


def price_for_offset(params: dict[str, Any], offset: int) -> str:
    price = Decimal(str(params.get("price", "75000")))
    if offset <= 0:
        return decimal_text(price)
    if params.get("price_step_bps") is not None:
        step = price * Decimal(str(params["price_step_bps"])) / Decimal("10000")
    else:
        step = Decimal(str(params.get("price_step", "0")))
    if step == 0:
        return decimal_text(price)
    if normalize_side(str(params.get("side", "bid"))) == "bid":
        return decimal_text(price - (step * offset))
    return decimal_text(price + (step * offset))


def decimal_text(value: Decimal) -> str:
    text = format(value.normalize(), "f")
    if "." in text:
        text = text.rstrip("0").rstrip(".")
    return text


def client_order_id(params: dict[str, Any], req: dict[str, Any], offset: int) -> str:
    explicit = params.get("client_order_id")
    if explicit:
        if offset == 0:
            return str(explicit)
        return str(uuid.uuid5(uuid.NAMESPACE_URL, f"{explicit}:{offset}"))
    run_id = params.get("run_id")
    if run_id:
        seed = f"{run_id}:{req.get('iteration', 0)}:{params.get('symbol', 'BTC')}:{params.get('side', 'bid')}:{offset}".encode()
        digest = hashlib.blake2b(seed, digest_size=16).hexdigest()
        return str(uuid.UUID(digest))
    return str(uuid.uuid4())


def cleanup_ref(order: dict[str, Any]) -> dict[str, Any]:
    return {
        "venue": "pacifica",
        "symbol": order["symbol"],
        "client_order_id": order["client_order_id"],
    }


def order_payload(params: dict[str, Any], req: dict[str, Any], offset: int) -> dict[str, Any]:
    payload = {
        "symbol": str(params.get("symbol", "BTC")).upper(),
        "reduce_only": bool(params.get("reduce_only", False)),
        "amount": str(params.get("amount", "0.001")),
        "side": normalize_side(str(params.get("side", "bid"))),
        "client_order_id": client_order_id(params, req, offset),
    }
    if operation_type(params) == "create_market_order":
        payload["slippage_percent"] = str(params.get("slippage_percent", "0.5"))
        return payload
    payload["price"] = price_for_offset(params, offset)
    payload["tif"] = str(params.get("tif", "ALO")).upper()
    return payload


def operation_type(params: dict[str, Any]) -> str:
    explicit = str(params.get("operation_type", "")).strip()
    if explicit:
        return explicit
    if str(params.get("order_type", "")).lower() == "market":
        return "create_market_order"
    return "create_order"


def confirmation_metadata(params: dict[str, Any], account: str, cleanup_orders: list[dict[str, Any]], order_type: str) -> dict[str, Any]:
    if params.get("confirmation") is False:
        return {}
    return {
        "venue": "pacifica",
        "ws_url": params.get("ws_url", DEFAULT_WS_URL),
        "account": account,
        "client_order_ids": [order["client_order_id"] for order in cleanup_orders],
        "order_type": order_type,
    }


def benchmark_order_type(params: dict[str, Any]) -> str:
    if operation_type(params) == "create_market_order":
        return "market"
    tif = str(params.get("tif") or "ALO").upper()
    if tif == "ALO":
        return "post_only"
    if tif in {"IOC", "TOB"}:
        return tif.lower()
    return "limit"


def speed_bump_ns(scenario: str, order_type: str, tif: str) -> int:
    if order_type == "market":
        return 100_000_000 if scenario == "batch" else 200_000_000
    if tif in {"ALO", "TOB"}:
        return 0
    if scenario == "batch":
        return 100_000_000
    if order_type in {"limit", "ioc", "market"}:
        return 200_000_000
    return 0


def speed_bump_source(scenario: str, order_type: str, tif: str) -> str:
    if order_type == "market" and scenario == "batch":
        return "pacifica docs: batch has randomized 50-100 ms delay for market/GTC/IOC batches; conservative 100 ms recorded"
    if order_type == "market":
        return "pacifica docs: market orders have roughly 200 ms delay"
    if tif in {"ALO", "TOB"}:
        return "pacifica docs: ALO/TOB orders avoid documented latency protection delay"
    if scenario == "batch":
        return "pacifica docs: batch has randomized 50-100 ms delay for market/GTC/IOC batches; conservative 100 ms recorded"
    if order_type in {"limit", "ioc", "market"}:
        return "pacifica docs: market and GTC/IOC limit orders have roughly 200 ms delay"
    return "pacifica docs: no fixed speed bump for this order type"


def normalize_side(side: str) -> str:
    side = side.lower().strip()
    if side == "buy":
        return "bid"
    if side == "sell":
        return "ask"
    return side


def expiry_window(params: dict[str, Any]) -> int:
    return int(params.get("expiry_window") or 5000)


def sort_json_keys(value: Any) -> Any:
    if isinstance(value, dict):
        return {key: sort_json_keys(value[key]) for key in sorted(value.keys())}
    if isinstance(value, list):
        return [sort_json_keys(item) for item in value]
    return value


def compact_json(value: Any) -> str:
    return json.dumps(value, separators=(",", ":"), ensure_ascii=True)


def env_or_param(params: dict[str, Any], key: str, env_key: str) -> str:
    value = params.get(key)
    if value not in (None, ""):
        return str(value)
    value = os.getenv(env_key, "")
    if value:
        return value
    raise SystemExit(f"missing {env_key}")


if __name__ == "__main__":
    raise SystemExit(main())
