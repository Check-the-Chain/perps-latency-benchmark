#!/usr/bin/env python3
"""Fetch Hyperliquid exact benchmark costs and account balance snapshots."""

from __future__ import annotations

import json
import os
import sys
import time
import urllib.request
from decimal import Decimal
from pathlib import Path
from typing import Any

sys.path.append(str(Path(__file__).resolve().parents[1]))
from cost_common import (
    add_fill,
    build_round_trip_cost,
    compact_json,
    completed_at_ms,
    finish_aggregate,
    new_aggregate,
    number,
    order_ref_values,
    utc_now,
)


DEFAULT_BASE_URL = "https://api.hyperliquid.xyz"


def main() -> int:
    try:
        from eth_account import Account
    except ImportError as exc:
        raise SystemExit("Hyperliquid cost fetch requires eth-account") from exc

    for line in sys.stdin:
        if not line.strip():
            continue
        print(compact_json(build(json.loads(line), Account)), flush=True)
    return 0


def build(req: dict[str, Any], Account: Any) -> dict[str, Any]:
    params = dict(req.get("params") or {})
    builder_params = dict(params.get("builder_params") or {})
    phase = str(params.get("phase") or "")
    client = HyperliquidCostClient(builder_params, Account)
    if phase == "balance":
        return {"metadata": {"balance": client.balance_snapshot()}}
    if phase == "sample_cost":
        return {"metadata": {"cost": sample_cost(client, params, builder_params)}}
    raise SystemExit(f"unsupported Hyperliquid cost phase {phase!r}")


def sample_cost(client: "HyperliquidCostClient", params: dict[str, Any], builder_params: dict[str, Any]) -> dict[str, Any]:
    sample = dict(params.get("sample") or {})
    metadata = dict(sample.get("metadata") or {})
    confirmation = dict(metadata.get("confirmation") or {})
    if confirmation.get("user"):
        client.user = str(confirmation["user"])
    before = dict(params.get("balance_before") or {})
    symbol = str(builder_params.get("symbol", "BTC"))
    entry_cloids = order_ref_values(sample, "order_refs", "cloid")
    exit_cloids = order_ref_values(sample, "closeout_order_refs", "cloid")

    entry_oids = client.oids_for_cloids(entry_cloids)
    exit_oids = client.oids_for_cloids(exit_cloids)
    start_ms = completed_at_ms(sample) - 60_000
    trades = client.wait_fills(start_ms, int(time.time() * 1000) + 10_000, entry_oids | exit_oids, set(entry_cloids) | set(exit_cloids))
    entry = aggregate_hyperliquid_fills(trades, symbol, entry_oids, set(entry_cloids), ",".join(entry_cloids))
    exit = aggregate_hyperliquid_fills(trades, symbol, exit_oids, set(exit_cloids), ",".join(exit_cloids))
    if not exit and entry:
        exit = aggregate_exit_after(trades, symbol, entry, entry_oids)

    return build_with_reconciled_balance(client, sample, before, entry, exit, builder_params)


def aggregate_hyperliquid_fills(trades: list[dict[str, Any]], symbol: str, oids: set[str], cloids: set[str], label: str) -> dict[str, Any]:
    aggregate = new_aggregate(label or ",".join(sorted(oids)))
    for trade in trades:
        oid = str(trade.get("oid") or "")
        cloid = str(trade.get("cloid") or "")
        if str(trade.get("coin") or "") != symbol or (oid not in oids and cloid not in cloids):
            continue
        qty = trade.get("sz") or "0"
        value = Decimal(str(trade.get("px") or "0")) * Decimal(str(qty))
        add_fill(aggregate, trade.get("side"), qty, value, trade.get("fee"), trade.get("time"))
    return finish_aggregate(aggregate)


