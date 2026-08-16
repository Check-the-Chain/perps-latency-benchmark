#!/usr/bin/env python3
"""Build signed RISEx order payloads for perps-bench.

The script reads payload.Request JSON lines from stdin and writes
payload.Built JSON lines to stdout. Per-order signing (the VerifyWitness
permit) is fully local; the only network calls are one-time setup calls made
lazily on the first request of a process lifetime (EIP-712 domain, router
address, market tick/step config, starting nonce anchor), which the benchmark's
warmup iterations are expected to absorb. See README.md for the RISEx signing
scheme and the order_data bit-packing caveat.
"""

from __future__ import annotations

import json
import os
import sys
import time
import urllib.error
import urllib.request
from decimal import ROUND_DOWN, Decimal
from typing import Any

DEFAULT_BASE_URL = "https://api.rise.trade"
DEFAULT_WS_URL = "wss://ws.rise.trade/ws"
DEFAULT_CHAIN_ID = 4153

_CONTEXT: dict[str, Any] = {}


def main() -> int:
    try:
        from eth_account import Account
        from eth_account.messages import encode_typed_data
    except ImportError as exc:
        raise SystemExit("missing RISEx builder dependency; run with `uv run --with eth-account --with eth-utils python ...`") from exc

    for line in sys.stdin:
        if not line.strip():
            continue
        built = build(json.loads(line), Account, encode_typed_data)
        print(compact_json(built), flush=True)
    return 0


def build(req: dict[str, Any], Account: Any, encode_typed_data: Any) -> dict[str, Any]:
    if req.get("scenario", "single") == "batch":
        # RISEx has no documented native multi-place-order endpoint, and
        # unlike Nado/Extended (whose nonces are time-based and safe to
        # submit concurrently), RISEx's nonce_anchor is a strict per-account
        # on-chain sequence position. Confirmed live: firing N single-order
        # requests concurrently as a workaround does not work here -- in one
        # live batch_size=5 test, only 1 of 5 concurrently-submitted orders
        # actually succeeded, the other 4 were silently rejected server-side,
        # while the benchmark still reported the sample as ok (DoParallelFastest
        # only tracks the fastest of the N requests, not whether all N
        # succeeded). Refuse rather than produce misleading numbers, matching
        # edgeX's precedent for venues with no real batch endpoint. See
        # internal/venues/risex/README.md.
        raise SystemExit("RISEx builder does not support the batch scenario: no native multi-order endpoint, and concurrent single-order submission is confirmed unreliable due to RISEx's strict sequential nonce_anchor. See internal/venues/risex/README.md.")

    params = dict(req.get("params") or {})
    ctx = ensure_context(params, Account)

    # Fetch the starting nonce_anchor once per Build() call (fresh, so it
    # can't go stale from another process's activity between builds -- see
    # open_next_nonce_anchor).
    ctx["nonce_anchor"] = None
    order = signed_place_order(ctx, params, req, encode_typed_data, 0)

    cleanup_orders = [{"venue": "risex", "market_id": ctx["market_id"], "client_order_id": order["client_order_id"]}]
    metadata = {
        "builder": "risex-eip712-permit",
        "market_id": ctx["market_id"],
        "symbol": params.get("symbol"),
        "side": ctx["side"],
        "order_type": normalized_order_type(params),
        "client_order_id": order["client_order_id"],
        "client_order_ids": [order["client_order_id"]],
        "cleanup_orders": cleanup_orders,
        "confirmation": confirmation_metadata(params, ctx, [order["client_order_id"]], encode_typed_data),
    }
    return {
        "headers": {"Content-Type": "application/json"},
        "body": order["body"],
        "metadata": metadata,
    }


