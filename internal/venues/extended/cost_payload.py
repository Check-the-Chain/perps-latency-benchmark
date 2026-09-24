#!/usr/bin/env python3
"""Fetch Extended exact benchmark costs and account balance snapshots."""

from __future__ import annotations

import json
import os
import sys
import time
import urllib.parse
import urllib.request
from datetime import datetime, timezone
from decimal import Decimal
from pathlib import Path
from typing import Any

sys.path.append(str(Path(__file__).resolve().parents[1]))
from cost_common import order_ref_values


def main() -> int:
    for line in sys.stdin:
        if not line.strip():
            continue
        print(compact_json(build(json.loads(line))), flush=True)
    return 0


def build(req: dict[str, Any]) -> dict[str, Any]:
    params = dict(req.get("params") or {})
    builder_params = dict(params.get("builder_params") or {})
    phase = str(params.get("phase") or "")
    if phase == "balance":
        return {"metadata": {"balance": balance_snapshot(builder_params)}}
    if phase == "sample_cost":
        return {"metadata": {"cost": sample_cost(params, builder_params)}}
    raise SystemExit(f"unsupported extended cost phase {phase!r}")


def balance_snapshot(params: dict[str, Any]) -> dict[str, Any]:
    body = private_json(params, "/api/v1/user/balance")
    data = body.get("data") or {}
    balance = number(data.get("balance"))
    equity = number(data.get("equity"))
    return {
        "venue": "extended",
        "currency": str(data.get("collateralName") or "USD"),
        "balance_usd": equity,
        "equity_usd": equity,
        "unrealized_usd": number(data.get("unrealisedPnl")),
        "captured_at": utc_now(),
        "metadata": {
            "wallet_balance_usd": str(balance),
            "available_for_trade": str(data.get("availableForTrade") or ""),
            "available_for_withdrawal": str(data.get("availableForWithdrawal") or ""),
            "updated_time": data.get("updatedTime"),
        },
    }


def sample_cost(params: dict[str, Any], builder_params: dict[str, Any]) -> dict[str, Any]:
    sample = dict(params.get("sample") or {})
    before = dict(params.get("balance_before") or {})
    market = str(builder_params.get("market", "BTC-USD"))
    entry_id = first_value(order_ref_values(sample, "order_refs", "external_id", "externalId", "externalOrderId"))
    exit_id = first_value(order_ref_values(sample, "closeout_order_refs", "external_id", "externalId", "externalOrderId"))
    trades = wait_trades(builder_params, market, {entry_id, exit_id})

    entry = aggregate_trades(trades, entry_id)
    exit = aggregate_trades(trades, exit_id)
    if not exit and entry:
        exit = aggregate_neutralize_after(trades, entry["last_created_time"])
        exit_id = exit.get("order_id", "") if exit else exit_id

    if not entry or not exit:
        after = balance_snapshot(builder_params)
        cost = base_cost(sample, before, after)
        cost.update({
            "entry_order_id": entry_id or "",
            "exit_order_id": exit_id or "",
            "balance_before": before,
            "balance_after": after,
            "balance_before_usd": number(before.get("balance_usd")),
            "balance_after_usd": number(after.get("balance_usd")),
            "balance_delta_usd": number(after.get("balance_usd")) - number(before.get("balance_usd")),
        })
        cost["clean"] = False
        cost["quality_reason"] = "missing entry or exit fill"
        return cost

    return build_with_reconciled_balance(
        sample,
        before,
        builder_params,
        entry_id,
        exit_id,
        entry,
        exit,
    )


def build_with_reconciled_balance(
    sample: dict[str, Any],
    before: dict[str, Any],
    builder_params: dict[str, Any],
    entry_id: str,
    exit_id: str,
    entry: dict[str, Any],
    exit: dict[str, Any],
) -> dict[str, Any]:
    best: dict[str, Any] | None = None
    best_abs_diff: Decimal | None = None
    attempts = max(1, int(builder_params.get("cost_balance_poll_attempts", builder_params.get("cost_poll_attempts", 12))))
    interval = max(0, int(builder_params.get("cost_balance_poll_interval_ms", builder_params.get("cost_poll_interval_ms", 500)))) / 1000
    for attempt in range(attempts):
        cost = build_cost(sample, before, balance_snapshot(builder_params), entry_id, exit_id, entry, exit, attempt + 1)
        diff = abs(Decimal(str(cost["reconciliation_diff_usd"])))
        if best is None or best_abs_diff is None or diff < best_abs_diff:
            best = cost
            best_abs_diff = diff
        if cost.get("clean"):
            return cost
        if attempt != attempts - 1:
            time.sleep(interval)
    return best or {}


