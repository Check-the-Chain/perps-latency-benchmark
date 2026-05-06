#!/usr/bin/env python3
"""Build signed Extended Exchange order payloads for perps-bench.

Run through the command builder, for example:

  uv run --with x10-python-trading-starknet \
    python internal/venues/extended/build_payload.py

The script uses Extended's official Python SDK to create the signed order
object and emits the REST request body. It does not submit the order.
"""

from __future__ import annotations

import json
import os
import sys
import time
import hashlib
from datetime import datetime, timedelta, timezone
from decimal import Decimal
from typing import Any


def main() -> int:
    try:
        from x10.config import MAINNET_CONFIG, TESTNET_CONFIG, StarknetDomain
        from x10.core.stark_account import StarkPerpetualAccount
        from x10.models.market import MarketModel
        from x10.models.order import OrderSide, OrderType, SelfTradeProtectionLevel, TimeInForce
        from x10.perpetual.order_object import create_order_object
    except ImportError as exc:
        raise SystemExit(
            "missing Extended SDK dependencies; run with "
            "`uv run --with x10-python-trading-starknet python ...`"
        ) from exc

    for line in sys.stdin:
        if not line.strip():
            continue
        built = build(
            json.loads(line),
            MAINNET_CONFIG,
            TESTNET_CONFIG,
            StarknetDomain,
            StarkPerpetualAccount,
            MarketModel,
            OrderSide,
            OrderType,
            SelfTradeProtectionLevel,
            TimeInForce,
            create_order_object,
        )
        print(compact_json(built), flush=True)
    return 0


def build(
    req: dict[str, Any],
    MAINNET_CONFIG: Any,
    TESTNET_CONFIG: Any,
    StarknetDomain: Any,
    StarkPerpetualAccount: Any,
    MarketModel: Any,
    OrderSide: Any,
    OrderType: Any,
    SelfTradeProtectionLevel: Any,
    TimeInForce: Any,
    create_order_object: Any,
) -> dict[str, Any]:
    params = dict(req.get("params") or {})
    scenario = req["scenario"]

    account = StarkPerpetualAccount(
        vault=env_or_param(params, "vault", "EXTENDED_VAULT"),
        private_key=env_or_param(params, "private_key", "EXTENDED_PRIVATE_KEY"),
        public_key=env_or_param(params, "public_key", "EXTENDED_PUBLIC_KEY"),
        api_key=env_or_param(params, "api_key", "EXTENDED_API_KEY"),
    )
    config = sdk_config(params, MAINNET_CONFIG, TESTNET_CONFIG, StarknetDomain)
    market = market_model(params, MarketModel)
    batch_size = int(req.get("batch_size") or 1)
    order_count = batch_size if scenario == "batch" else 1
    nonce_base = int(params.get("nonce_base") or (time.time_ns() % (2**31 - 1))) + int(req["iteration"])
    order_type = str(params.get("order_type", "limit")).lower()
    tif = str(params.get("time_in_force", "GTT")).upper()

    orders = [
        signed_order(
            params,
            req,
            account,
            market,
            config,
            OrderSide,
            OrderType,
            SelfTradeProtectionLevel,
            TimeInForce,
            create_order_object,
            nonce_base + offset,
            offset,
        )
        for offset in range(order_count)
    ]
    external_ids = [order["external_id"] for order in orders]
    cleanup_orders = [
        {"venue": "extended", "market": str(params.get("market", "BTC-USD")), "external_id": external_id}
        for external_id in external_ids
    ]
    headers = {
        "Content-Type": "application/json",
        "User-Agent": params.get("user_agent", "perps-latency-benchmark"),
        "X-Api-Key": account.api_key,
    }
    metadata = {
        "builder": "x10-python-trading-starknet",
        "nonce": nonce_base,
        "run_id": params.get("run_id"),
        "order_type": benchmark_order_type(order_type, tif, bool(params.get("post_only", True))),
        "time_in_force": tif,
        "speed_bump_ns": 0,
        "speed_bump_ms": 0,
        "speed_bump_source": "extended official API docs do not document a fixed order-entry speed bump",
        "cleanup_orders": cleanup_orders,
        "confirmation": {
            "venue": "extended",
            "ws_url": params.get("ws_url", "wss://api.starknet.extended.exchange/stream.extended.exchange/v1/account"),
            "api_key": account.api_key,
            "external_ids": external_ids,
            "order_type": benchmark_order_type(order_type, tif, bool(params.get("post_only", True))),
        },
    }
    if scenario == "batch":
        metadata["orders"] = order_count
        metadata["submission_model"] = "parallel_http_single_orders"
        metadata["native_batch_endpoint"] = False
        metadata["batch_note"] = "Extended has no documented batch order endpoint; benchmark sends signed single orders concurrently."
        return {
            "headers": headers,
            "parallel_requests": [
                {
                    "headers": headers,
                    "body": compact_json(order["body"]),
                }
                for order in orders
            ],
            "metadata": metadata,
        }

    return {
        "headers": headers,
        "body": compact_json(orders[0]["body"]),
        "metadata": metadata,
    }


