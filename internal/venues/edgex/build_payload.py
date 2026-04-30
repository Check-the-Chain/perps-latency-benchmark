#!/usr/bin/env python3
"""Build signed edgeX createOrder payloads for perps-bench.

Run through the command builder, for example:

  uv run --with edgex-python-sdk --with requests \
    python internal/venues/edgex/build_payload.py

The script uses the official edgeX Python SDK's StarkEx signing and request
signature helpers. It expects exchange metadata in params.metadata so it can
build the payload without fetching metadata or submitting the order.
"""

from __future__ import annotations

import json
import os
import sys
import time
from inspect import signature
from decimal import ROUND_CEILING, Decimal, InvalidOperation
from typing import Any


DEFAULT_PATH = "/api/v1/private/order/createOrder"
DEFAULT_BASE_URL = "https://pro.edgex.exchange"


def main() -> int:
    try:
        from Crypto.Hash import keccak
        from edgex_sdk import CreateOrderParams, OrderSide, OrderType, StarkExSigningAdapter, TimeInForce
        from edgex_sdk.internal.async_client import AsyncClient
    except ImportError as exc:
        raise SystemExit(
            "missing edgeX SDK dependencies; run with "
            "`uv run --with edgex-python-sdk --with requests python ...`"
        ) from exc

    for line in sys.stdin:
        if not line.strip():
            continue
        built = build(json.loads(line), keccak, CreateOrderParams, OrderSide, OrderType, StarkExSigningAdapter, TimeInForce, AsyncClient)
        print(compact_json(built), flush=True)
    return 0


def build(
    req: dict[str, Any],
    keccak: Any,
    CreateOrderParams: Any,
    OrderSide: Any,
    OrderType: Any,
    StarkExSigningAdapter: Any,
    TimeInForce: Any,
    AsyncClient: Any,
) -> dict[str, Any]:
    params = dict(req.get("params") or {})

    if req.get("scenario") == "batch":
        raise SystemExit("edgeX builder only supports single orders; no official batch create-order endpoint is documented")
    if req.get("transport") == "websocket":
        raise SystemExit("edgeX builder only supports HTTP order submission; official WebSocket docs do not expose order submission")

    account_id = int(env_or_param(params, "account_id", "EDGEX_ACCOUNT_ID"))
    stark_private_key = strip_0x(env_or_param(params, "stark_private_key", "EDGEX_STARK_PRIVATE_KEY"))
    metadata = params.get("metadata")
    if not isinstance(metadata, dict):
        raise SystemExit("edgeX builder requires params.metadata with contractList and coinList")

    async_client = AsyncClient(
        base_url=str(params.get("base_url", DEFAULT_BASE_URL)),
        account_id=account_id,
        stark_pri_key=stark_private_key,
        signing_adapter=StarkExSigningAdapter(),
    )

    order_type = enum_value(OrderType, str(params.get("type", "LIMIT")))
    side = enum_value(OrderSide, str(params.get("side", "BUY")).upper())
    l2_price = decimal_param(params, "l2_price", default=params.get("price"))
    if order_type == OrderType.MARKET and "l2_price" not in params:
        raise SystemExit("edgeX market orders require params.l2_price so the builder does not fetch quotes")

    order_kwargs: dict[str, Any] = {
        "contract_id": str(params["contract_id"]),
        "price": str(params["price"]),
        "size": str(params["size"]),
        "type": order_type,
        "side": enum_wire(side),
        "client_order_id": client_order_id(params, int(req.get("iteration") or 0)),
        "time_in_force": str(params["time_in_force"]) if params.get("time_in_force") else None,
        "reduce_only": bool(params.get("reduce_only", False)),
    }
    if params.get("expire_time") is not None:
        expire_name = "expire_time" if "expire_time" in signature(CreateOrderParams).parameters else "l2_expire_time"
        order_kwargs[expire_name] = int(params["expire_time"])
    order = CreateOrderParams(**order_kwargs)

    body = build_order_body(async_client, order, metadata, l2_price, OrderType, TimeInForce)
    timestamp = int(params.get("timestamp_ms") or time.time() * 1000)
    method = str(params.get("method", "POST")).upper()
    path = str(params.get("path", DEFAULT_PATH))

    sign_content = async_client._build_signature_content(timestamp, method, path, body, None)
    content_hash = keccak.new(digest_bits=256)
    content_hash.update(sign_content.encode())
    auth_signature = async_client.sign(content_hash.digest())

    return {
        "method": method,
        "headers": {
            "Accept": "application/json",
            "Content-Type": "application/json",
            "X-edgeX-Api-Timestamp": str(timestamp),
            "X-edgeX-Api-Signature": f"{auth_signature.r}{auth_signature.s}",
        },
        "body": compact_json(body),
        "metadata": {
            "builder": "edgex-python-sdk",
            "orders": 1,
            "client_order_id": body["clientOrderId"],
            "l2_nonce": body["l2Nonce"],
            "request_timestamp_ms": timestamp,
        },
    }


