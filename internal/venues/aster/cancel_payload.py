#!/usr/bin/env python3
"""Cleanup Aster benchmark orders outside the measured latency window."""

from __future__ import annotations

import json
import sys
import time
import urllib.error
import urllib.request
from decimal import Decimal
from pathlib import Path
from typing import Any

sys.path.append(str(Path(__file__).resolve().parents[1]))
from cleanup_common import cleanup_orders_for_venue, cleanup_result, result_orders_for_venue
from build_payload import DEFAULT_WS_BASE_URL, cached_listen_key, client_order_id, compact_json, load_signer


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
    client = AsterClient(builder_params)
    phase = params.get("phase", "after_sample")
    if phase == "before_run":
        return before_run(client, params, builder_params)
    if phase == "after_run":
        return after_run(client, params, builder_params)
    return after_sample(client, params, builder_params)


class AsterClient:
    def __init__(self, params: dict[str, Any]):
        self.params = params
        self.base_url = str(params.get("base_url", DEFAULT_BASE_URL)).rstrip("/")
        self.signer = load_signer(params)
        self.timeout = float(params.get("cleanup_request_timeout_seconds", 10))

    def signed(self, method: str, path: str, values: dict[str, str]) -> tuple[int, Any]:
        body = self.signer.sign(values)
        data = body.encode() if method != "GET" else None
        url = self.base_url + path
        if method == "GET":
            url = url + "?" + body
        request = urllib.request.Request(
            url,
            data=data,
            method=method,
            headers={
                "Content-Type": "application/x-www-form-urlencoded",
            },
        )
        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as response:
                return response.status, json.loads(response.read().decode() or "{}")
        except urllib.error.HTTPError as exc:
            raw = exc.read().decode(errors="replace")
            try:
                decoded = json.loads(raw)
            except json.JSONDecodeError:
                decoded = {"msg": raw}
            return exc.code, decoded

    def open_orders(self, symbol: str) -> list[dict[str, Any]]:
        _, body = self.signed("GET", "/fapi/v3/openOrders", {"symbol": symbol})
        if isinstance(body, list):
            return body
        require_ok(body)
        return []

    def cancel_order(self, symbol: str, client_order_id: str) -> tuple[int, Any]:
        return self.signed("DELETE", "/fapi/v3/order", {"symbol": symbol, "origClientOrderId": client_order_id})

    def cancel_order_body(self, symbol: str, client_order_id: str) -> str:
        return self.signer.sign({"symbol": symbol, "origClientOrderId": client_order_id})

    def cancel_batch_body(self, refs: list[dict[str, str]]) -> str:
        symbols = {ref["symbol"] for ref in refs}
        if len(symbols) != 1:
            raise SystemExit("Aster batch cancel requires one symbol per request")
        client_ids = [ref["client_order_id"] for ref in refs]
        return self.signer.sign({"symbol": next(iter(symbols)), "origClientOrderIdList": compact_json(client_ids)})

    def cancel_confirmation(self, refs: list[dict[str, str]]) -> dict[str, Any]:
        if self.params.get("cancel_confirmation") is False:
            return {}
        listen_key = str(self.params.get("listen_key") or cached_listen_key(self.params, self.signer))
        ws_base = str(self.params.get("ws_url") or self.params.get("ws_base_url") or DEFAULT_WS_BASE_URL).rstrip("/")
        ws_url = ws_base + "/" + listen_key if ws_base.endswith("/ws") else ws_base + "/ws/" + listen_key
        return {
            "venue": "aster",
            "ws_url": ws_url,
            "listen_key": listen_key,
            "client_order_ids": [ref["client_order_id"] for ref in refs],
        }

    def create_market_order(self, symbol: str, side: str, quantity: Decimal) -> tuple[int, Any]:
        body = self.market_order_body(symbol, side, quantity)
        request = urllib.request.Request(
            self.base_url + "/fapi/v3/order",
            data=body.encode(),
            method="POST",
            headers={"Content-Type": "application/x-www-form-urlencoded"},
        )
        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as response:
                return response.status, json.loads(response.read().decode() or "{}")
        except urllib.error.HTTPError as exc:
            raw = exc.read().decode(errors="replace")
            try:
                decoded = json.loads(raw)
            except json.JSONDecodeError:
                decoded = {"msg": raw}
            return exc.code, decoded

    def market_order_body(self, symbol: str, side: str, quantity: Decimal, client_order_id: str | None = None) -> str:
        values = {
            "symbol": symbol,
            "side": side,
            "type": "MARKET",
            "quantity": str(quantity.normalize()),
            "newClientOrderId": client_order_id or "pb_neutralize_" + str(time.time_ns())[-18:],
            "newOrderRespType": "ACK",
            "reduceOnly": "true",
        }
        return self.signer.sign(values)

    def position_snapshot(self, symbol: str) -> list[dict[str, str]]:
        _, body = self.signed("GET", "/fapi/v3/positionRisk", {"symbol": symbol})
        if not isinstance(body, list):
            require_ok(body)
            return []
        positions = []
        for position in body:
            amount = Decimal(str(first_present(position, "positionAmt", "positionAmtInAsset", "size") or "0"))
            if amount == 0:
                continue
            positions.append({
                "symbol": str(first_present(position, "symbol") or symbol),
                "position_side": str(first_present(position, "positionSide") or "BOTH"),
                "position_amt": str(amount),
            })
        return sorted(positions, key=lambda item: (item["symbol"], item["position_side"]))