def build_with_reconciled_balance(client: "HyperliquidCostClient", sample: dict[str, Any], before: dict[str, Any], entry: dict[str, Any], exit: dict[str, Any], builder_params: dict[str, Any]) -> dict[str, Any]:
    source_metadata = {
        "cost_source": "hyperliquid userFillsByTime",
        "balance_source": "hyperliquid auto balance snapshot",
        "reconciliation_tolerance_usd": str(builder_params.get("cost_reconciliation_tolerance_usd", "0.02")),
    }
    if not entry or not exit:
        after = client.balance_snapshot()
        return build_round_trip_cost("hyperliquid", sample, before, after, entry, exit, source_metadata)
    before_source = str(dict(before.get("metadata") or {}).get("balance_source") or "")
    best: dict[str, Any] | None = None
    best_abs_diff: Decimal | None = None
    for attempt in range(client.balance_poll_attempts):
        after = client.balance_snapshot()
        cost = build_round_trip_cost("hyperliquid", sample, before, after, entry, exit, source_metadata | {"balance_poll_attempt": attempt + 1})
        after_source = str(dict(after.get("metadata") or {}).get("balance_source") or "")
        if before_source and after_source and before_source != after_source:
            cost["clean"] = False
            cost["quality_reason"] = "balance source changed during sample"
        diff = abs(number(cost.get("reconciliation_diff_usd")))
        if best is None or best_abs_diff is None or diff < best_abs_diff:
            best = cost
            best_abs_diff = diff
        if cost.get("clean"):
            return cost
        if attempt != client.balance_poll_attempts - 1:
            time.sleep(client.balance_poll_interval)
    return best or {}


def aggregate_exit_after(trades: list[dict[str, Any]], symbol: str, entry: dict[str, Any], entry_oids: set[str]) -> dict[str, Any]:
    aggregate = new_aggregate("")
    entry_side = "buy" if Decimal(str(entry.get("buy_qty") or "0")) >= Decimal(str(entry.get("sell_qty") or "0")) else "sell"
    want_side = "A" if entry_side == "buy" else "B"
    start_ms = int(entry.get("last_time_ms") or 0)
    for trade in trades:
        if str(trade.get("coin") or "") != symbol:
            continue
        if str(trade.get("oid") or "") in entry_oids:
            continue
        time_ms = int(trade.get("time") or 0)
        if time_ms < start_ms or time_ms > start_ms + 30_000:
            continue
        if str(trade.get("side") or "") != want_side:
            continue
        qty = trade.get("sz") or "0"
        value = Decimal(str(trade.get("px") or "0")) * Decimal(str(qty))
        add_fill(aggregate, trade.get("side"), qty, value, trade.get("fee"), time_ms)
    aggregate["order_id"] = "fallback_after_entry"
    return finish_aggregate(aggregate)


class HyperliquidCostClient:
    def __init__(self, params: dict[str, Any], Account: Any):
        self.base_url = str(params.get("base_url", DEFAULT_BASE_URL)).rstrip("/")
        self.timeout = float(params.get("cost_request_timeout_seconds", 8))
        wallet = Account.from_key(env_or_param(params, "secret_key", "HYPERLIQUID_SECRET_KEY"))
        self.user = str(params.get("user_address") or params.get("user") or os.getenv("HYPERLIQUID_USER_ADDRESS") or wallet.address)
        self.poll_attempts = max(1, int(params.get("cost_poll_attempts", 40)))
        self.poll_interval = max(0, int(params.get("cost_poll_interval_ms", 500))) / 1000
        self.balance_poll_attempts = max(1, int(params.get("cost_balance_poll_attempts", self.poll_attempts)))
        self.balance_poll_interval = max(0, int(params.get("cost_balance_poll_interval_ms", int(self.poll_interval * 1000)))) / 1000
        self.balance_source = str(params.get("balance_source") or "auto")

    def balance_snapshot(self) -> dict[str, Any]:
        state = self.info({"type": "clearinghouseState", "user": self.user})
        spot_state = self.info({"type": "spotClearinghouseState", "user": self.user})
        return balance_snapshot_from_states(state, spot_state, self.balance_source)

    def oids_for_cloids(self, cloids: list[str]) -> set[str]:
        oids: set[str] = set()
        for cloid in cloids:
            status = self.info({"type": "orderStatus", "user": self.user, "oid": cloid})
            oid = find_key(status, "oid")
            if oid not in (None, ""):
                oids.add(str(oid))
        return oids

    def wait_fills(self, start_ms: int, end_ms: int, oids: set[str], cloids: set[str]) -> list[dict[str, Any]]:
        fills: list[dict[str, Any]] = []
        for attempt in range(self.poll_attempts):
            fills = self.info({
                "type": "userFillsByTime",
                "user": self.user,
                "startTime": max(0, start_ms),
                "endTime": max(start_ms + 1, end_ms),
                "aggregateByTime": False,
            })
            if not isinstance(fills, list):
                fills = []
            seen_oids = {str(fill.get("oid") or "") for fill in fills}
            seen_cloids = {str(fill.get("cloid") or "") for fill in fills}
            oids_ready = bool(oids) and oids.issubset(seen_oids)
            cloids_ready = bool(cloids) and cloids.issubset(seen_cloids)
            if oids_ready or cloids_ready or (not oids and not cloids):
                return fills
            if attempt != self.poll_attempts - 1:
                time.sleep(self.poll_interval)
        return fills

    def info(self, body: dict[str, Any]) -> Any:
        request = urllib.request.Request(
            self.base_url + "/info",
            data=json.dumps(body).encode(),
            method="POST",
            headers={"Content-Type": "application/json"},
        )
        with urllib.request.urlopen(request, timeout=self.timeout) as response:
            return json.loads(response.read().decode() or "{}")