def build_order_body(
    async_client: Any,
    params: Any,
    metadata: dict[str, Any],
    l2_price: Decimal,
    OrderType: Any,
    TimeInForce: Any,
) -> dict[str, Any]:
    if not params.time_in_force:
        if enum_wire(params.type) == OrderType.MARKET.value:
            params.time_in_force = TimeInForce.IMMEDIATE_OR_CANCEL.value
        elif enum_wire(params.type) == OrderType.LIMIT.value:
            params.time_in_force = TimeInForce.GOOD_TIL_CANCEL.value

    contract = find_by_id(metadata.get("contractList"), "contractId", params.contract_id)
    if contract is None:
        raise SystemExit(f"contract not found in params.metadata.contractList: {params.contract_id}")

    quote_coin = find_by_id(metadata.get("coinList"), "coinId", contract.get("quoteCoinId"))
    if quote_coin is None:
        raise SystemExit(f"coin not found in params.metadata.coinList: {contract.get('quoteCoinId')}")

    try:
        synthetic_factor = Decimal(hex_to_int(async_client, contract.get("starkExResolution", "0x0")))
        shift_factor = Decimal(hex_to_int(async_client, quote_coin.get("starkExResolution", "0x0")))
        size = Decimal(str(params.size))
    except (InvalidOperation, TypeError, ValueError) as exc:
        raise SystemExit(f"failed to parse edgeX order metadata or size: {exc}") from exc

    value_dm = l2_price * size
    amount_synthetic = int(size * synthetic_factor)
    amount_collateral = int(value_dm * shift_factor)

    fee_rate = Decimal(str(contract.get("defaultTakerFeeRate") or "0.00038"))
    limit_fee = (size * l2_price * fee_rate).quantize(Decimal("1"), rounding=ROUND_CEILING)
    max_amount_fee = int(limit_fee * shift_factor)

    explicit_expire_time = getattr(params, "expire_time", None) or getattr(params, "l2_expire_time", None)
    expire_time = int(explicit_expire_time or (time.time() * 1000 + 24 * 60 * 60 * 1000))
    l2_expire_time = expire_time + 9 * 24 * 60 * 60 * 1000
    l2_expire_hour = l2_expire_time // (60 * 60 * 1000)
    nonce = async_client.calc_nonce(params.client_order_id)

    msg_hash = async_client.calc_limit_order_hash(
        contract.get("starkExSyntheticAssetId", ""),
        quote_coin.get("starkExAssetId", ""),
        quote_coin.get("starkExAssetId", ""),
        enum_wire(params.side) == "BUY",
        amount_synthetic,
        amount_collateral,
        max_amount_fee,
        nonce,
        async_client.get_account_id(),
        l2_expire_hour,
    )
    signature = async_client.sign(msg_hash)

    return {
        "accountId": str(async_client.get_account_id()),
        "contractId": params.contract_id,
        "price": params.price,
        "size": params.size,
        "type": enum_wire(params.type),
        "side": enum_wire(params.side),
        "timeInForce": params.time_in_force,
        "clientOrderId": params.client_order_id,
        "expireTime": str(expire_time),
        "l2Nonce": str(nonce),
        "l2Signature": f"{signature.r}{signature.s}",
        "l2ExpireTime": str(l2_expire_time),
        "l2Value": str(value_dm),
        "l2Size": params.size,
        "l2LimitFee": str(limit_fee),
        "reduceOnly": params.reduce_only,
    }


def find_by_id(values: Any, key: str, wanted: Any) -> dict[str, Any] | None:
    if not isinstance(values, list):
        return None
    wanted = str(wanted)
    for value in values:
        if isinstance(value, dict) and str(value.get(key)) == wanted:
            return value
    return None


def enum_value(enum_type: Any, value: str) -> Any:
    try:
        return enum_type[value]
    except KeyError:
        try:
            return enum_type(value)
        except ValueError as exc:
            allowed = ", ".join(item.name for item in enum_type)
            raise SystemExit(f"invalid {enum_type.__name__} {value!r}; expected one of {allowed}") from exc


def enum_wire(value: Any) -> str:
    return str(getattr(value, "value", value))


def hex_to_int(async_client: Any, value: Any) -> int:
    convert = getattr(async_client, "hex_to_big_integer", None)
    if convert is not None:
        return int(convert(str(value)))
    return int(str(value), 16)


def decimal_param(params: dict[str, Any], key: str, default: Any = None) -> Decimal:
    value = params.get(key, default)
    if value in (None, ""):
        raise SystemExit(f"missing {key}")
    try:
        return Decimal(str(value))
    except InvalidOperation as exc:
        raise SystemExit(f"invalid decimal for {key}: {value!r}") from exc


def client_order_id(params: dict[str, Any], iteration: int) -> str:
    if params.get("client_order_id"):
        return str(params["client_order_id"])
    if params.get("client_order_id_base") is not None:
        return str(int(params["client_order_id_base"]) + iteration)
    return random_client_id()


def random_client_id() -> str:
    return str(int(time.time_ns()))


def env_or_param(params: dict[str, Any], key: str, env_key: str) -> str:
    value = params.get(key) or os.getenv(env_key)
    if value in (None, ""):
        raise SystemExit(f"missing {key}; set params.{key} or {env_key}")
    return str(value)


def strip_0x(value: str) -> str:
    return value[2:] if value.startswith(("0x", "0X")) else value


def compact_json(value: Any) -> str:
    return json.dumps(value, separators=(",", ":"), sort_keys=False)


if __name__ == "__main__":
    raise SystemExit(main())