def signed_order(
    params: dict[str, Any],
    req: dict[str, Any],
    account: Any,
    market: Any,
    config: Any,
    OrderSide: Any,
    OrderType: Any,
    SelfTradeProtectionLevel: Any,
    TimeInForce: Any,
    create_order_object: Any,
    nonce: int,
    offset: int,
) -> dict[str, Any]:
    external_id = order_external_id(params, req, offset)
    order = create_order_object(
        account=account,
        market=market,
        amount_of_synthetic=Decimal(str(params["size"])),
        price=price_for_offset(params, offset),
        side=enum_value(OrderSide, params.get("side", "buy")),
        starknet_domain=config.signing.starknet_domain,
        order_type=enum_value(OrderType, params.get("order_type", "limit")),
        post_only=bool(params.get("post_only", True)),
        previous_order_external_id=optional_str(params.get("previous_order_external_id") or params.get("cancel_id")),
        expire_time=expiration_time(params),
        order_external_id=external_id,
        time_in_force=enum_value(TimeInForce, params.get("time_in_force", "GTT")),
        self_trade_protection_level=enum_value(
            SelfTradeProtectionLevel,
            params.get("self_trade_protection_level", "ACCOUNT"),
        ),
        nonce=nonce,
        taker_fee=optional_decimal(params.get("taker_fee") or params.get("fee")),
        builder_fee=optional_decimal(params.get("builder_fee")),
        builder_id=optional_int(params.get("builder_id")),
        reduce_only=bool(params.get("reduce_only", False)),
    )
    return {"body": order.to_api_request_json(exclude_none=True), "external_id": external_id}


def price_for_offset(params: dict[str, Any], offset: int) -> Decimal:
    base = Decimal(str(params["price"]))
    if offset <= 0:
        return base
    if params.get("price_step_bps") is not None:
        step = base * Decimal(str(params["price_step_bps"])) / Decimal("10000")
    else:
        step = Decimal(str(params.get("price_step", "0")))
    if step == 0:
        return base
    side = str(params.get("side", "buy")).lower()
    return base - (step * offset) if side == "buy" else base + (step * offset)


def sdk_config(params: dict[str, Any], mainnet_config: Any, testnet_config: Any, StarknetDomain: Any) -> Any:
    env = str(params.get("env", os.getenv("EXTENDED_ENV", "mainnet"))).lower()
    config = testnet_config if env in ("testnet", "sepolia") else mainnet_config

    domain = params.get("starknet_domain")
    if domain:
        return replace_starknet_domain(
            config,
            StarknetDomain(
                name=str(domain["name"]),
                version=str(domain["version"]),
                chain_id=str(domain["chain_id"]),
                revision=str(domain["revision"]),
            ),
        )
    return config


def replace_starknet_domain(config: Any, starknet_domain: Any) -> Any:
    from dataclasses import replace

    return replace(config, signing=replace(config.signing, starknet_domain=starknet_domain))


def market_model(params: dict[str, Any], MarketModel: Any) -> Any:
    raw_market = params.get("market_model")
    if raw_market is None and isinstance(params.get("market"), dict):
        raw_market = params["market"]
    if raw_market is not None:
        return MarketModel.model_validate(raw_market)

    l2_config = params.get("l2_config") or {}
    required_l2 = ("synthetic_id", "synthetic_resolution", "collateral_id", "collateral_resolution")
    missing = [key for key in required_l2 if l2_config.get(key) in (None, "")]
    if missing:
        raise SystemExit(
            "Extended builder requires params.market_model or params.l2_config with "
            + ", ".join(required_l2)
        )

    market_name = str(params.get("market_name") or params.get("market") or "BTC-USD")
    return MarketModel.model_validate(
        {
            "name": market_name,
            "assetName": params.get("asset_name", market_name.split("-")[0]),
            "assetPrecision": int(params.get("asset_precision", 8)),
            "collateralAssetName": params.get("collateral_asset_name", "USD"),
            "collateralAssetPrecision": int(params.get("collateral_asset_precision", 6)),
            "active": True,
            "marketStats": default_market_stats(params),
            "tradingConfig": default_trading_config(params),
            "l2Config": {
                "type": l2_config.get("type", "STARKX"),
                "collateralId": l2_config["collateral_id"],
                "collateralResolution": int(l2_config["collateral_resolution"]),
                "syntheticId": l2_config["synthetic_id"],
                "syntheticResolution": int(l2_config["synthetic_resolution"]),
            },
        }
    )