def signed_place_order(ctx: dict[str, Any], params: dict[str, Any], req: dict[str, Any], encode_typed_data: Any, offset: int) -> dict[str, Any]:
    market = ctx["market"]
    market_id = ctx["market_id"]
    side = ctx["side"]
    post_only = bool_param(params, "post_only", True)
    reduce_only = bool_param(params, "reduce_only", False)
    stp_mode = int(params.get("stp_mode", 0))
    order_type_code = order_type_code_for(normalized_order_type(params))
    tif_code = time_in_force_code_for(params)

    price_ticks = price_to_ticks(params["price"], market, offset, side)
    size_steps = amount_to_steps(params["amount"], market)
    client_order_id = derive_client_order_id(params, req, offset)
    builder_id = int(params.get("builder_id", 0))
    builder_fee_bps = int(params.get("builder_fee_bps", 0))
    ttl_units = int(params.get("ttl_units", 0))

    order_data = pack_order_data(market_id, size_steps, price_ticks, side, post_only, order_type_code, tif_code)
    # header_flags and the action hash's optional-field words (builderId,
    # clientOrderId, ttlUnits) must match what the router derives from the
    # request body it actually receives -- per RISEx's published spec
    # (https://developer.rise.trade/reference/integration.md), header_flags
    # bit 0x02/0x04/0x10 is set exactly when builder_id/client_order_id/
    # ttl_units is nonzero, and builderFeeBps is the only word omitted from
    # the hash rather than zero-padded (only included when builder_fee_bps >
    # 0). A prior version hardcoded header_flags=0x01 and clientOrderId=0
    # while still sending a real nonzero client_order_id in the body, so the
    # router's independently-derived hash never matched what we signed --
    # confirmed live: every PlaceOrder reverted with SignerNotAuthorized,
    # citing a different nonsensical recovered address on each call (which
    # is exactly what a hash mismatch does to ecrecover). See README.md.
    header_flags = 0x01
    if builder_id != 0:
        header_flags |= 0x02
    if client_order_id:
        header_flags |= 0x04
    if ttl_units != 0:
        header_flags |= 0x10
    hash_words = [header_flags, order_data, builder_id]
    if builder_fee_bps > 0:
        hash_words.append(builder_fee_bps)
    hash_words.extend([int(client_order_id), ttl_units])
    action_hash = risex_action_hash(b"RISE_PERPS_PLACE_ORDER_V1", *hash_words)
    permit = sign_permit(ctx, encode_typed_data, action_hash, int(params.get("deadline_secs", 3600)))

    body = {
        "market_id": market_id,
        "size_steps": size_steps,
        "price_ticks": price_ticks,
        "side": side,
        "post_only": post_only,
        "reduce_only": reduce_only,
        "stp_mode": stp_mode,
        "order_type": order_type_code,
        "time_in_force": tif_code,
        "builder_id": builder_id,
        "builder_fee_bps": builder_fee_bps,
        "client_order_id": client_order_id,
        "ttl_units": ttl_units,
        "permit": permit,
    }
    return {"body": compact_json(body), "client_order_id": client_order_id}


WS_AUTH_REFRESH_SECS = 240  # refresh before RISEx's ~5-minute auth nonce window elapses


def confirmation_metadata(params: dict[str, Any], ctx: dict[str, Any], client_order_ids: list[str], encode_typed_data: Any) -> dict[str, Any]:
    if params.get("confirmation") is not True:
        return {}
    return {
        "venue": "risex",
        "ws_url": params.get("ws_url", DEFAULT_WS_URL),
        "account": ctx["account"],
        "signer": ctx["signer"],
        "market_id": ctx["market_id"],
        "client_order_ids": client_order_ids,
        "order_type": normalized_order_type(params),
        "auth_v2": ensure_ws_auth(ctx, encode_typed_data),
    }


