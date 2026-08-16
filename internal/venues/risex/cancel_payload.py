#!/usr/bin/env python3
"""Build signed RISEx cancel-order payloads for benchmark cleanup.

Resolving resting_order_id requires one HTTP GET per cleanup call (RISEx's
cancel endpoint needs resting_order_id, not the client_order_id or order_id
returned by place). That network call is acceptable here because cleanup runs
outside the timed benchmark path (see internal/cleanup).
"""

from __future__ import annotations

import json
import os
import sys
import urllib.parse
from typing import Any

from build_payload import (
    DEFAULT_BASE_URL,
    DEFAULT_WS_URL,
    compact_json,
    env_or_param,
    fetch_json,
    parse_eip712_domain,
    resolve_account_address,
    risex_action_hash,
    sign_verify_witness,
    sign_ws_auth_frame,
    starting_nonce_anchor,
)

_ROUTER_CONTEXT: dict[str, dict[str, Any]] = {}


def main() -> int:
    try:
        from eth_account import Account
        from eth_account.messages import encode_typed_data
    except ImportError as exc:
        raise SystemExit("missing RISEx cancel dependency; run with `uv run --with eth-account --with eth-utils python ...`") from exc

    for line in sys.stdin:
        if not line.strip():
            continue
        built = build(json.loads(line), Account, encode_typed_data)
        print(compact_json(built), flush=True)
    return 0


def build(req: dict[str, Any], Account: Any, encode_typed_data: Any) -> dict[str, Any]:
    params = dict(req.get("params") or {})
    builder_params = dict(params.get("builder_params") or {})
    phase = params.get("phase", "after_sample")

    if phase in ("before_run", "after_run"):
        # internal/cleanup's CommandAdapter calls BeforeRun/AfterRun for every
        # venue with cleanup enabled, unconditionally -- they carry no
        # order_refs (nothing has been placed yet, or the run result doesn't
        # track resting orders directly), so this can't reuse
        # cleanup_orders(params) the way after_sample does. Sweep whatever is
        # actually open on the account instead: it's the only way this phase
        # can serve its purpose as a safety net for orders a crashed/killed
        # run left behind (RISEx cancels one order per request -- see
        # after_sample below -- so this reports remaining count rather than
        # guaranteeing zero open orders in one call).
        return sweep_open_orders(builder_params, Account, encode_typed_data, phase)

    orders = cleanup_orders(params)
    if not orders:
        return {"cleanup": {"attempted": False, "ok": True, "description": "no RISEx cleanup_orders"}}

    base_url = str(builder_params.get("base_url", os.getenv("RISEX_BASE_URL", DEFAULT_BASE_URL))).rstrip("/")
    account_address = resolve_account_address(builder_params, Account)
    signer = Account.from_key(env_or_param(builder_params, "signer_private_key", "RISEX_SIGNER_PRIVATE_KEY"))

    resolved = resolve_resting_orders(base_url, account_address, orders)
    if not resolved:
        return {"cleanup": {"attempted": False, "ok": True, "description": "no RISEx orders still open for cleanup_orders"}}

    return cancel_one(base_url, account_address, signer, resolved, builder_params, encode_typed_data)


def cancel_one(base_url: str, account_address: str, signer: Any, resolved: list[dict[str, Any]], builder_params: dict[str, Any], encode_typed_data: Any) -> dict[str, Any]:
    """Build a cancel request for resolved[0]. RISEx cancels one order per
    request, so any further entries in `resolved` are reported as still
    outstanding rather than cancelled -- see the `orders_remaining` field.
    """
    router, eip712_domain = router_context(base_url)
    # Always fetch a fresh nonce_anchor rather than caching one across calls:
    # RISEx requires nonce_anchor to be exactly (current + 1), a strict
    # sequence position, not just "any unused value" (confirmed live -- see
    # README.md). Caching would race against build_payload.py's independent
    # process opening its own anchors. Cleanup is infrequent and already off
    # the timed path, so the extra fetch per call is cheap.
    nonce_anchor = starting_nonce_anchor(base_url, account_address, builder_params)

    target = resolved[0]
    action_hash = risex_action_hash(b"RISE_PERPS_CANCEL_ORDER_V1", 1, int(target["resting_order_id"]))
    permit = sign_verify_witness(signer, encode_typed_data, account_address, router, eip712_domain, action_hash, nonce_anchor, int(builder_params.get("deadline_secs", 3600)))

    body = {
        "market_id": target["market_id"],
        "order_id": target["order_id"],
        "permit": permit,
    }
    metadata: dict[str, Any] = {
        "cleanup": "orders_cancel",
        "client_order_id": target["client_order_id"],
        "resting_order_id": target["resting_order_id"],
        "orders_remaining": len(resolved) - 1,
    }
    if bool_param(builder_params, "cancel_confirmation", False):
        metadata["cancel_confirmation"] = {
            "venue": "risex",
            "ws_url": builder_params.get("ws_url", DEFAULT_WS_URL),
            "account": account_address,
            "market_id": target["market_id"],
            "client_order_ids": [target["client_order_id"]],
            "auth_v2": sign_ws_auth_frame(signer, encode_typed_data, account_address, eip712_domain, fetch_json(base_url + "/v1/auth/nonce")["data"]["nonce"]),
        }
    return {
        "method": "POST",
        "url": base_url + "/v1/orders/cancel",
        "headers": {"Content-Type": "application/json"},
        "body": compact_json(body),
        "metadata": metadata,
    }


