#!/usr/bin/env python3
"""Cleanup Extended benchmark orders outside the measured latency window."""

from __future__ import annotations

import asyncio
import json
import os
import sys
import time
import urllib.parse
import urllib.request
from decimal import Decimal, ROUND_CEILING, ROUND_FLOOR
from pathlib import Path
from typing import Any

sys.path.append(str(Path(__file__).resolve().parents[1]))
from cleanup_common import cleanup_orders_for_venue, cleanup_result, result_orders_for_venue
from build_payload import compact_json, env_or_param, market_model, order_external_id, sdk_config, signed_order


def main() -> int:
    try:
        from x10.config import MAINNET_CONFIG, TESTNET_CONFIG, StarknetDomain
        from x10.core.stark_account import StarkPerpetualAccount
        from x10.models.market import MarketModel
        from x10.models.order import OrderSide, OrderType, SelfTradeProtectionLevel, TimeInForce
        from x10.perpetual.order_object import create_order_object
        from x10.perpetual.trading_client import PerpetualTradingClient
    except ImportError as exc:
        raise SystemExit(
            "missing Extended SDK dependencies; run with "
            "`uv run --with x10-python-trading-starknet python ...`"
        ) from exc

    asyncio.run(serve(
        MAINNET_CONFIG,
        TESTNET_CONFIG,
        StarknetDomain,
        StarkPerpetualAccount,
        MarketModel,
        PerpetualTradingClient,
        OrderSide,
        OrderType,
        SelfTradeProtectionLevel,
        TimeInForce,
        create_order_object,
    ))
    return 0


async def serve(
    MAINNET_CONFIG: Any,
    TESTNET_CONFIG: Any,
    StarknetDomain: Any,
    StarkPerpetualAccount: Any,
    MarketModel: Any,
    PerpetualTradingClient: Any,
    OrderSide: Any,
    OrderType: Any,
    SelfTradeProtectionLevel: Any,
    TimeInForce: Any,
    create_order_object: Any,
) -> None:
    for line in sys.stdin:
        if not line.strip():
            continue
        built = await build(
            json.loads(line),
            MAINNET_CONFIG,
            TESTNET_CONFIG,
            StarknetDomain,
            StarkPerpetualAccount,
            MarketModel,
            PerpetualTradingClient,
            OrderSide,
            OrderType,
            SelfTradeProtectionLevel,
            TimeInForce,
            create_order_object,
        )
        print(compact_json(built), flush=True)


async def build(
    req: dict[str, Any],
    MAINNET_CONFIG: Any,
    TESTNET_CONFIG: Any,
    StarknetDomain: Any,
    StarkPerpetualAccount: Any,
    MarketModel: Any,
    PerpetualTradingClient: Any,
    OrderSide: Any,
    OrderType: Any,
    SelfTradeProtectionLevel: Any,
    TimeInForce: Any,
    create_order_object: Any,
) -> dict[str, Any]:
    params = dict(req.get("params") or {})
    builder_params = dict(params.get("builder_params") or {})
    client = sdk_client(builder_params, MAINNET_CONFIG, TESTNET_CONFIG, StarkPerpetualAccount, PerpetualTradingClient)
    try:
        phase = params.get("phase", "after_sample")
        if phase == "before_run":
            return await before_run(
                client,
                params,
                builder_params,
                StarknetDomain,
                StarkPerpetualAccount,
                MarketModel,
                OrderSide,
                OrderType,
                SelfTradeProtectionLevel,
                TimeInForce,
                create_order_object,
                MAINNET_CONFIG,
                TESTNET_CONFIG,
            )
        if phase == "after_run":
            return await after_run(client, params, builder_params)
        return await after_sample(
            client,
            params,
            builder_params,
            StarknetDomain,
            StarkPerpetualAccount,
            MarketModel,
            OrderSide,
            OrderType,
            SelfTradeProtectionLevel,
            TimeInForce,
            create_order_object,
            MAINNET_CONFIG,
            TESTNET_CONFIG,
        )
    finally:
        await client.close()