def before_run(client: AsterClient, params: dict[str, Any], builder_params: dict[str, Any]) -> dict[str, Any]:
    symbol = str(builder_params.get("symbol", "BTCUSDT")).upper()
    position = client.position_snapshot(symbol)
    metadata = {"position": position}
    if bool(builder_params.get("cleanup_all_open_orders")):
        refs = order_refs_from_open_orders(client.open_orders(symbol), symbol)
    else:
        refs = planned_orders(params, builder_params)
        refs = open_cleanup_orders(client, refs, builder_params)
    if refs:
        cancelled = execute_cancel_orders(client, refs, metadata)
        cancelled_result = dict(cancelled.get("cleanup") or {})
        if not bool(cancelled_result.get("ok")):
            return cancelled
        metadata = dict(cancelled_result.get("metadata") or metadata)
    if bool(builder_params.get("neutralize_on_fill")) and signed_position_size(position, symbol) != 0:
        return cleanup_result(
            True,
            False,
            "Aster account has an existing position; close it before starting taker benchmark",
            metadata=metadata,
        )
    if refs:
        return cleanup_result(True, True, "cancel Aster benchmark orders by client order ID", metadata=metadata)
    return cleanup_result(False, True, "no stale Aster benchmark orders", metadata=metadata)


def after_run(client: AsterClient, params: dict[str, Any], builder_params: dict[str, Any]) -> dict[str, Any]:
    symbol = str(builder_params.get("symbol", "BTCUSDT")).upper()
    refs = result_orders(dict(params.get("result") or {}))
    remaining = wait_no_open_cleanup_orders(client, refs, builder_params)
    before_position = dict(params.get("run_metadata") or {}).get("position")
    after_position = client.position_snapshot(symbol)
    problems = []
    if remaining:
        problems.append(f"{len(remaining)} Aster benchmark orders still open")
    if before_position is not None and before_position != after_position:
        problems.append("Aster position changed during run")
    metadata = {"position_before": before_position, "position_after": after_position}
    if problems:
        return cleanup_result(True, False, "; ".join(problems), metadata=metadata)
    return cleanup_result(True, True, "no Aster benchmark orders open after run and position unchanged", metadata=metadata)


def after_sample(client: AsterClient, params: dict[str, Any], builder_params: dict[str, Any]) -> dict[str, Any]:
    refs = cleanup_orders(dict(params.get("metadata") or {}))
    if not refs:
        return cleanup_result(False, True, "no Aster cleanup_orders")
    remaining = wait_open_cleanup_orders(client, refs, builder_params)
    if remaining:
        return cancel_request(client, remaining, {})
    neutralize = neutralize_position(client, params, builder_params)
    if neutralize:
        return neutralize
    return cleanup_result(False, True, "no Aster cleanup action needed")