def default_market_stats(params: dict[str, Any]) -> dict[str, Any]:
    price = str(params.get("price", "1"))
    return {
        "dailyVolume": "0",
        "dailyVolumeBase": "0",
        "dailyPriceChange": "0",
        "dailyLow": price,
        "dailyHigh": price,
        "lastPrice": price,
        "askPrice": price,
        "bidPrice": price,
        "markPrice": price,
        "indexPrice": price,
        "fundingRate": "0",
        "nextFundingRate": 0,
        "openInterest": "0",
        "openInterestBase": "0",
    }


def default_trading_config(params: dict[str, Any]) -> dict[str, Any]:
    return {
        "minOrderSize": str(params.get("min_order_size", "0.0001")),
        "minOrderSizeChange": str(params.get("min_order_size_change", "0.0001")),
        "minPriceChange": str(params.get("min_price_change", "0.1")),
        "maxMarketOrderValue": str(params.get("max_market_order_value", "10000000")),
        "maxLimitOrderValue": str(params.get("max_limit_order_value", "10000000")),
        "maxPositionValue": str(params.get("max_position_value", "10000000")),
        "maxLeverage": str(params.get("max_leverage", "20")),
        "maxNumOrders": int(params.get("max_num_orders", 1000)),
        "limitPriceCap": str(params.get("limit_price_cap", "0.05")),
        "limitPriceFloor": str(params.get("limit_price_floor", "0.05")),
        "riskFactorConfig": params.get(
            "risk_factor_config",
            [{"upperBound": "10000000", "riskFactor": "0.05"}],
        ),
    }


def expiration_time(params: dict[str, Any]) -> datetime:
    millis = params.get("expiry_epoch_millis") or params.get("expiration_ms")
    if millis is not None:
        return datetime.fromtimestamp(int(millis) / 1000, tz=timezone.utc)
    seconds = int(params.get("expiration_seconds", 3600))
    return datetime.now(tz=timezone.utc) + timedelta(seconds=seconds)


def order_external_id(params: dict[str, Any], req: dict[str, Any], offset: int = 0) -> str:
    for key in ("order_external_id", "external_id", "client_order_id"):
        value = params.get(key)
        if value not in (None, ""):
            raw = str(value)
            if offset == 0:
                return raw
            return (raw[:58] + f"-{offset}")[:64]
    run_id = params.get("run_id")
    if run_id and is_fill_likely(params):
        seed = f"{run_id}:{req.get('iteration', 0)}:{params.get('market', 'BTC-USD')}:{params.get('side', 'buy')}:{offset}:{time.time_ns()}".encode()
        return "pb-" + hashlib.blake2b(seed, digest_size=16).hexdigest()
    if run_id:
        seed = f"{run_id}:{req.get('iteration', 0)}:{params.get('market', 'BTC-USD')}:{params.get('side', 'buy')}:{offset}".encode()
        return "pb-" + hashlib.blake2b(seed, digest_size=16).hexdigest()
    return "pb-" + format(time.time_ns() & ((1 << 63) - 1), "x")


def is_fill_likely(params: dict[str, Any]) -> bool:
    order_type = str(params.get("order_type", "limit")).lower()
    tif = str(params.get("time_in_force", "GTT")).upper()
    return order_type == "market" or tif in ("IOC", "FOK")


def benchmark_order_type(order_type: str, time_in_force: str, post_only: bool) -> str:
    if order_type == "market":
        return "market"
    if time_in_force in ("IOC", "FOK"):
        return time_in_force.lower()
    if post_only:
        return "post_only"
    return order_type or "limit"


def enum_value(enum_cls: Any, value: Any) -> Any:
    return getattr(enum_cls, str(value).upper())


def optional_decimal(value: Any) -> Decimal | None:
    if value in (None, ""):
        return None
    return Decimal(str(value))


def optional_int(value: Any) -> int | None:
    if value in (None, ""):
        return None
    return int(value)


def optional_str(value: Any) -> str | None:
    if value in (None, ""):
        return None
    return str(value)


def env_or_param(params: dict[str, Any], key: str, env_key: str) -> str:
    value = params.get(key) or os.getenv(env_key)
    if value in (None, ""):
        raise SystemExit(f"missing {key}; set params.{key} or {env_key}")
    return str(value)


def compact_json(value: Any) -> str:
    return json.dumps(value, separators=(",", ":"), sort_keys=False)


if __name__ == "__main__":
    raise SystemExit(main())
