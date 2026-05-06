#!/usr/bin/env python3
"""Fetch Aster exact benchmark costs and account balance snapshots."""

from __future__ import annotations

import json
import sys
import time
import urllib.error
import urllib.parse
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
from build_payload import load_signer


DEFAULT_BASE_URL = "https://fapi.asterdex.com"


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
    client = AsterCostClient(builder_params)
    if phase == "balance":
        return {"metadata": {"balance": client.balance_snapshot()}}
    if phase == "sample_cost":
        return {"metadata": {"cost": sample_cost(client, params, builder_params)}}
    raise SystemExit(f"unsupported Aster cost phase {phase!r}")


def sample_cost(client: "AsterCostClient", params: dict[str, Any], builder_params: dict[str, Any]) -> dict[str, Any]:
    sample = dict(params.get("sample") or {})
    before = dict(params.get("balance_before") or {})
    symbol = str(builder_params.get("symbol", "BTCUSDT")).upper()
    entry_client_ids = order_ref_values(sample, "order_refs", "client_order_id", "newClientOrderId", "clientOrderId")
    exit_client_ids = order_ref_values(sample, "closeout_order_refs", "client_order_id", "newClientOrderId", "clientOrderId")

    entry_order_ids = client.order_ids(symbol, entry_client_ids)
    exit_order_ids = client.order_ids(symbol, exit_client_ids)
    trades = client.wait_trades(symbol, completed_at_ms(sample) - 60_000, int(time.time() * 1000) + 10_000, entry_order_ids | exit_order_ids)

    entry = aggregate_aster_trades(trades, entry_order_ids, ",".join(entry_client_ids))
    exit = aggregate_aster_trades(trades, exit_order_ids, ",".join(exit_client_ids))
    return build_with_reconciled_balance(client, sample, before, entry, exit, builder_params)


def build_with_reconciled_balance(
    client: "AsterCostClient",
    sample: dict[str, Any],
    before: dict[str, Any],
    entry: dict[str, Any],
    exit: dict[str, Any],
    builder_params: dict[str, Any],
) -> dict[str, Any]:
    source_metadata = {
        "cost_source": "aster private user trades",
        "balance_source": "aster futures account balance",
        "reconciliation_tolerance_usd": str(builder_params.get("cost_reconciliation_tolerance_usd", "0.02")),
    }
    if not entry or not exit:
        return build_round_trip_cost("aster", sample, before, client.balance_snapshot(), entry, exit, source_metadata)

    best: dict[str, Any] | None = None
    best_abs_diff: Decimal | None = None
    for attempt in range(client.balance_poll_attempts):
        cost = build_round_trip_cost(
            "aster",
            sample,
            before,
            client.balance_snapshot(),
            entry,
            exit,
            source_metadata | {"balance_poll_attempt": attempt + 1},
        )
        diff = abs(number(cost.get("reconciliation_diff_usd")))
        if best is None or best_abs_diff is None or diff < best_abs_diff:
            best = cost
            best_abs_diff = diff
        if cost.get("clean"):
            return cost
        if attempt != client.balance_poll_attempts - 1:
            time.sleep(client.balance_poll_interval)
    return best or {}


def aggregate_aster_trades(trades: list[dict[str, Any]], order_ids: set[str], label: str) -> dict[str, Any]:
    aggregate = new_aggregate(label or ",".join(sorted(order_ids)))
    for trade in trades:
        if str(trade.get("orderId") or "") not in order_ids:
            continue
        qty = trade.get("qty") or "0"
        value = trade.get("quoteQty") or (Decimal(str(trade.get("price") or "0")) * Decimal(str(qty)))
        add_fill(aggregate, trade.get("side"), qty, value, trade.get("commission"), trade.get("time"))
    return finish_aggregate(aggregate)


class AsterCostClient:
    def __init__(self, params: dict[str, Any]):
        self.base_url = str(params.get("base_url", DEFAULT_BASE_URL)).rstrip("/")
        self.signer = load_signer(params)
        self.timeout = float(params.get("cost_request_timeout_seconds", 8))
        self.poll_attempts = max(1, int(params.get("cost_poll_attempts", 5)))
        self.poll_interval = max(0, int(params.get("cost_poll_interval_ms", 500))) / 1000
        self.balance_poll_attempts = max(1, int(params.get("cost_balance_poll_attempts", self.poll_attempts)))
        self.balance_poll_interval = max(0, int(params.get("cost_balance_poll_interval_ms", int(self.poll_interval * 1000)))) / 1000

    def balance_snapshot(self) -> dict[str, Any]:
        body = self.signed_get("/fapi/v3/balance", {})
        balances = body if isinstance(body, list) else []
        usdt = next((item for item in balances if str(item.get("asset") or "").upper() == "USDT"), balances[0] if balances else {})
        balance = number(usdt.get("balance"))
        unrealized = number(usdt.get("crossUnPnl"))
        equity = balance + unrealized
        return {
            "venue": "aster",
            "currency": str(usdt.get("asset") or "USDT"),
            "balance_usd": float(equity),
            "equity_usd": float(equity),
            "unrealized_usd": float(unrealized),
            "captured_at": utc_now(),
            "metadata": {
                "available_balance": str(usdt.get("availableBalance") or ""),
                "wallet_balance": str(balance),
                "cross_wallet_balance": str(usdt.get("crossWalletBalance") or ""),
                "update_time": usdt.get("updateTime"),
            },
        }

    def order_ids(self, symbol: str, client_ids: list[str]) -> set[str]:
        ids: set[str] = set()
        for client_id in client_ids:
            order = self.signed_get("/fapi/v3/order", {"symbol": symbol, "origClientOrderId": client_id})
            order_id = order.get("orderId") if isinstance(order, dict) else None
            if order_id not in (None, ""):
                ids.add(str(order_id))
        return ids

    def wait_trades(self, symbol: str, start_ms: int, end_ms: int, order_ids: set[str]) -> list[dict[str, Any]]:
        trades: list[dict[str, Any]] = []
        for attempt in range(self.poll_attempts):
            trades = self.user_trades(symbol, start_ms, end_ms)
            if not order_ids or order_ids.issubset({str(trade.get("orderId") or "") for trade in trades}):
                return trades
            if attempt != self.poll_attempts - 1:
                time.sleep(self.poll_interval)
        return trades

    def user_trades(self, symbol: str, start_ms: int, end_ms: int) -> list[dict[str, Any]]:
        body = self.signed_get("/fapi/v3/userTrades", {
            "symbol": symbol,
            "startTime": str(max(0, start_ms)),
            "endTime": str(max(start_ms + 1, end_ms)),
            "limit": "1000",
        })
        return body if isinstance(body, list) else []

    def signed_get(self, path: str, values: dict[str, str]) -> Any:
        body = self.signer.sign(values)
        request = urllib.request.Request(
            self.base_url + path + "?" + body,
            method="GET",
            headers={"Content-Type": "application/x-www-form-urlencoded"},
        )
        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as response:
                return json.loads(response.read().decode() or "{}")
        except urllib.error.HTTPError as exc:
            raw = exc.read().decode(errors="replace")
            try:
                decoded = json.loads(raw)
            except json.JSONDecodeError:
                decoded = {"msg": raw}
            raise SystemExit(f"Aster API returned {exc.code}: {decoded}") from exc


if __name__ == "__main__":
    raise SystemExit(main())