def ensure_ws_auth(ctx: dict[str, Any], encode_typed_data: Any) -> dict[str, Any]:
    """Build (and cache) the auth_v2 frame for the private orders WebSocket.

    RISEx's WS auth requires a fresh server-issued nonce per signature
    (GET /v1/auth/nonce), unlike order permits which are fully local. Fetching
    that nonce on every Build() call would put a network round-trip in the
    timed request path, so the signed frame is cached and only refreshed
    every WS_AUTH_REFRESH_SECS, well inside the ~5-minute nonce validity
    window docs describe. The refresh itself still costs one request; it
    lands on whichever iteration happens to trigger it, same tradeoff Nado's
    per-call stream-auth signing makes (see nado/build_payload.py).
    """
    cached = ctx.get("ws_auth")
    if cached is not None and time.time() < ctx.get("ws_auth_refresh_at", 0):
        return cached
    nonce = fetch_json(ctx["base_url"] + "/v1/auth/nonce")["data"]["nonce"]
    auth = sign_ws_auth_frame(ctx["signer_account"], encode_typed_data, ctx["account"], ctx["eip712_domain"], nonce)
    auth["expires_at"] = str(int(time.time()) + WS_AUTH_REFRESH_SECS)
    ctx["ws_auth"] = auth
    ctx["ws_auth_refresh_at"] = time.time() + WS_AUTH_REFRESH_SECS
    return auth


def sign_ws_auth_frame(signer_account: Any, encode_typed_data: Any, account: str, eip712_domain: dict[str, Any], nonce: Any) -> dict[str, Any]:
    """Sign the auth_v2 frame for the private orders WebSocket (RegisterV2).
    Shared by build_payload.py (cached, see ensure_ws_auth) and
    cancel_payload.py (uncached -- cleanup is infrequent enough not to need
    it).
    """
    message = "WebSocket Authentication"
    typed_data = {
        "types": {
            "EIP712Domain": EIP712_DOMAIN_TYPES,
            "RegisterV2": [
                {"name": "signer", "type": "address"},
                {"name": "message", "type": "string"},
                {"name": "nonce", "type": "uint256"},
            ],
        },
        "primaryType": "RegisterV2",
        "domain": eip712_domain,
        "message": {
            "signer": signer_account.address,
            "message": message,
            "nonce": parse_nonce(nonce),
        },
    }
    signed = signer_account.sign_message(encode_typed_data(full_message=typed_data))
    return {
        "account": account,
        "signer": signer_account.address,
        "message": message,
        "nonce": nonce,
        "signature": hex0x(signed.signature.hex()),
    }


def hex0x(value: str) -> str:
    return value if value.startswith("0x") else "0x" + value


def parse_nonce(nonce: Any) -> int:
    """GET /v1/auth/nonce returns a bare 32-byte hex string (no 0x prefix)."""
    if isinstance(nonce, int):
        return nonce
    text = str(nonce)
    return int(text[2:] if text.startswith("0x") else text, 16)


# --- one-time setup (network I/O confined to the first call per process) ---


def ensure_context(params: dict[str, Any], Account: Any) -> dict[str, Any]:
    global _CONTEXT
    if _CONTEXT:
        _CONTEXT["market_id"] = int(params.get("market_id", _CONTEXT["market_id"]))
        _CONTEXT["market"] = market_config(_CONTEXT, _CONTEXT["market_id"])
        _CONTEXT["side"] = side_code(params)
        return _CONTEXT

    base_url = str(params.get("base_url", os.getenv("RISEX_BASE_URL", DEFAULT_BASE_URL))).rstrip("/")
    account_address = resolve_account_address(params, Account)
    signer_key = env_or_param(params, "signer_private_key", "RISEX_SIGNER_PRIVATE_KEY")
    signer = Account.from_key(signer_key)

    domain = fetch_json(base_url + "/v1/auth/eip712-domain")["data"]
    system_config = fetch_json(base_url + "/v1/system/config")["data"]
    router = system_config["addresses"]["router"]

    market_id = int(params.get("market_id", 1))

    _CONTEXT = {
        "base_url": base_url,
        "account": account_address,
        "signer": signer.address,
        "signer_account": signer,
        "router": router,
        "eip712_domain": parse_eip712_domain(domain),
        "market_id": market_id,
        "markets": {},
    }
    _CONTEXT["market"] = market_config(_CONTEXT, market_id)
    _CONTEXT["side"] = side_code(params)
    return _CONTEXT