def sdk_client(params: dict[str, Any], MAINNET_CONFIG: Any, TESTNET_CONFIG: Any, StarkPerpetualAccount: Any, PerpetualTradingClient: Any) -> Any:
    account = StarkPerpetualAccount(
        vault=env_or_param(params, "vault", "EXTENDED_VAULT"),
        private_key=env_or_param(params, "private_key", "EXTENDED_PRIVATE_KEY"),
        public_key=env_or_param(params, "public_key", "EXTENDED_PUBLIC_KEY"),
        api_key=env_or_param(params, "api_key", "EXTENDED_API_KEY"),
    )
    env = str(params.get("env", os.getenv("EXTENDED_ENV", "mainnet"))).lower()
    return PerpetualTradingClient(TESTNET_CONFIG if env in ("testnet", "sepolia") else MAINNET_CONFIG, account)


async def before_run(
    client: Any,
    params: dict[str, Any],
    builder_params: dict[str, Any],
    StarknetDomain: Any = None,
    StarkPerpetualAccount: Any = None,
    MarketModel: Any = None,
    OrderSide: Any = None,
    OrderType: Any = None,
    SelfTradeProtectionLevel: Any = None,
    TimeInForce: Any = None,
    create_order_object: Any = None,
    MAINNET_CONFIG: Any = None,
    TESTNET_CONFIG: Any = None,
) -> dict[str, Any]:
    position = await position_snapshot(client, builder_params)
    if position and bool(builder_params.get("neutralize_on_fill")):
        neutralize = await neutralize_position(
            client,
            {"run_metadata": {"position": []}},
            builder_params,
            StarknetDomain,
            StarkPerpetualAccount,
            MarketModel,
            OrderSide,
            OrderType,
            SelfTradeProtectionLevel,
            TimeInForce,
            create_order_object,
            MAINNET_CONFIG,
            TESTNET_CONFIG,
        )
        if not neutralize:
            return cleanup_result(True, False, "extended position is open before run and no neutralize order was built", metadata={"position": position})
        ok, description = submit_order_payload(neutralize, builder_params)
        if not ok:
            return cleanup_result(True, False, description, metadata={"position": position, **dict(neutralize.get("metadata") or {})})
        after_position = await wait_position_snapshot(client, builder_params, [])
        if after_position:
            return cleanup_result(
                True,
                False,
                "extended position remained open after startup neutralize",
                metadata={"position_before": position, "position_after": after_position, **dict(neutralize.get("metadata") or {})},
            )
        return cleanup_result(
            True,
            True,
            "flattened extended position before run",
            metadata={"position_before": position, "position_after": after_position, **dict(neutralize.get("metadata") or {})},
        )
    if bool(builder_params.get("cleanup_all_open_orders")):
        orders = await active_market_orders(client, builder_params)
    else:
        orders = await open_cleanup_orders(client, planned_orders(params, builder_params), builder_params)
    if not orders:
        return cleanup_result(False, True, "no stale extended benchmark orders", metadata={"position": position})
    cleanup = await cancel_orders(client, orders, {"position": position})
    return cleanup


async def after_run(client: Any, params: dict[str, Any], builder_params: dict[str, Any]) -> dict[str, Any]:
    refs = result_orders(dict(params.get("result") or {}))
    remaining = await wait_no_open_cleanup_orders(client, refs, builder_params)
    before_position = dict(params.get("run_metadata") or {}).get("position")
    after_position = await wait_position_snapshot(client, builder_params, before_position)
    problems = []
    if remaining:
        problems.append(f"{len(remaining)} extended benchmark orders still open")
    if before_position is not None and before_position != after_position:
        problems.append("extended position changed during run")
    metadata = {"position_before": before_position, "position_after": after_position}
    if problems:
        return cleanup_result(True, False, "; ".join(problems), metadata=metadata)
    return cleanup_result(True, True, "no extended benchmark orders open after run and position unchanged", metadata=metadata)


