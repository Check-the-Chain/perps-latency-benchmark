#!/usr/bin/env python3
"""Fetch Lighter exact benchmark costs and account balance snapshots."""

from __future__ import annotations

import asyncio
import inspect
import json
import sys
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
from build_payload import api_key_material, env_or_param, to_plain


DEFAULT_BASE_URL = "https://mainnet.zklighter.elliot.ai"


def main() -> int:
    try:
        import lighter
    except ImportError as exc:
        raise SystemExit("missing Lighter SDK; run with `uv run --with lighter-sdk python ...`") from exc

    asyncio.run(serve(lighter))
    return 0


async def serve(lighter: Any) -> None:
    for line in sys.stdin:
        if not line.strip():
            continue
        print(compact_json(await build(json.loads(line), lighter)), flush=True)


async def build(req: dict[str, Any], lighter: Any) -> dict[str, Any]:
    params = dict(req.get("params") or {})
    builder_params = dict(params.get("builder_params") or {})
    venue = str(req.get("venue") or "lighter")
    phase = str(params.get("phase") or "")
    client = LighterCostClient(lighter, builder_params, venue)
    try:
        if phase == "balance":
            return {"metadata": {"balance": await client.balance_snapshot()}}
        if phase == "sample_cost":
            return {"metadata": {"cost": await sample_cost(client, params, builder_params)}}
    finally:
        await client.close()
    raise SystemExit(f"unsupported Lighter cost phase {phase!r}")


async def sample_cost(client: "LighterCostClient", params: dict[str, Any], builder_params: dict[str, Any]) -> dict[str, Any]:
    sample = dict(params.get("sample") or {})
    before = dict(params.get("balance_before") or {})
    market_index = int(builder_params.get("market_index", 1))
    entry_ids = order_ref_values(sample, "order_refs", "client_order_index", "client_order_id", "order_index")
    exit_ids = order_ref_values(sample, "closeout_order_refs", "client_order_index", "client_order_id", "order_index")
    start_ms = completed_at_ms(sample) - 10_000
    end_ms = completed_at_ms(sample) + 60_000

    trades = await client.wait_trades(market_index, set(entry_ids) | set(exit_ids), start_ms, end_ms)
    entry = aggregate_lighter_trades(trades, client.account_index, set(entry_ids), ",".join(entry_ids), builder_params, start_ms, end_ms)
    exit = aggregate_lighter_trades(trades, client.account_index, set(exit_ids), ",".join(exit_ids), builder_params, start_ms, end_ms)
    return await build_with_reconciled_balance(client, sample, before, entry, exit, builder_params)


def aggregate_lighter_trades(trades: list[dict[str, Any]], account_index: int, client_ids: set[str], label: str, params: dict[str, Any], start_ms: int, end_ms: int) -> dict[str, Any]:
    aggregate = new_aggregate(label)
    fee_scale = Decimal(str(params.get("fee_scale", "1000000")))
    for trade in trades:
        ask_account = int(trade.get("ask_account_id") or -1)
        bid_account = int(trade.get("bid_account_id") or -1)
        time_ms = int(trade.get("timestamp") or 0)
        if time_ms < start_ms or time_ms > end_ms:
            continue
        if ask_account == account_index:
            if str(trade.get("ask_client_id") or "") not in client_ids:
                continue
            side = "sell"
            fee = trade.get("maker_fee") if bool(trade.get("is_maker_ask")) else trade.get("taker_fee")
        elif bid_account == account_index:
            if str(trade.get("bid_client_id") or "") not in client_ids:
                continue
            side = "buy"
            fee = trade.get("taker_fee") if bool(trade.get("is_maker_ask")) else trade.get("maker_fee")
        else:
            continue
        value = Decimal(str(trade.get("usd_amount") or Decimal(str(trade.get("price") or "0")) * Decimal(str(trade.get("size") or "0"))))
        add_fill(
            aggregate,
            side,
            trade.get("size") or "0",
            value,
            value * Decimal(str(fee or "0")) / fee_scale,
            time_ms,
        )
    return finish_aggregate(aggregate)


async def build_with_reconciled_balance(client: "LighterCostClient", sample: dict[str, Any], before: dict[str, Any], entry: dict[str, Any], exit: dict[str, Any], builder_params: dict[str, Any]) -> dict[str, Any]:
    market_index = int(builder_params.get("market_index", 1))
    preexisting_positions = nonzero_positions(before, market_index)
    source_metadata = {
        "cost_source": "lighter private trades",
        "balance_source": "lighter account equity",
        "reconciliation_tolerance_usd": str(builder_params.get("cost_reconciliation_tolerance_usd", "0.02")),
    }
    if not entry or not exit:
        after = await client.balance_snapshot()
        return mark_position_contaminated(build_round_trip_cost(client.venue_for_sample(sample), sample, before, after, entry, exit, source_metadata), preexisting_positions)
    best: dict[str, Any] | None = None
    best_abs_diff: Decimal | None = None
    for attempt in range(client.balance_poll_attempts):
        after = await client.balance_snapshot()
        cost = mark_position_contaminated(
            build_round_trip_cost(client.venue_for_sample(sample), sample, before, after, entry, exit, source_metadata | {"balance_poll_attempt": attempt + 1}),
            preexisting_positions,
        )
        diff = abs(number(cost.get("reconciliation_diff_usd")))
        if best is None or best_abs_diff is None or diff < best_abs_diff:
            best = cost
            best_abs_diff = diff
        if cost.get("clean"):
            return cost
        if attempt != client.balance_poll_attempts - 1:
            await asyncio.sleep(client.balance_poll_interval)
    return mark_position_contaminated(best or {}, preexisting_positions)