def market_config(ctx: dict[str, Any], market_id: int) -> dict[str, Any]:
    cache = ctx["markets"]
    if market_id in cache:
        return cache[market_id]
    decoded = fetch_json(ctx["base_url"] + "/v1/markets")["data"]["markets"]
    for entry in decoded:
        cache[int(entry["market_id"])] = entry
    if market_id not in cache:
        raise SystemExit(f"risex market_id {market_id} not found in /v1/markets")
    return cache[market_id]


def starting_nonce_anchor(base_url: str, account: str, params: dict[str, Any]) -> int:
    """Pick the next valid nonce_anchor to open.

    Confirmed against live RISEx mainnet: the contract rejects any anchor
    other than exactly (current nonce_anchor + 1) with
    InvalidNonceAnchor(account, given, expected) -- it is a strict sequence
    position, not just "any unused value". Once opened, an anchor's 208
    nonceBitmap slots (0-207) can all be consumed locally with no further
    network calls; only opening a *new* anchor needs a fresh nonce-state
    fetch. See README.md for why cancel_payload.py always fetches fresh here
    rather than caching a starting anchor the way build_payload.py does.
    """
    override = params.get("nonce_anchor") or os.getenv("RISEX_NONCE_ANCHOR")
    if override:
        return int(override)
    state = fetch_json(base_url + f"/v1/nonce-state/{account}")["data"]
    return int(state["nonce_anchor"]) + 1


def fetch_json(url: str) -> Any:
    request = urllib.request.Request(url, headers={"User-Agent": "perps-latency-benchmark"})
    try:
        with urllib.request.urlopen(request, timeout=15) as response:
            return json.loads(response.read().decode())
    except urllib.error.HTTPError as exc:
        raise SystemExit(f"risex setup request {url} failed: HTTP {exc.code} {exc.read().decode(errors='replace')}") from exc


# --- permit signing (local only, no network) --------------------------------


EIP712_DOMAIN_TYPES = [
    {"name": "name", "type": "string"},
    {"name": "version", "type": "string"},
    {"name": "chainId", "type": "uint256"},
    {"name": "verifyingContract", "type": "address"},
]


def parse_eip712_domain(domain: dict[str, Any]) -> dict[str, Any]:
    """Map GET /v1/auth/eip712-domain's response onto EIP712Domain fields."""
    return {
        "name": domain["name"],
        "version": domain["version"],
        "chainId": int(domain["chain_id"]),
        "verifyingContract": domain["verifying_contract"],
    }


def sign_verify_witness(signer_account: Any, encode_typed_data: Any, account: str, target: str, eip712_domain: dict[str, Any], action_hash: str, nonce_anchor: int, deadline_secs: int) -> dict[str, Any]:
    """Sign a VerifyWitness permit: the EIP-712 authorization RISEx requires
    on every order/cancel. nonce_bitmap_index is always 0 -- each permit
    opens its own nonce_anchor rather than sharing one across bits (see
    open_next_nonce_anchor). Shared by build_payload.py (place) and
    cancel_payload.py (cancel).
    """
    deadline = int(time.time()) + deadline_secs
    typed_data = {
        "types": {
            "EIP712Domain": EIP712_DOMAIN_TYPES,
            "VerifyWitness": [
                {"name": "account", "type": "address"},
                {"name": "target", "type": "address"},
                {"name": "hash", "type": "bytes32"},
                {"name": "nonceAnchor", "type": "uint48"},
                {"name": "nonceBitmap", "type": "uint8"},
                {"name": "deadline", "type": "uint32"},
            ],
        },
        "primaryType": "VerifyWitness",
        "domain": eip712_domain,
        "message": {
            "account": account,
            "target": target,
            "hash": action_hash,
            "nonceAnchor": nonce_anchor,
            "nonceBitmap": 0,
            "deadline": deadline,
        },
    }
    signed = signer_account.sign_message(encode_typed_data(full_message=typed_data))
    r = signed.r.to_bytes(32, "big")
    s = bytearray(signed.s.to_bytes(32, "big"))
    if signed.v == 28:
        s[0] |= 0x80
    return {
        "account": account,
        "signer": signer_account.address,
        "nonce_anchor": str(nonce_anchor),
        "nonce_bitmap_index": 0,
        "deadline": deadline,
        "signature": base64_encode(r + bytes(s)),
    }