def build_cost(
    sample: dict[str, Any],
    before: dict[str, Any],
    after: dict[str, Any],
    entry_id: str,
    exit_id: str,
    entry: dict[str, Any],
    exit: dict[str, Any],
    balance_poll_attempt: int,
) -> dict[str, Any]:
    cost = base_cost(sample, before, after)
    cost.update({
        "entry_order_id": entry_id or "",
        "exit_order_id": exit_id or "",
        "balance_before": before,
        "balance_after": after,
        "balance_before_usd": number(before.get("balance_usd")),
        "balance_after_usd": number(after.get("balance_usd")),
        "balance_delta_usd": number(after.get("balance_usd")) - number(before.get("balance_usd")),
    })
    entry_value = entry["buy_value"] + entry["sell_value"]
    exit_value = exit["buy_value"] + exit["sell_value"]
    entry_qty = entry["buy_qty"] + entry["sell_qty"]
    exit_qty = exit["buy_qty"] + exit["sell_qty"]
    entry_fee = entry["fee"]
    exit_fee = exit["fee"]
    side = "buy" if entry["buy_qty"] >= entry["sell_qty"] else "sell"
    if side == "buy":
        price_move_cost = entry["buy_value"] - exit["sell_value"]
    else:
        price_move_cost = exit["buy_value"] - entry["sell_value"]
    trade_cost = price_move_cost + entry_fee + exit_fee
    balance_cost = Decimal(str(-(cost["balance_delta_usd"])))
    cost.update({
        "entry_qty": float(entry_qty),
        "exit_qty": float(exit_qty),
        "entry_value_usd": float(entry_value),
        "exit_value_usd": float(exit_value),
        "entry_fee_usd": float(entry_fee),
        "exit_fee_usd": float(exit_fee),
        "price_move_cost_usd": float(price_move_cost),
        "trade_cost_usd": float(trade_cost),
        "reconciliation_diff_usd": float(balance_cost - trade_cost),
        "clean": True,
        "metadata": {
            "balance_poll_attempt": balance_poll_attempt,
            "cost_source": "extended private user trades",
            "balance_source": "extended private user balance",
            "entry_trade_count": entry["count"],
            "exit_trade_count": exit["count"],
        },
    })
    expected_entry_qty = expected_fill_size(sample, "expected_entry_fill")
    expected_exit_qty = expected_fill_size(sample, "expected_exit_fill")
    if (expected_entry_qty > 0 and incomplete_qty(entry_qty, expected_entry_qty)) or (
        expected_exit_qty > 0 and incomplete_qty(exit_qty, expected_exit_qty)
    ):
        cost["clean"] = False
        cost["quality_reason"] = "incomplete entry or exit fill"
        metadata = dict(cost.get("metadata") or {})
        metadata["expected_entry_qty"] = str(expected_entry_qty)
        metadata["expected_exit_qty"] = str(expected_exit_qty)
        cost["metadata"] = metadata
    if abs(Decimal(str(cost["reconciliation_diff_usd"]))) > Decimal("0.02"):
        cost["clean"] = False
        cost["quality_reason"] = "balance reconciliation differs from fill cost"
    return cost


def base_cost(sample: dict[str, Any], before: dict[str, Any], after: dict[str, Any]) -> dict[str, Any]:
    return {
        "venue": "extended",
        "run_id": str(sample.get("run_id") or ""),
        "completed_at": str(sample.get("completed_at") or utc_now()),
        "balance_before_usd": number(before.get("balance_usd")),
        "balance_after_usd": number(after.get("balance_usd")),
        "clean": False,
    }


def aggregate_trades(trades: list[dict[str, Any]], external_id: str | None) -> dict[str, Any]:
    if not external_id:
        return {}
    matched = [trade for trade in trades if str(trade.get("externalOrderId") or trade.get("externalId") or "") == str(external_id)]
    return aggregate(matched, external_id)


