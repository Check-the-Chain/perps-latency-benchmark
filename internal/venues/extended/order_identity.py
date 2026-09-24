"""Extended benchmark order identity and cleanup refs."""

from __future__ import annotations

import hashlib
import time
from typing import Any


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


def cleanup_ref(params: dict[str, Any], external_id: str) -> dict[str, str]:
    return {
        "venue": "extended",
        "market": str(params.get("market", "BTC-USD")),
        "external_id": external_id,
    }


def cleanup_refs_for_orders(params: dict[str, Any], external_ids: list[str]) -> list[dict[str, str]]:
    return [cleanup_ref(params, external_id) for external_id in external_ids]


def planned_cleanup_refs(params: dict[str, Any], builder_params: dict[str, Any]) -> list[dict[str, str]]:
    run = dict(params.get("run") or {})
    run_id = run.get("run_id")
    if not run_id:
        return []
    total = int(run.get("iterations") or 0) + int(run.get("warmups") or 0)
    warmups = int(run.get("warmups") or 0)
    scenario = str(run.get("scenario") or "single")
    batch_size = int(run.get("batch_size") or 1)
    orders_per_iteration = batch_size if scenario == "batch" else 1

    order_params = dict(builder_params)
    order_params["run_id"] = run_id
    refs = []
    for index in range(total):
        req = {"iteration": index - warmups}
        for offset in range(orders_per_iteration):
            refs.append(cleanup_ref(
                order_params,
                order_external_id(order_params, req, offset),
            ))
    return refs


def is_fill_likely(params: dict[str, Any]) -> bool:
    order_type = str(params.get("order_type", "limit")).lower()
    tif = str(params.get("time_in_force", "GTT")).upper()
    return order_type == "market" or tif in ("IOC", "FOK")