class LighterCostClient:
    def __init__(self, lighter: Any, params: dict[str, Any], venue: str = "lighter"):
        self.lighter = lighter
        self.venue = venue or "lighter"
        self.account_index = int(env_or_param(params, "account_index", "LIGHTER_ACCOUNT_INDEX"))
        self.api_key_index, private_key, _role = api_key_material(params, "market")
        self.signer = lighter.SignerClient(
            url=params.get("base_url", DEFAULT_BASE_URL),
            api_private_keys={self.api_key_index: private_key},
            account_index=self.account_index,
        )
        self.account_api = lighter.AccountApi(self.signer.api_client)
        self.order_api = lighter.OrderApi(self.signer.api_client)
        self.timeout = float(params.get("cost_request_timeout_seconds", 8))
        self.poll_attempts = max(1, int(params.get("cost_poll_attempts", 12)))
        self.poll_interval = max(0, int(params.get("cost_poll_interval_ms", 500))) / 1000
        self.balance_poll_attempts = max(1, int(params.get("cost_balance_poll_attempts", self.poll_attempts)))
        self.balance_poll_interval = max(0, int(params.get("cost_balance_poll_interval_ms", int(self.poll_interval * 1000)))) / 1000

    async def close(self) -> None:
        await self.signer.close()

    async def balance_snapshot(self) -> dict[str, Any]:
        response = await maybe_await(self.account_api.account(by="index", value=str(self.account_index), _request_timeout=self.timeout))
        plain = to_plain(response)
        accounts = plain.get("accounts") if isinstance(plain, dict) else []
        account = accounts[0] if accounts else {}
        collateral = number(account.get("collateral"))
        equity = number(account.get("total_asset_value") or account.get("collateral"))
        unrealized = Decimal("0")
        positions = []
        for position in account.get("positions") or []:
            unrealized += number(position.get("unrealized_pnl"))
            item = position_item(position)
            if Decimal(item["size"]) != 0:
                positions.append(item)
        return {
            "venue": self.venue,
            "currency": "USDC",
            "balance_usd": float(equity),
            "equity_usd": float(equity),
            "unrealized_usd": float(unrealized),
            "positions": positions,
            "captured_at": utc_now(),
            "metadata": {
                "collateral": str(collateral),
                "available_balance": str(account.get("available_balance") or ""),
                "account_type": account.get("account_type"),
                "transaction_time": account.get("transaction_time"),
            },
        }

    def venue_for_sample(self, sample: dict[str, Any]) -> str:
        return str(sample.get("venue") or self.venue or "lighter")

    async def wait_trades(self, market_index: int, client_ids: set[str], start_ms: int, end_ms: int) -> list[dict[str, Any]]:
        if not client_ids:
            return []
        auth, error = self.signer.create_auth_token_with_expiry(api_key_index=self.api_key_index)
        if error:
            raise SystemExit(f"Lighter auth token failed: {error}")
        trades: list[dict[str, Any]] = []
        for attempt in range(self.poll_attempts):
            response = await maybe_await(self.order_api.trades(
                sort_by="timestamp",
                sort_dir="desc",
                limit=100,
                auth=auth,
                market_id=market_index,
                account_index=self.account_index,
                _request_timeout=self.timeout,
            ))
            plain = to_plain(response)
            trades = plain.get("trades") if isinstance(plain, dict) else []
            seen = set()
            for trade in trades:
                time_ms = int(trade.get("timestamp") or 0)
                if time_ms < start_ms or time_ms > end_ms:
                    continue
                if int(trade.get("ask_account_id") or -1) == self.account_index:
                    seen.add(str(trade.get("ask_client_id") or ""))
                if int(trade.get("bid_account_id") or -1) == self.account_index:
                    seen.add(str(trade.get("bid_client_id") or ""))
            if client_ids.issubset(seen):
                return trades
            if attempt != self.poll_attempts - 1:
                await asyncio.sleep(self.poll_interval)
        return trades


async def maybe_await(value: Any) -> Any:
    if inspect.isawaitable(value):
        return await value
    return value


def position_item(position: dict[str, Any]) -> dict[str, Any]:
    market = first_present(position, "market_index", "market_id", "symbol")
    size = first_present(position, "position", "position_size", "open_order_base_amount", "base_amount", "size")
    return {
        "market": str(market),
        "symbol": str(first_present(position, "symbol", "market") or ""),
        "size": str(size or "0"),
        "unrealized_usd": float(number(position.get("unrealized_pnl"))),
        "avg_entry_price": str(first_present(position, "avg_entry_price", "entry_price", "average_entry_price") or ""),
    }


def first_present(data: dict[str, Any], *keys: str) -> Any:
    for key in keys:
        if key in data and data[key] is not None:
            return data[key]
    return None


def nonzero_positions(snapshot: dict[str, Any], market_index: int) -> list[dict[str, Any]]:
    positions = snapshot.get("positions") or []
    out = []
    for position in positions:
        if not isinstance(position, dict):
            continue
        if str(position.get("market") or "") != str(market_index):
            continue
        if number(position.get("size")) != 0:
            out.append(position)
    return out


def mark_position_contaminated(cost: dict[str, Any], positions: list[dict[str, Any]]) -> dict[str, Any]:
    if not positions:
        return cost
    cost["clean"] = False
    cost["quality_reason"] = "account has an existing Lighter position; balance audit cannot isolate benchmark cost"
    metadata = dict(cost.get("metadata") or {})
    metadata["balance_audit_blocked_by_position"] = True
    metadata["preexisting_position_count"] = len(positions)
    cost["metadata"] = metadata
    return cost


if __name__ == "__main__":
    raise SystemExit(main())