def sweep_open_orders(builder_params: dict[str, Any], Account: Any, encode_typed_data: Any, phase: str) -> dict[str, Any]:
    base_url = str(builder_params.get("base_url", os.getenv("RISEX_BASE_URL", DEFAULT_BASE_URL))).rstrip("/")
    account_address = resolve_account_address(builder_params, Account)
    market_id = int(builder_params.get("market_id", 1))

    query = urllib.parse.urlencode({"account": account_address, "market_id": market_id})
    try:
        open_orders = fetch_json(f"{base_url}/v1/orders/open?{query}")["data"]["orders"]
    except (KeyError, TypeError):
        open_orders = []
    if not open_orders:
        return {"cleanup": {"attempted": False, "ok": True, "description": f"no RISEx orders open on market_id {market_id} at {phase}"}}

    signer = Account.from_key(env_or_param(builder_params, "signer_private_key", "RISEX_SIGNER_PRIVATE_KEY"))
    resolved = [
        {
            "market_id": market_id,
            "client_order_id": str(order.get("client_order_id") or ""),
            "order_id": order.get("order_id") or order.get("id"),
            "resting_order_id": order["resting_order_id"],
        }
        for order in open_orders
        if order.get("resting_order_id") is not None
    ]
    if not resolved:
        return {"cleanup": {"attempted": False, "ok": True, "description": f"no RISEx orders open on market_id {market_id} at {phase}"}}

    built = cancel_one(base_url, account_address, signer, resolved, builder_params, encode_typed_data)
    remaining = built["metadata"]["orders_remaining"]
    built["metadata"]["phase"] = phase
    if remaining > 0:
        # Only one order can be cancelled per Build() call (see cancel_one),
        # so a sweep that finds more than one open order can't clear them
        # all in a single before_run/after_run call. Recorded here in
        # `orders_remaining`/`description` so this is visible in run
        # metadata rather than silently reporting the account as clean.
        built["metadata"]["description"] = f"{remaining} RISEx order(s) still open on market_id {market_id} after {phase} swept one"
    return built


def cleanup_orders(params: dict[str, Any]) -> list[dict[str, Any]]:
    raw = params.get("order_refs") or []
    orders = [dict(order) for order in raw if dict(order).get("venue") == "risex"]
    if orders:
        return orders
    metadata = dict(params.get("metadata") or {})
    return [dict(order) for order in metadata.get("cleanup_orders") or [] if dict(order).get("venue") == "risex"]


def resolve_resting_orders(base_url: str, account: str, orders: list[dict[str, Any]]) -> list[dict[str, Any]]:
    wanted = {str(order["client_order_id"]): order for order in orders if order.get("client_order_id") is not None}
    if not wanted:
        return []
    resolved: list[dict[str, Any]] = []
    for market_id in sorted({int(order.get("market_id", 1)) for order in orders}):
        query = urllib.parse.urlencode({"account": account, "market_id": market_id})
        try:
            open_orders = fetch_json(f"{base_url}/v1/orders/open?{query}")["data"]["orders"]
        except (KeyError, TypeError):
            open_orders = []
        for open_order in open_orders:
            cid = str(open_order.get("client_order_id") or "")
            if cid in wanted and open_order.get("resting_order_id") is not None:
                resolved.append({
                    "market_id": market_id,
                    "client_order_id": cid,
                    "order_id": open_order.get("order_id") or open_order.get("id"),
                    "resting_order_id": open_order["resting_order_id"],
                })
    return resolved


def router_context(base_url: str) -> tuple[str, dict[str, Any]]:
    """Return (router, eip712_domain) for base_url, cached per process.

    This is a persistent_command process (one process handles every cleanup
    call for the whole benchmark run), and both values are static for the
    process's lifetime -- fetching them fresh on every call was pure waste
    (~2 redundant HTTP round trips per cleanup call). Mirrors the caching
    build_payload.py's ensure_context() already does for the same data.
    """
    cached = _ROUTER_CONTEXT.get(base_url)
    if cached is not None:
        return cached["router"], cached["eip712_domain"]
    domain = fetch_json(base_url + "/v1/auth/eip712-domain")["data"]
    system_config = fetch_json(base_url + "/v1/system/config")["data"]
    router = system_config["addresses"]["router"]
    eip712_domain = parse_eip712_domain(domain)
    _ROUTER_CONTEXT[base_url] = {"router": router, "eip712_domain": eip712_domain}
    return router, eip712_domain


def bool_param(params: dict[str, Any], key: str, default: bool) -> bool:
    value = params.get(key, default)
    if isinstance(value, bool):
        return value
    return str(value).lower() in ("1", "true", "yes", "on")


if __name__ == "__main__":
    raise SystemExit(main())