def wait_trades(params: dict[str, Any], market: str, external_ids: set[str]) -> list[dict[str, Any]]:
    attempts = max(1, int(params.get("cost_poll_attempts", 12)))
    interval = max(0, int(params.get("cost_poll_interval_ms", 500))) / 1000
    wanted = {external_id for external_id in external_ids if external_id}
    trades: list[dict[str, Any]] = []
    for attempt in range(attempts):
        trades = private_json(params, "/api/v1/user/trades?" + urllib.parse.urlencode({"market": market})).get("data") or []
        seen = {str(trade.get("externalOrderId") or trade.get("externalId") or "") for trade in trades}
        if not wanted or wanted.issubset(seen):
            return trades
        if attempt != attempts - 1:
            time.sleep(interval)
    return trades


def aggregate_neutralize_after(trades: list[dict[str, Any]], created_time: int) -> dict[str, Any]:
    matched = [
        trade
        for trade in trades
        if str(trade.get("externalOrderId") or "").startswith("pb-neutralize-")
        and int(trade.get("createdTime") or 0) >= created_time
        and int(trade.get("createdTime") or 0) <= created_time + 15000
    ]
    if not matched:
        return {}
    return aggregate(matched, str(matched[0].get("externalOrderId") or ""))


def aggregate(trades: list[dict[str, Any]], order_id: str) -> dict[str, Any]:
    if not trades:
        return {}
    buy_qty = Decimal("0")
    sell_qty = Decimal("0")
    buy_value = Decimal("0")
    sell_value = Decimal("0")
    fee = Decimal("0")
    last_created = 0
    for trade in trades:
        qty = Decimal(str(trade.get("qty") or trade.get("filledQty") or "0"))
        value = Decimal(str(trade.get("value") or "0"))
        fee += Decimal(str(trade.get("fee") or "0"))
        side = str(trade.get("side") or "").upper()
        if side == "BUY":
            buy_qty += qty
            buy_value += value
        elif side == "SELL":
            sell_qty += qty
            sell_value += value
        last_created = max(last_created, int(trade.get("createdTime") or 0))
    return {
        "order_id": order_id,
        "buy_qty": buy_qty,
        "sell_qty": sell_qty,
        "buy_value": buy_value,
        "sell_value": sell_value,
        "fee": fee,
        "count": len(trades),
        "last_created_time": last_created,
    }


def first_external_id(value: Any) -> str:
    if isinstance(value, list):
        for item in value:
            if isinstance(item, dict):
                external_id = item.get("external_id") or item.get("externalId") or item.get("externalOrderId")
                if external_id not in (None, ""):
                    return str(external_id)
    return ""


def first_value(values: list[str]) -> str:
    return values[0] if values else ""


def private_json(params: dict[str, Any], path: str) -> dict[str, Any]:
    base_url = str(params.get("base_url", "https://api.starknet.extended.exchange")).rstrip("/")
    api_key = str(params.get("api_key") or os.getenv("EXTENDED_API_KEY") or "")
    if not api_key:
        raise SystemExit("EXTENDED_API_KEY is required for Extended cost fetch")
    request = urllib.request.Request(
        base_url + path,
        headers={"X-Api-Key": api_key, "User-Agent": "perps-latency-benchmark"},
    )
    with urllib.request.urlopen(request, timeout=float(params.get("cost_request_timeout_seconds", 8))) as response:
        body = json.loads(response.read().decode() or "{}")
    if str(body.get("status") or "").upper() != "OK":
        raise SystemExit(f"Extended API returned {body}")
    return body


def number(value: Any) -> float:
    if value in (None, ""):
        return 0.0
    return float(Decimal(str(value)))


def expected_fill_size(sample: dict[str, Any], field: str) -> Decimal:
    fill = sample.get(field)
    if not isinstance(fill, dict):
        return Decimal("0")
    return Decimal(str(number(fill.get("size"))))


def incomplete_qty(actual: Decimal, expected: Decimal) -> bool:
    if expected <= 0:
        return False
    tolerance = max(expected * Decimal("0.000001"), Decimal("0.00000001"))
    return actual + tolerance < expected


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def compact_json(value: Any) -> str:
    return json.dumps(value, separators=(",", ":"), sort_keys=False)


if __name__ == "__main__":
    raise SystemExit(main())