async def after_sample(
    client: Any,
    params: dict[str, Any],
    builder_params: dict[str, Any],
    StarknetDomain: Any,
    StarkPerpetualAccount: Any,
    MarketModel: Any,
    OrderSide: Any,
    OrderType: Any,
    SelfTradeProtectionLevel: Any,
    TimeInForce: Any,
    create_order_object: Any,
    MAINNET_CONFIG: Any,
    TESTNET_CONFIG: Any,
) -> dict[str, Any]:
    orders = cleanup_orders(dict(params.get("metadata") or {}))
    if not orders:
        return cleanup_result(False, True, "no extended cleanup_orders")
    remaining = await wait_open_cleanup_orders(client, orders, builder_params)
    if remaining:
        return await cancel_orders(client, remaining, {})
    close = await neutralize_confirmed_benchmark_order(
        params,
        builder_params,
        StarknetDomain,
        StarkPerpetualAccount,
        MarketModel,
        OrderSide,
        OrderType,
        SelfTradeProtectionLevel,
        TimeInForce,
        create_order_object,
        MAINNET_CONFIG,
        TESTNET_CONFIG,
    )
    if close:
        return close
    neutralize = await neutralize_position(
        client,
        params,
        builder_params,
        StarknetDomain,
        StarkPerpetualAccount,
        MarketModel,
        OrderSide,
        OrderType,
        SelfTradeProtectionLevel,
        TimeInForce,
        create_order_object,
        MAINNET_CONFIG,
        TESTNET_CONFIG,
    )
    if neutralize:
        return neutralize
    return cleanup_result(False, True, "no extended cleanup action needed")


async def neutralize_confirmed_benchmark_order(
    params: dict[str, Any],
    builder_params: dict[str, Any],
    StarknetDomain: Any,
    StarkPerpetualAccount: Any,
    MarketModel: Any,
    OrderSide: Any,
    OrderType: Any,
    SelfTradeProtectionLevel: Any,
    TimeInForce: Any,
    create_order_object: Any,
    MAINNET_CONFIG: Any,
    TESTNET_CONFIG: Any,
) -> dict[str, Any] | None:
    if not bool(builder_params.get("neutralize_on_fill")) or not is_fill_likely(builder_params):
        return None
    sample = dict(params.get("sample") or {})
    refs = cleanup_orders(dict(params.get("metadata") or {}))
    entry_id = str(refs[0].get("external_id") or "") if refs else ""
    market = str(builder_params.get("market", "BTC-USD"))
    side = str(builder_params.get("side", "buy")).lower()
    close_is_buy = side == "sell"
    size = Decimal(str(builder_params.get("size") or "0"))
    if size <= 0:
        return None
    payload = await reduce_only_payload(
        builder_params,
        StarknetDomain,
        StarkPerpetualAccount,
        MarketModel,
        OrderSide,
        OrderType,
        SelfTradeProtectionLevel,
        TimeInForce,
        create_order_object,
        MAINNET_CONFIG,
        TESTNET_CONFIG,
        market,
        close_is_buy,
        size,
        "neutralize_confirmed_fill",
    )
    payload["metadata"]["reconciliation"] = {
        "entry_external_id": entry_id,
        "entry_side": side,
        "entry_size": str(size),
        "sample_completed_at": sample.get("completed_at"),
    }
    return payload


async def cancel_orders(client: Any, orders: list[dict[str, Any]], reconciliation: dict[str, Any]) -> dict[str, Any]:
    if not orders:
        return cleanup_result(False, True, "no extended orders to cancel", metadata=reconciliation)
    external_ids = [str(order["external_id"]) for order in orders if order.get("external_id")]
    if not external_ids:
        return cleanup_result(False, True, "no extended external IDs to cancel", metadata=reconciliation)
    if len(external_ids) == 1:
        response = await client.orders.cancel_order_by_external_id(external_ids[0])
    else:
        response = await client.orders.mass_cancel(external_order_ids=external_ids)
    ok, description = response_ok(response)
    return cleanup_result(True, ok, description or "cancel extended benchmark orders by external ID", metadata=reconciliation)


async def active_market_orders(client: Any, params: dict[str, Any]) -> list[dict[str, Any]]:
    market = str(params.get("market", "BTC-USD"))
    response = await client.account.get_open_orders(market_names=[market])
    return [
        {"venue": "extended", "market": market, "external_id": str(get_field(order, "external_id", "externalId"))}
        for order in response_data(response)
        if get_field(order, "external_id", "externalId") not in (None, "")
    ]


async def open_cleanup_orders(client: Any, refs: list[dict[str, Any]], params: dict[str, Any]) -> list[dict[str, Any]]:
    if not refs:
        return []
    open_ids: set[str] = set()
    for market in sorted({str(ref.get("market") or params.get("market", "BTC-USD")) for ref in refs}):
        response = await client.account.get_open_orders(market_names=[market])
        open_ids.update(
            str(get_field(order, "external_id", "externalId"))
            for order in response_data(response)
            if get_field(order, "external_id", "externalId") not in (None, "")
        )
    return [ref for ref in refs if str(ref.get("external_id")) in open_ids]


