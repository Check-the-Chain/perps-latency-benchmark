"""Shared cleanup-script result helpers."""

from __future__ import annotations

from typing import Any


def cleanup_result(attempted: bool, ok: bool, description: str, metadata: dict[str, Any] | None = None) -> dict[str, Any]:
    result = {"attempted": attempted, "ok": ok, "description": description}
    if not ok:
        result["error"] = description
    if metadata:
        result["metadata"] = metadata
    return {"cleanup": result}


def cleanup_orders_for_venue(metadata: dict[str, Any], venue: str) -> list[dict[str, Any]]:
    return [order for order in metadata.get("cleanup_orders") or [] if order.get("venue") == venue]


def result_orders_for_venue(result: dict[str, Any], venue: str) -> list[dict[str, Any]]:
    refs = []
    for sample in result.get("samples") or []:
        typed_refs = [order for order in sample.get("order_refs") or [] if order.get("venue") == venue]
        if typed_refs:
            refs.extend(typed_refs)
        else:
            refs.extend(cleanup_orders_for_venue(dict(sample.get("metadata") or {}), venue))
    return refs
