"""Shared cleanup-script result helpers.

cleanup_result, cleanup_orders_for_venue and result_orders_for_venue are
mirrored in JavaScript by internal/venues/bullet/cancel_payload.mjs, whose
builder runs under Node and cannot import this module. Changing the shape or
semantics of those three helpers means changing the mirror too.
"""

from __future__ import annotations

from typing import Any


def cleanup_result(attempted: bool, ok: bool, description: str, metadata: dict[str, Any] | None = None) -> dict[str, Any]:
    result = {"attempted": attempted, "ok": ok, "description": description}
    if not ok:
        result["error"] = description
    if metadata:
        result["metadata"] = metadata
    return {"cleanup": result}


def flat_position_guard(venue: str, position: list[dict[str, Any]], params: dict[str, Any]) -> dict[str, Any] | None:
    if (
        not position
        or bool_param(params, "allow_preexisting_position")
        or not guard_preexisting_position(params)
    ):
        return None
    return cleanup_result(
        False,
        False,
        f"{venue} account has preexisting position; close it before starting benchmark or set allow_preexisting_position=true",
        metadata={
            "position": position,
            "blocked_by_position": True,
            "guard": "flat_position",
        },
    )


def guard_preexisting_position(params: dict[str, Any]) -> bool:
    return bool_param(params, "guard_preexisting_position") or bool_param(params, "neutralize_on_fill")


def self_heal_preexisting_position(params: dict[str, Any]) -> bool:
    return guard_preexisting_position(params) and bool_param(params, "neutralize_preexisting_position")


def mark_self_heal_position(payload: dict[str, Any], venue: str, position: list[dict[str, Any]]) -> dict[str, Any]:
    metadata = dict(payload.get("metadata") or {})
    metadata["cleanup"] = metadata.get("cleanup") or "neutralize_position"
    metadata["self_heal_position"] = True
    metadata["restart_before_measurement"] = True
    metadata["venue"] = venue
    metadata["preexisting_position"] = position
    payload["metadata"] = metadata
    return payload


def bool_param(params: dict[str, Any], key: str) -> bool:
    value = params.get(key)
    if isinstance(value, bool):
        return value
    if value is None:
        return False
    return str(value).strip().lower() in {"1", "true", "yes", "on"}


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