async def wait_open_cleanup_orders(client: Any, refs: list[dict[str, Any]], params: dict[str, Any]) -> list[dict[str, Any]]:
    attempts = max(1, int(params.get("cleanup_poll_attempts", 5)))
    interval = max(0, int(params.get("cleanup_poll_interval_ms", 250))) / 1000
    for attempt in range(attempts):
        remaining = await open_cleanup_orders(client, refs, params)
        if remaining or attempt == attempts - 1:
            return remaining
        await asyncio.sleep(interval)
    return []


async def wait_no_open_cleanup_orders(client: Any, refs: list[dict[str, Any]], params: dict[str, Any]) -> list[dict[str, Any]]:
    attempts = max(1, int(params.get("reconciliation_poll_attempts", 5)))
    interval = max(0, int(params.get("reconciliation_poll_interval_ms", 250))) / 1000
    remaining = []
    for attempt in range(attempts):
        remaining = await open_cleanup_orders(client, refs, params)
        if not remaining or attempt == attempts - 1:
            return remaining
        await asyncio.sleep(interval)
    return remaining


def planned_orders(params: dict[str, Any], builder_params: dict[str, Any]) -> list[dict[str, Any]]:
    run = dict(params.get("run") or {})
    run_id = run.get("run_id")
    if not run_id:
        return []
    total = int(run.get("iterations") or 0) + int(run.get("warmups") or 0)
    warmups = int(run.get("warmups") or 0)
    order_params = dict(builder_params)
    order_params["run_id"] = run_id
    refs = []
    for index in range(total):
        req = {"iteration": index - warmups}
        refs.append({
            "venue": "extended",
            "market": str(order_params.get("market", "BTC-USD")),
            "external_id": order_external_id(order_params, req),
        })
    return refs


def result_orders(result: dict[str, Any]) -> list[dict[str, Any]]:
    return result_orders_for_venue(result, "extended")


def cleanup_orders(metadata: dict[str, Any]) -> list[dict[str, Any]]:
    return cleanup_orders_for_venue(metadata, "extended")


async def position_snapshot(client: Any, params: dict[str, Any]) -> list[dict[str, str]]:
    response = await client.account.get_positions(market_names=[str(params.get("market", "BTC-USD"))])
    positions = []
    for position in response_data(response):
        size = Decimal(str(get_field(position, "size") or "0"))
        if size == 0:
            continue
        side = str(get_field(position, "side") or "")
        positions.append({
            "market": str(get_field(position, "market") or ""),
            "side": side,
            "size": str(size),
        })
    return sorted(positions, key=lambda item: (item["market"], item["side"]))


async def wait_position_snapshot(client: Any, params: dict[str, Any], target: Any) -> list[dict[str, str]]:
    attempts = max(1, int(params.get("position_reconciliation_poll_attempts", 20)))
    interval = max(0, int(params.get("position_reconciliation_poll_interval_ms", 500))) / 1000
    current = await position_snapshot(client, params)
    for attempt in range(attempts):
        if target is None or current == target or attempt == attempts - 1:
            return current
        await asyncio.sleep(interval)
        current = await position_snapshot(client, params)
    return current


async def neutralize_position(
    client: Any,
    params: dict[str, Any],
    builder_params: dict[str, Any],
    StarknetDomain: Any,
    StarkPerpetualAccount: Any,
    MarketModel: Any,
    OrderSide: Any,
    OrderType: Any,
    SelfTradeProtectionLevel: Any,
    TimeInForce: Any,
    create_order_object: Any,
    MAINNET_CONFIG: Any,
    TESTNET_CONFIG: Any,
) -> dict[str, Any] | None:
    if not bool(builder_params.get("neutralize_on_fill")):
        return None
    before_position = dict(params.get("run_metadata") or {}).get("position")
    if before_position is None:
        return None
    market = str(builder_params.get("market", "BTC-USD"))
    after_position = await wait_for_position_delta(client, builder_params, before_position, market)
    delta = signed_position_size(after_position, market) - signed_position_size(before_position, market)
    if delta == 0:
        return None

    is_buy = delta < 0
    reconciliation = {"position_before": before_position, "position_after": after_position, "delta": str(delta)}
    payload = await reduce_only_payload(
        builder_params,
        StarknetDomain,
        StarkPerpetualAccount,
        MarketModel,
        OrderSide,
        OrderType,
        SelfTradeProtectionLevel,
        TimeInForce,
        create_order_object,
        MAINNET_CONFIG,
        TESTNET_CONFIG,
        market,
        is_buy,
        abs(delta),
        "neutralize_position",
    )
    payload["metadata"]["reconciliation"] = reconciliation
    return payload


