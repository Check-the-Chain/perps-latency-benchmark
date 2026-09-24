from __future__ import annotations

import json
from datetime import datetime, timezone
from decimal import Decimal
from typing import Any


def base_cost(venue: str, sample: dict[str, Any], before: dict[str, Any], after: dict[str, Any]) -> dict[str, Any]:
    before_usd = number(before.get("balance_usd"))
    after_usd = number(after.get("balance_usd"))
    return {
        "venue": venue,
        "run_id": str(sample.get("run_id") or ""),
        "completed_at": str(sample.get("completed_at") or utc_now()),
        "balance_before": before,
        "balance_after": after,
        "balance_before_usd": float(before_usd),
        "balance_after_usd": float(after_usd),
        "balance_delta_usd": float(after_usd - before_usd),
        "clean": False,
    }


def build_round_trip_cost(
    venue: str,
    sample: dict[str, Any],
    before: dict[str, Any],
    after: dict[str, Any],
    entry: dict[str, Any],
    exit: dict[str, Any],
    source_metadata: dict[str, Any],
) -> dict[str, Any]:
    cost = base_cost(venue, sample, before, after)
    cost.update({
        "entry_order_id": str(entry.get("order_id") or ""),
        "exit_order_id": str(exit.get("order_id") or ""),
    })
    if not entry or not exit:
        cost["clean"] = False
        cost["quality_reason"] = "missing entry or exit fill"
        return cost

    entry_buy_qty = decimal(entry.get("buy_qty"))
    entry_sell_qty = decimal(entry.get("sell_qty"))
    exit_buy_qty = decimal(exit.get("buy_qty"))
    exit_sell_qty = decimal(exit.get("sell_qty"))
    entry_buy_value = decimal(entry.get("buy_value"))
    entry_sell_value = decimal(entry.get("sell_value"))
    exit_buy_value = decimal(exit.get("buy_value"))
    exit_sell_value = decimal(exit.get("sell_value"))
    entry_fee = decimal(entry.get("fee"))
    exit_fee = decimal(exit.get("fee"))
    entry_qty = entry_buy_qty + entry_sell_qty
    exit_qty = exit_buy_qty + exit_sell_qty

    side = "buy" if entry_buy_qty >= entry_sell_qty else "sell"
    if side == "buy":
        price_move_cost = entry_buy_value - exit_sell_value
    else:
        price_move_cost = exit_buy_value - entry_sell_value
    trade_cost = price_move_cost + entry_fee + exit_fee
    balance_cost = -decimal(cost["balance_delta_usd"])

    cost.update({
        "entry_qty": float(entry_qty),
        "exit_qty": float(exit_qty),
        "entry_value_usd": float(entry_buy_value + entry_sell_value),
        "exit_value_usd": float(exit_buy_value + exit_sell_value),
        "entry_fee_usd": float(entry_fee),
        "exit_fee_usd": float(exit_fee),
        "price_move_cost_usd": float(price_move_cost),
        "trade_cost_usd": float(trade_cost),
        "reconciliation_diff_usd": float(balance_cost - trade_cost),
        "clean": True,
        "metadata": source_metadata | {
            "entry_trade_count": int(entry.get("count") or 0),
            "exit_trade_count": int(exit.get("count") or 0),
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
    if abs(decimal(cost["reconciliation_diff_usd"])) > decimal(source_metadata.get("reconciliation_tolerance_usd", "0.02")):
        cost["clean"] = False
        cost["quality_reason"] = "balance reconciliation differs from fill cost"
    return cost


def new_aggregate(order_id: str) -> dict[str, Any]:
    return {
        "order_id": order_id,
        "buy_qty": Decimal("0"),
        "sell_qty": Decimal("0"),
        "buy_value": Decimal("0"),
        "sell_value": Decimal("0"),
        "fee": Decimal("0"),
        "count": 0,
        "last_time_ms": 0,
    }


def add_fill(aggregate: dict[str, Any], side: str, qty: Any, value: Any, fee: Any, time_ms: Any = 0) -> None:
    qty_dec = decimal(qty)
    value_dec = decimal(value)
    side_text = str(side or "").lower()
    if side_text in ("buy", "b", "bid", "long"):
        aggregate["buy_qty"] += qty_dec
        aggregate["buy_value"] += value_dec
    elif side_text in ("sell", "s", "a", "ask", "short"):
        aggregate["sell_qty"] += qty_dec
        aggregate["sell_value"] += value_dec
    else:
        return
    aggregate["fee"] += abs(decimal(fee))
    aggregate["count"] += 1
    aggregate["last_time_ms"] = max(int(aggregate.get("last_time_ms") or 0), int(time_ms or 0))


def finish_aggregate(aggregate: dict[str, Any]) -> dict[str, Any]:
    if int(aggregate.get("count") or 0) == 0:
        return {}
    return aggregate


def cleanup_values(metadata: dict[str, Any], *keys: str) -> list[str]:
    values: list[str] = []
    raw = metadata.get("cleanup_orders")
    if isinstance(raw, list):
        for item in raw:
            if not isinstance(item, dict):
                continue
            for key in keys:
                value = item.get(key)
                if value not in (None, ""):
                    values.append(str(value))
                    break
    return values


def order_ref_values(sample: dict[str, Any], field: str, *keys: str) -> list[str]:
    values = refs_values(sample.get(field), *keys)
    if values:
        return values
    metadata = dict(sample.get("metadata") or {})
    if field == "order_refs":
        return cleanup_values(metadata, *keys)
    return cleanup_values(cleanup_metadata(metadata), *keys)


def refs_values(refs: Any, *keys: str) -> list[str]:
    values: list[str] = []
    if isinstance(refs, list):
        for item in refs:
            if not isinstance(item, dict):
                continue
            for key in keys:
                value = item.get(key)
                if value not in (None, ""):
                    values.append(str(value))
                    break
    return values


def cleanup_metadata(sample_metadata: dict[str, Any]) -> dict[str, Any]:
    value = sample_metadata.get("cleanup_metadata")
    return value if isinstance(value, dict) else {}


def completed_at_ms(sample: dict[str, Any]) -> int:
    return time_ms(sample.get("completed_at") or sample.get("sent_at") or sample.get("scheduled_at"))


def time_ms(value: Any) -> int:
    if value in (None, ""):
        return 0
    if isinstance(value, (int, float)):
        return int(value)
    text = str(value)
    if text.isdigit():
        return int(text)
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"
    return int(datetime.fromisoformat(text).timestamp() * 1000)


def decimal(value: Any) -> Decimal:
    if value in (None, ""):
        return Decimal("0")
    return Decimal(str(value))


def expected_fill_size(sample: dict[str, Any], field: str) -> Decimal:
    fill = sample.get(field)
    if not isinstance(fill, dict):
        return Decimal("0")
    return decimal(fill.get("size"))


def incomplete_qty(actual: Decimal, expected: Decimal) -> bool:
    if expected <= 0:
        return False
    tolerance = max(expected * Decimal("0.000001"), Decimal("0.00000001"))
    return actual + tolerance < expected


def number(value: Any) -> Decimal:
    return decimal(value)


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def compact_json(value: Any) -> str:
    return json.dumps(value, separators=(",", ":"), sort_keys=False, default=str)