def cancel_request(client: AsterClient, refs: list[dict[str, Any]], metadata: dict[str, Any]) -> dict[str, Any]:
    orders = normalized_cancel_refs(refs)
    if not orders:
        return cleanup_result(False, True, "no Aster orders to cancel", metadata=metadata)
    headers = {"Content-Type": "application/x-www-form-urlencoded"}
    if len(orders) == 1:
        order = orders[0]
        return {
            "method": "DELETE",
            "headers": headers,
            "body": client.cancel_order_body(order["symbol"], order["client_order_id"]),
            "metadata": {"cleanup": "cancel_order", "orders": 1, "cancel_confirmation": client.cancel_confirmation(orders), **metadata},
        }
    return {
        "method": "DELETE",
        "url": client.base_url + "/fapi/v3/batchOrders",
        "headers": headers,
        "body": client.cancel_batch_body(orders),
        "metadata": {"cleanup": "cancel_batch_orders", "orders": len(orders), "cancel_confirmation": client.cancel_confirmation(orders), **metadata},
    }


def execute_cancel_orders(client: AsterClient, refs: list[dict[str, Any]], metadata: dict[str, Any]) -> dict[str, Any]:
    failed = []
    attempted = False
    for ref in normalized_cancel_refs(refs):
        symbol = ref["symbol"]
        client_id = ref["client_order_id"]
        attempted = True
        status, body = client.cancel_order(symbol, client_id)
        if status == 400 and isinstance(body, dict) and int(body.get("code") or 0) == -2011:
            continue
        if not response_ok(status, body):
            failed.append(f"{client_id}: {response_reason(body)}")
    if failed:
        return cleanup_result(True, False, "; ".join(failed), metadata=metadata)
    return cleanup_result(attempted, True, "cancel Aster benchmark orders by client order ID", metadata=metadata)


def normalized_cancel_refs(refs: list[dict[str, Any]]) -> list[dict[str, str]]:
    orders = []
    for ref in refs:
        symbol = str(ref.get("symbol") or "").upper()
        client_id = str(ref.get("client_order_id") or "")
        if symbol and client_id:
            orders.append({"symbol": symbol, "client_order_id": client_id})
    return orders


def planned_orders(params: dict[str, Any], builder_params: dict[str, Any]) -> list[dict[str, Any]]:
    run = dict(params.get("run") or {})
    run_id = run.get("run_id")
    if not run_id:
        return []
    scenario = run.get("scenario", "single")
    total = int(run.get("iterations") or 0) + int(run.get("warmups") or 0)
    warmups = int(run.get("warmups") or 0)
    batch_size = int(run.get("batch_size") or 1)
    count = batch_size if scenario == "batch" else 1
    order_params = dict(builder_params)
    order_params["run_id"] = run_id
    refs = []
    for index in range(total):
        req = {"iteration": index - warmups}
        for offset in range(count):
            refs.append({
                "venue": "aster",
                "symbol": str(order_params.get("symbol", "BTCUSDT")).upper(),
                "client_order_id": client_order_id(order_params, req, offset),
            })
    return refs


def result_orders(result: dict[str, Any]) -> list[dict[str, Any]]:
    return result_orders_for_venue(result, "aster")


def cleanup_orders(metadata: dict[str, Any]) -> list[dict[str, Any]]:
    return cleanup_orders_for_venue(metadata, "aster")


def open_cleanup_orders(client: AsterClient, refs: list[dict[str, Any]], params: dict[str, Any]) -> list[dict[str, Any]]:
    if not refs:
        return []
    by_symbol = {str(ref.get("symbol") or params.get("symbol", "BTCUSDT")).upper() for ref in refs}
    open_ids: set[tuple[str, str]] = set()
    for symbol in by_symbol:
        for order in client.open_orders(symbol):
            client_id = first_present(order, "clientOrderId", "origClientOrderId")
            if client_id not in (None, ""):
                open_ids.add((symbol, str(client_id)))
    return [ref for ref in refs if (str(ref.get("symbol")).upper(), str(ref.get("client_order_id"))) in open_ids]