def sign_permit(ctx: dict[str, Any], encode_typed_data: Any, action_hash: str, deadline_secs: int) -> dict[str, Any]:
    open_next_nonce_anchor(ctx)
    return sign_verify_witness(ctx["signer_account"], encode_typed_data, ctx["account"], ctx["router"], ctx["eip712_domain"], action_hash, ctx["nonce_anchor"], deadline_secs)


def open_next_nonce_anchor(ctx: dict[str, Any]) -> None:
    """Assign the next nonce_anchor for a permit about to be signed.

    RISEx requires nonce_anchor to be exactly (current + 1) -- a strict
    sequence position, not "any unused value" (confirmed live via
    InvalidNonceAnchor reverts). An earlier version cached one anchor per
    *process* and advanced only a local bitmap index to avoid a network
    call here, but that raced with cancel_payload.py's cleanup calls (a
    separate process that also opens fresh anchors): once cleanup opened
    the very next anchor for its own cancel, this process's cached value
    went stale and every subsequent order in the same run reverted with
    InvalidNonceAnchor -- confirmed live on any run with more than one
    sample and cleanup enabled. Every permit now opens its own anchor
    (nonce_bitmap_index is always 0 -- see sign_verify_witness), so there's
    no bitmap index to advance, only the anchor itself.

    build() resets ctx["nonce_anchor"] to None at the start of every
    Build() call, so the first order signed in a call fetches fresh here
    (safe for the benchmark's reported latency: this runs inside Build(),
    tracked separately as `prepared_ns` and never counted in
    network_ns/latency_ms -- see internal/bench/runner.go). Later orders in
    the *same* Build() call (batch scenario) reuse that fetch and just
    increment locally: batch orders are all signed here before any of them
    are submitted, so re-fetching per order would see the same unchanged
    on-chain value and hand out duplicate anchors. See README.md.
    """
    if ctx.get("nonce_anchor") is None:
        ctx["nonce_anchor"] = starting_nonce_anchor(ctx["base_url"], ctx["account"], {})
    else:
        ctx["nonce_anchor"] += 1


def risex_action_hash(tag: bytes, *words: int) -> str:
    from eth_utils import keccak

    packed = keccak(tag)
    for value in words:
        packed += int(value).to_bytes(32, "big")
    return "0x" + keccak(packed).hex()


# --- order_data bit-packing --------------------------------------------------
#
# Reverse-engineered from RISEx's single published Python example (a
# post-only, GTC, limit order) at
# https://developer.rise.trade/reference/integration.md. That example never
# varies order_type, time_in_force, reduce_only, or side, so this packer only
# supports the exact combination that example demonstrates and raises for
# everything else rather than guess at undocumented bit positions. Verify any
# relaxation of these checks against a live testnet order first (see
# README.md).


def pack_order_data(market_id: int, size_steps: int, price_ticks: int, side: int, post_only: bool, order_type: int, time_in_force: int) -> int:
    if order_type != 1:
        raise SystemExit("risex order_data packing is only verified for order_type=limit (1); see risex/README.md")
    if time_in_force != 0:
        raise SystemExit("risex order_data packing is only verified for time_in_force=GTC (0); see risex/README.md")
    if side not in (0, 1):
        raise SystemExit("risex side must be 0 (buy) or 1 (sell)")
    flags = side | ((1 << 1) if post_only else 0) | (1 << 5)
    return (market_id << 70) | (size_steps << 38) | (price_ticks << 14) | (flags << 6) | (1 << 1)


