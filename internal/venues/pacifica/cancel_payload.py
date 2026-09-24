#!/usr/bin/env python3
"""Build Pacifica cleanup cancel payloads outside the measured latency window."""

from __future__ import annotations

import json
import sys
import time
import uuid
from pathlib import Path
from typing import Any

sys.path.append(str(Path(__file__).resolve().parents[1]))
from cleanup_common import cleanup_orders_for_venue, cleanup_result, result_orders_for_venue

from build_payload import DEFAULT_WS_URL, PacificaSigner, compact_json, expiry_window, load_signer


def main() -> int:
    for line in sys.stdin:
        if not line.strip():
            continue
        print(compact_json(build(json.loads(line))), flush=True)
    return 0


def build(req: dict[str, Any]) -> dict[str, Any]:
    params = dict(req.get("params") or {})
    builder_params = dict(params.get("builder_params") or {})
    phase = params.get("phase", "after_sample")
    if phase != "after_sample":
        return {
            "cleanup": cleanup_result(
                False,
                True,
                "Pacifica cleanup currently prepares per-sample cancel_order requests; stale-order discovery and final position reconciliation require authenticated account REST reads",
            )["cleanup"]
        }
    refs = cleanup_orders(dict(params.get("metadata") or {}))
    if not refs:
        refs = cleanup_orders_for_venue(dict(params.get("metadata") or {}), "pacifica")
    if not refs:
        refs = result_orders(dict(params.get("sample") or {}))
    refs = normalized_refs(refs, builder_params)
    if not refs:
        return cleanup_result(False, True, "no Pacifica cleanup_orders")

    signer = load_signer(builder_params)
    timestamp = int(builder_params.get("timestamp") or (time.time() * 1_000))
    actions = [cancel_action(builder_params, signer, timestamp, ref) for ref in refs]
    metadata = {
        "cancel_confirmation": {
            "venue": "pacifica",
            "ws_url": builder_params.get("ws_url", DEFAULT_WS_URL),
            "account": signer.account,
            "client_order_ids": [ref["client_order_id"] for ref in refs],
        }
    }
    if len(actions) == 1:
        body = compact_json({"id": str(uuid.uuid4()), "params": {"cancel_order": actions[0]["data"]}})
    else:
        body = compact_json({"id": str(uuid.uuid4()), "params": {"batch_orders": {"actions": actions}}})
    return {
        "ws_url": builder_params.get("ws_url", DEFAULT_WS_URL),
        "ws_body": body,
        "metadata": metadata,
    }


def cancel_action(params: dict[str, Any], signer: PacificaSigner, timestamp: int, ref: dict[str, str]) -> dict[str, Any]:
    payload = {
        "symbol": ref["symbol"],
        "client_order_id": ref["client_order_id"],
    }
    signature = signer.sign("cancel_order", payload, timestamp, expiry_window(params))
    return {
        "type": "Cancel",
        "data": signer.request("cancel_order", payload, timestamp, signature, expiry_window(params)),
    }


def normalized_refs(refs: list[dict[str, Any]], params: dict[str, Any]) -> list[dict[str, str]]:
    symbol = str(params.get("symbol", "BTC")).upper()
    out: list[dict[str, str]] = []
    for ref in refs:
        if str(ref.get("venue", "pacifica")) != "pacifica":
            continue
        client_order_id = ref.get("client_order_id") or ref.get("clientOrderId") or ref.get("I")
        if not client_order_id:
            continue
        out.append({
            "symbol": str(ref.get("symbol") or symbol).upper(),
            "client_order_id": str(client_order_id),
        })
    return out


def cleanup_orders(metadata: dict[str, Any]) -> list[dict[str, Any]]:
    return cleanup_orders_for_venue(metadata, "pacifica")


def result_orders(sample: dict[str, Any]) -> list[dict[str, Any]]:
    return result_orders_for_venue(sample, "pacifica")


if __name__ == "__main__":
    raise SystemExit(main())