def wait_open_cleanup_orders(client: AsterClient, refs: list[dict[str, Any]], params: dict[str, Any]) -> list[dict[str, Any]]:
    attempts = max(1, int(params.get("cleanup_poll_attempts", 5)))
    interval = max(0, int(params.get("cleanup_poll_interval_ms", 250))) / 1000
    for attempt in range(attempts):
        remaining = open_cleanup_orders(client, refs, params)
        if remaining or attempt == attempts - 1:
            return remaining
        time.sleep(interval)
    return []


def wait_no_open_cleanup_orders(client: AsterClient, refs: list[dict[str, Any]], params: dict[str, Any]) -> list[dict[str, Any]]:
    attempts = max(1, int(params.get("reconciliation_poll_attempts", 5)))
    interval = max(0, int(params.get("reconciliation_poll_interval_ms", 250))) / 1000
    remaining = []
    for attempt in range(attempts):
        remaining = open_cleanup_orders(client, refs, params)
        if not remaining or attempt == attempts - 1:
            return remaining
        time.sleep(interval)
    return remaining


def order_refs_from_open_orders(orders: list[dict[str, Any]], symbol: str) -> list[dict[str, str]]:
    refs = []
    for order in orders:
        client_id = first_present(order, "clientOrderId", "origClientOrderId")
        if client_id not in (None, ""):
            refs.append({"venue": "aster", "symbol": str(first_present(order, "symbol") or symbol), "client_order_id": str(client_id)})
    return refs


def neutralize_position(client: AsterClient, params: dict[str, Any], builder_params: dict[str, Any]) -> dict[str, Any] | None:
    if not bool(builder_params.get("neutralize_on_fill")):
        return None
    before_position = dict(params.get("run_metadata") or {}).get("position")
    if before_position is None:
        return None
    symbol = str(builder_params.get("symbol", "BTCUSDT")).upper()
    after_position = client.position_snapshot(symbol)
    delta = signed_position_size(after_position, symbol) - signed_position_size(before_position, symbol)
    if delta == 0:
        return None
    side = "SELL" if delta > 0 else "BUY"
    reconciliation = {"position_before": before_position, "position_after": after_position, "delta": str(delta)}
    client_order_id = "pb_neutralize_" + str(time.time_ns())[-18:]
    return {
        "method": "POST",
        "headers": {"Content-Type": "application/x-www-form-urlencoded"},
        "body": client.market_order_body(symbol, side, abs(delta), client_order_id),
        "metadata": {
            "cleanup": "neutralize_position",
            "cleanup_orders": [{"venue": "aster", "symbol": symbol, "client_order_id": client_order_id}],
            "reconciliation": reconciliation,
        },
    }


def signed_position_size(positions: list[dict[str, Any]], symbol: str) -> Decimal:
    total = Decimal("0")
    for position in positions or []:
        if str(position.get("symbol", "")).upper() != symbol:
            continue
        amount = Decimal(str(position.get("position_amt", "0") or "0"))
        side = str(position.get("position_side", "BOTH")).upper()
        if side == "SHORT":
            total -= abs(amount)
        elif side == "LONG":
            total += abs(amount)
        else:
            total += amount
    return total


def response_ok(status: int, body: Any) -> bool:
    if status < 200 or status >= 300:
        return False
    if isinstance(body, dict) and "code" in body:
        try:
            return int(body["code"]) >= 0
        except (TypeError, ValueError):
            return str(body["code"]).upper() in ("SUCCESS", "OK")
    return True


def require_ok(body: Any) -> None:
    if isinstance(body, dict) and "code" in body:
        try:
            if int(body["code"]) < 0:
                raise SystemExit(response_reason(body))
        except (TypeError, ValueError):
            if str(body["code"]).upper() not in ("SUCCESS", "OK"):
                raise SystemExit(response_reason(body))


def response_reason(body: Any) -> str:
    if isinstance(body, dict):
        for key in ("msg", "message", "error", "reason", "code"):
            value = body.get(key)
            if value not in (None, ""):
                return str(value)
    return "Aster API response was not successful"


def first_present(data: Any, *keys: str) -> Any:
    if not isinstance(data, dict):
        return None
    for key in keys:
        value = data.get(key)
        if value not in (None, ""):
            return value
    return None


if __name__ == "__main__":
    raise SystemExit(main())