async def reduce_only_payload(
    builder_params: dict[str, Any],
    StarknetDomain: Any,
    StarkPerpetualAccount: Any,
    MarketModel: Any,
    OrderSide: Any,
    OrderType: Any,
    SelfTradeProtectionLevel: Any,
    TimeInForce: Any,
    create_order_object: Any,
    MAINNET_CONFIG: Any,
    TESTNET_CONFIG: Any,
    market: str,
    is_buy: bool,
    size: Decimal,
    cleanup: str,
) -> dict[str, Any]:
    account = StarkPerpetualAccount(
        vault=env_or_param(builder_params, "vault", "EXTENDED_VAULT"),
        private_key=env_or_param(builder_params, "private_key", "EXTENDED_PRIVATE_KEY"),
        public_key=env_or_param(builder_params, "public_key", "EXTENDED_PUBLIC_KEY"),
        api_key=env_or_param(builder_params, "api_key", "EXTENDED_API_KEY"),
    )
    order_params = dict(builder_params)
    external_id = "pb-neutralize-" + format(time.time_ns() & ((1 << 63) - 1), "x")
    order_params.update({
        "market": market,
        "side": "buy" if is_buy else "sell",
        "size": str(size),
        "price": str(await neutralize_price(builder_params, is_buy)),
        "order_type": "market",
        "post_only": False,
        "time_in_force": str(builder_params.get("neutralize_time_in_force", "IOC")).upper(),
        "fee": str(builder_params.get("neutralize_taker_fee", builder_params.get("taker_fee", builder_params.get("fee", "0.0002")))),
        "reduce_only": True,
        "expiration_seconds": int(builder_params.get("neutralize_expiration_seconds", 60)),
        "external_id": external_id,
    })
    order = signed_order(
        order_params,
        {"iteration": int(time.time_ns() % (2**31 - 1))},
        account,
        market_model(order_params, MarketModel),
        sdk_config(order_params, MAINNET_CONFIG, TESTNET_CONFIG, StarknetDomain),
        OrderSide,
        OrderType,
        SelfTradeProtectionLevel,
        TimeInForce,
        create_order_object,
        int(time.time_ns() % (2**31 - 1)),
        0,
    )
    return {
        "method": "POST",
        "headers": {
            "Content-Type": "application/json",
            "User-Agent": builder_params.get("user_agent", "perps-latency-benchmark"),
            "X-Api-Key": account.api_key,
        },
        "body": compact_json(order["body"]),
        "metadata": {
            "cleanup": cleanup,
            "cleanup_orders": [{"venue": "extended", "market": market, "external_id": external_id, "side": order_params["side"], "size": str(size)}],
        },
    }


def is_fill_likely(params: dict[str, Any]) -> bool:
    order_type = str(params.get("order_type") or "").lower()
    tif = str(params.get("time_in_force") or params.get("tif") or "").lower()
    post_only = bool(params.get("post_only", False))
    return not post_only and (order_type == "market" or tif in ("ioc", "fok", "immediate_or_cancel", "fill_or_kill"))


def signed_position_size(positions: list[dict[str, Any]], market: str) -> Decimal:
    total = Decimal("0")
    for position in positions or []:
        if str(position.get("market", "")) != market:
            continue
        size = Decimal(str(position.get("size", "0") or "0"))
        side = str(position.get("side", "")).upper()
        total += -size if side == "SHORT" else size
    return total


async def neutralize_price(builder_params: dict[str, Any], is_buy: bool) -> Decimal:
    if builder_params.get("neutralize_price") is not None:
        return Decimal(str(builder_params["neutralize_price"]))
    orderbook = public_json(builder_params, f"/api/v1/info/markets/{urllib.parse.quote(str(builder_params.get('market', 'BTC-USD')), safe='')}/orderbook")
    price = neutralize_price_from_orderbook(orderbook, builder_params, is_buy)
    if price is not None:
        return price
    return fallback_neutralize_price(builder_params, is_buy)