def balance_snapshot_from_states(state: dict[str, Any], spot_state: dict[str, Any], source: str = "auto") -> dict[str, Any]:
    margin = dict(state.get("marginSummary") or state.get("crossMarginSummary") or {})
    account_value = number(margin.get("accountValue"))
    spot_usdc = spot_balance(spot_state, "USDC")
    spot_total = number(spot_usdc.get("total"))
    spot_hold = number(spot_usdc.get("hold"))
    unrealized = Decimal("0")
    for wrapped in state.get("assetPositions") or []:
        position = dict(wrapped.get("position") or {})
        unrealized += number(position.get("unrealizedPnl"))

    selected_source = source
    if source == "auto":
        selected_source = "spotClearinghouseState.USDC.total" if account_value == 0 and spot_total != 0 else "clearinghouseState.marginSummary.accountValue"
    if selected_source in ("spot", "spot_usdc", "spotClearinghouseState.USDC.total"):
        balance = spot_total
        selected_source = "spotClearinghouseState.USDC.total"
    elif selected_source in ("clearinghouse", "clearinghouseState.marginSummary.accountValue"):
        balance = account_value
        selected_source = "clearinghouseState.marginSummary.accountValue"
    else:
        raise SystemExit(f"unsupported Hyperliquid balance_source {source!r}")

    return {
        "venue": "hyperliquid",
        "currency": "USDC",
        "balance_usd": float(balance),
        "equity_usd": float(balance),
        "unrealized_usd": float(unrealized),
        "captured_at": utc_now(),
        "metadata": {
            "balance_source": selected_source,
            "clearinghouse_account_value": str(account_value),
            "spot_usdc_total": str(spot_total),
            "spot_usdc_hold": str(spot_hold),
            "withdrawable": str(state.get("withdrawable") or ""),
            "total_ntl_pos": str(margin.get("totalNtlPos") or ""),
            "time": state.get("time") or spot_state.get("time"),
        },
    }


def spot_balance(state: dict[str, Any], coin: str) -> dict[str, Any]:
    for balance in state.get("balances") or []:
        if str(balance.get("coin") or "") == coin:
            return dict(balance)
    return {}


def find_key(value: Any, key: str) -> Any:
    if isinstance(value, dict):
        if key in value:
            return value[key]
        for child in value.values():
            found = find_key(child, key)
            if found not in (None, ""):
                return found
    if isinstance(value, list):
        for child in value:
            found = find_key(child, key)
            if found not in (None, ""):
                return found
    return None


def env_or_param(params: dict[str, Any], key: str, env_key: str) -> str:
    value = params.get(key) or os.getenv(env_key)
    if value in (None, ""):
        raise SystemExit(f"{env_key} is required for Hyperliquid cost fetch")
    return str(value)


if __name__ == "__main__":
    raise SystemExit(main())