# --- param parsing ------------------------------------------------------------


def side_code(params: dict[str, Any]) -> int:
    text = str(params.get("side", "buy")).lower()
    if text in ("sell", "ask", "short", "1"):
        return 1
    return 0


def normalized_order_type(params: dict[str, Any]) -> str:
    text = str(params.get("order_type", "limit")).lower().replace("-", "_")
    if text in ("alo", "maker", "post_only"):
        return "limit"
    return text


def order_type_code_for(order_type: str) -> int:
    return {"market": 0, "limit": 1}.get(order_type, 1)


def time_in_force_code_for(params: dict[str, Any]) -> int:
    text = str(params.get("time_in_force", "gtc")).lower()
    return {"gtc": 0, "gtt": 1, "fok": 2, "ioc": 3}.get(text, 0)


def price_to_ticks(price: Any, market: dict[str, Any], offset: int, side: int) -> int:
    step_price = Decimal(str(market["config"]["step_price"]))
    value = Decimal(str(price))
    if offset:
        step = step_price * offset
        value = value - step if side == 0 else value + step
    ticks = (value / step_price).to_integral_value(rounding=ROUND_DOWN)
    return int(ticks)


def amount_to_steps(amount: Any, market: dict[str, Any]) -> int:
    step_size = Decimal(str(market["config"]["step_size"]))
    value = Decimal(str(amount))
    steps = (value / step_size).to_integral_value(rounding=ROUND_DOWN)
    return int(steps)


def derive_client_order_id(params: dict[str, Any], req: dict[str, Any], offset: int) -> str:
    if params.get("client_order_id") is not None:
        return str(int(str(params["client_order_id"]), 0) + offset)
    run_id = params.get("run_id")
    if run_id:
        import hashlib

        seed = f"{run_id}:{req.get('iteration', 0)}:{offset}:{time.time_ns()}".encode()
        value = int.from_bytes(hashlib.blake2b(seed, digest_size=8).digest(), "big")
    else:
        value = time.time_ns()
    return str(value & 0x7FFFFFFFFFFFFFFF)


def bool_param(params: dict[str, Any], key: str, default: bool) -> bool:
    value = params.get(key, default)
    if isinstance(value, bool):
        return value
    return str(value).lower() in ("1", "true", "yes", "on")


def env_or_param(params: dict[str, Any], key: str, env_key: str) -> str:
    value = params.get(key) or os.getenv(env_key)
    if not value:
        raise SystemExit(f"missing {key}; set params.{key} or {env_key}")
    return str(value)


def resolve_account_address(params: dict[str, Any], Account: Any) -> str:
    """Return the main account's checksummed address.

    Only the address is ever needed here for order/cancel signing -- the
    session signer does all the EIP-712 signing (see README.md's auth model
    section), and RISEx's own website UI can complete signer registration
    without ever exposing the main account's raw key (e.g. for a hardware
    wallet). Prefer RISEX_PRIVATE_KEY if you have it; otherwise set
    RISEX_ACCOUNT_ADDRESS.
    """
    private_key = params.get("private_key") or os.getenv("RISEX_PRIVATE_KEY")
    if private_key:
        return str(Account.from_key(private_key).address)
    address = params.get("account_address") or os.getenv("RISEX_ACCOUNT_ADDRESS")
    if address:
        from eth_utils import to_checksum_address

        return str(to_checksum_address(address))
    raise SystemExit("missing RISEx account; set RISEX_PRIVATE_KEY or RISEX_ACCOUNT_ADDRESS")


def base64_encode(data: bytes) -> str:
    import base64

    return base64.b64encode(data).decode()


def compact_json(value: Any) -> str:
    return json.dumps(value, separators=(",", ":"), sort_keys=False)


if __name__ == "__main__":
    raise SystemExit(main())
