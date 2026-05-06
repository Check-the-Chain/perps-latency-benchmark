#!/usr/bin/env python3
"""Build signed GRVT order payloads for perps-bench.

Run through the command builder, for example:

  uv run --with grvt-pysdk --with eth-account \
    python internal/venues/grvt/build_payload.py

The script uses the official Python SDK signing utilities. It expects instrument
metadata in params.instruments so it does not need to fetch metadata during
payload construction.
"""

from __future__ import annotations

import json
import os
import random
import sys
import time
from decimal import Decimal
from typing import Any


def main() -> int:
    try:
        from pysdk.grvt_ccxt_env import GrvtEnv
        from pysdk.grvt_ccxt_utils import (
            GrvtOrder,
            GrvtOrderLeg,
            GrvtSignature,
            OrderMetadata,
            TimeInForce,
            get_order_payload,
            get_order_rpc_payload,
        )
    except ImportError as exc:
        raise SystemExit(
            "missing GRVT SDK dependencies; run with "
            "`uv run --with grvt-pysdk --with eth-account python ...`"
        ) from exc

    for line in sys.stdin:
        if not line.strip():
            continue
        built = build(
            json.loads(line),
            GrvtEnv,
            GrvtOrder,
            GrvtOrderLeg,
            GrvtSignature,
            OrderMetadata,
            TimeInForce,
            get_order_payload,
            get_order_rpc_payload,
        )
        print(compact_json(built), flush=True)
    return 0


def build(
    req: dict[str, Any],
    GrvtEnv: Any,
    GrvtOrder: Any,
    GrvtOrderLeg: Any,
    GrvtSignature: Any,
    OrderMetadata: Any,
    TimeInForce: Any,
    get_order_payload: Any,
    get_order_rpc_payload: Any,
) -> dict[str, Any]:
    params = dict(req.get("params") or {})
    private_key = env_or_param(params, "private_key", "GRVT_PRIVATE_KEY")
    env = GrvtEnv(params.get("env", os.getenv("GRVT_ENV", "prod")))
    instruments = params.get("instruments")
    if not instruments:
        raise SystemExit("GRVT builder requires params.instruments with base_decimals and instrument_hash")

    scenario = req["scenario"]
    batch_size = int(req.get("batch_size") or 1)
    orders = [build_order(params, offset, GrvtOrder, GrvtOrderLeg, GrvtSignature, OrderMetadata, TimeInForce) for offset in range(batch_size if scenario == "batch" else 1)]

    if scenario == "batch":
        payloads = [get_order_payload(order, private_key, env, instruments)["order"] for order in orders]
        body_obj = {"orders": payloads}
        ws_obj = {
            "jsonrpc": "2.0",
            "id": params.get("request_id", time.time_ns()),
            "method": params.get("batch_rpc_method", "v2/bulk_orders"),
            "params": body_obj,
        }
    else:
        body_obj = get_order_payload(orders[0], private_key, env, instruments)
        ws_obj = get_order_rpc_payload(
            orders[0],
            private_key,
            env,
            instruments,
            version=params.get("rpc_version", "v1"),
        )
        ws_obj["id"] = params.get("request_id", time.time_ns())

    return {
        "headers": {"Content-Type": "application/json"},
        "body": compact_json(body_obj),
        "ws_body": compact_json(ws_obj),
        "metadata": {"builder": "grvt-pysdk", "orders": len(orders)},
    }


def build_order(
    params: dict[str, Any],
    offset: int,
    GrvtOrder: Any,
    GrvtOrderLeg: Any,
    GrvtSignature: Any,
    OrderMetadata: Any,
    TimeInForce: Any,
) -> Any:
    nonce = int(params.get("nonce_base") or random.randint(0, 2**31 - 1)) + offset
    expiration = str(int(params.get("expiration_ns") or (time.time_ns() + 5 * 60 * 1_000_000_000)))
    client_order_id = str(int(params.get("client_order_id_base") or (2**63 + random.randint(0, 2**31))) + offset)
    return GrvtOrder(
        sub_account_id=str(env_or_param(params, "sub_account_id", "GRVT_TRADING_ACCOUNT_ID")),
        time_in_force=getattr(TimeInForce, params.get("time_in_force", "GOOD_TILL_TIME")),
        legs=[
            GrvtOrderLeg(
                instrument=params["instrument"],
                size=Decimal(str(params["size"])),
                is_buying_asset=str(params.get("side", "buy")).lower() == "buy",
                limit_price=Decimal(str(params["price"])),
            )
        ],
        signature=GrvtSignature(signer="", r="", s="", v=0, expiration=expiration, nonce=nonce),
        metadata=OrderMetadata(client_order_id=client_order_id),
        is_market=bool(params.get("is_market", False)),
        post_only=bool(params.get("post_only", True)),
        reduce_only=bool(params.get("reduce_only", False)),
    )


def env_or_param(params: dict[str, Any], key: str, env_key: str) -> str:
    value = params.get(key) or os.getenv(env_key)
    if value in (None, ""):
        raise SystemExit(f"missing {key}; set params.{key} or {env_key}")
    return str(value)


def compact_json(value: Any) -> str:
    return json.dumps(value, separators=(",", ":"), sort_keys=False)


if __name__ == "__main__":
    raise SystemExit(main())