def neutralize_price_from_orderbook(orderbook: dict[str, Any], builder_params: dict[str, Any], is_buy: bool) -> Decimal | None:
    data = orderbook.get("data")
    if not isinstance(data, dict):
        return None
    side = "ask" if is_buy else "bid"
    levels = data.get(side)
    if not isinstance(levels, list) or not levels:
        return None
    top = levels[0]
    if not isinstance(top, dict) or top.get("price") in (None, ""):
        return None
    buffer_bps = Decimal(str(builder_params.get("neutralize_price_buffer_bps", "150")))
    multiplier = Decimal("1") + buffer_bps / Decimal("10000")
    tick = Decimal(str(builder_params.get("min_price_change", "0.1")))
    top_price = Decimal(str(top["price"]))
    if is_buy:
        return (top_price * multiplier).quantize(tick, rounding=ROUND_CEILING)
    return (top_price / multiplier).quantize(tick, rounding=ROUND_FLOOR)


def fallback_neutralize_price(builder_params: dict[str, Any], is_buy: bool) -> Decimal:
    price = Decimal(str(builder_params["price"]))
    slippage_bps = Decimal(str(builder_params.get("neutralize_slippage_bps", "500")))
    multiplier = Decimal("1") + slippage_bps / Decimal("10000")
    tick = Decimal(str(builder_params.get("min_price_change", "0.1")))
    if is_buy:
        return (price * multiplier).quantize(tick, rounding=ROUND_CEILING)
    return (price / multiplier).quantize(tick, rounding=ROUND_FLOOR)


def public_json(builder_params: dict[str, Any], path: str) -> dict[str, Any]:
    base_url = str(builder_params.get("base_url", "https://api.starknet.extended.exchange")).rstrip("/")
    timeout = float(builder_params.get("market_data_timeout_seconds", 5))
    with urllib.request.urlopen(base_url + path, timeout=timeout) as response:
        return json.loads(response.read().decode())


def submit_order_payload(payload: dict[str, Any], builder_params: dict[str, Any]) -> tuple[bool, str]:
    base_url = str(builder_params.get("base_url", "https://api.starknet.extended.exchange")).rstrip("/")
    timeout = float(builder_params.get("cleanup_submit_timeout_seconds", 10))
    request = urllib.request.Request(
        base_url + "/api/v1/user/order",
        data=str(payload["body"]).encode(),
        headers=dict(payload.get("headers") or {}),
        method=str(payload.get("method") or "POST"),
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            body = response.read().decode(errors="replace")
            decoded = json.loads(body) if body else {"status": "OK"}
    except urllib.error.HTTPError as exc:
        body = exc.read().decode(errors="replace")
        try:
            decoded = json.loads(body)
        except json.JSONDecodeError:
            return False, body or str(exc)
    except Exception as exc:
        return False, str(exc)
    return response_ok(decoded)


def response_ok(response: Any) -> tuple[bool, str]:
    status = str(get_field(response, "status") or "").upper()
    error = get_field(response, "error")
    if status.endswith("OK") or status == "OK":
        return True, ""
    if error not in (None, ""):
        return False, str(error)
    return False, status or "extended API response was not OK"


def response_data(response: Any) -> list[Any]:
    ok, description = response_ok(response)
    if not ok:
        raise SystemExit(description)
    data = get_field(response, "data")
    if data is None:
        return []
    if isinstance(data, list):
        return data
    return [data]


def get_field(value: Any, *names: str) -> Any:
    if isinstance(value, dict):
        for name in names:
            if name in value:
                return value[name]
    for name in names:
        if hasattr(value, name):
            return getattr(value, name)
    return None


async def wait_for_position_delta(client: Any, params: dict[str, Any], before_position: Any, market: str) -> list[dict[str, str]]:
    attempts = max(1, int(params.get("position_reconciliation_poll_attempts", 20)))
    interval = max(0, int(params.get("position_reconciliation_poll_interval_ms", 500))) / 1000
    before_size = signed_position_size(before_position, market)
    current: list[dict[str, str]] = []
    for attempt in range(attempts):
        current = await position_snapshot(client, params)
        if signed_position_size(current, market) != before_size or attempt == attempts - 1:
            return current
        await asyncio.sleep(interval)
    return current


def compact_json(value: Any) -> str:
    return json.dumps(value, separators=(",", ":"), sort_keys=False)


if __name__ == "__main__":
    raise SystemExit(main())
