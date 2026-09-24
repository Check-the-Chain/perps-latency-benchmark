#!/usr/bin/env python3
"""Read or reserve Hyperliquid address request capacity."""

from __future__ import annotations

import json
import os
import sys
import time
import urllib.error
import urllib.request
from typing import Any


def main() -> int:
    try:
        from eth_account import Account
        from hyperliquid.utils.constants import MAINNET_API_URL
        from hyperliquid.utils.signing import sign_l1_action
    except ImportError as exc:
        raise SystemExit(
            "missing Hyperliquid SDK dependencies; run with "
            "`uv run --with hyperliquid-python-sdk --with eth-account python ...`"
        ) from exc

    mode = sys.argv[1] if len(sys.argv) > 1 else "status"
    wallet = Account.from_key(required_env("HYPERLIQUID_SECRET_KEY"))
    base_url = os.getenv("HYPERLIQUID_BASE_URL", MAINNET_API_URL).rstrip("/")
    if mode == "status":
        status = user_rate_limit(base_url, wallet.address)
        status["userAbstraction"] = user_abstraction(base_url, wallet.address)
        print(compact_json(status))
        return 0
    if mode == "reserve":
        if len(sys.argv) < 3:
            raise SystemExit("reserve requires a positive weight argument")
        weight = int(sys.argv[2])
        if weight <= 0:
            raise SystemExit("reserve weight must be positive")
        reserve_request_weight(base_url, wallet, sign_l1_action, weight)
        status = user_rate_limit(base_url, wallet.address)
        status["userAbstraction"] = user_abstraction(base_url, wallet.address)
        print(compact_json(status))
        return 0
    raise SystemExit(f"unknown mode {mode!r}")


def user_rate_limit(base_url: str, user: str) -> dict[str, Any]:
    return post_json(f"{base_url}/info", {"type": "userRateLimit", "user": user})


def user_abstraction(base_url: str, user: str) -> str:
    return str(post_json(f"{base_url}/info", {"type": "userAbstraction", "user": user}))


def reserve_request_weight(base_url: str, wallet: Any, sign_l1_action: Any, weight: int) -> dict[str, Any]:
    action = {"type": "reserveRequestWeight", "weight": weight}
    nonce = time.time_ns() // 1_000_000
    payload = {
        "action": action,
        "nonce": nonce,
        "signature": sign_l1_action(wallet, action, None, nonce, None, base_url == "https://api.hyperliquid.xyz"),
    }
    result = post_json(f"{base_url}/exchange", payload)
    if result.get("status") != "ok":
        raise SystemExit(f"reserveRequestWeight failed: {compact_json(result)}")
    return result


def post_json(url: str, payload: dict[str, Any]) -> dict[str, Any]:
    request = urllib.request.Request(
        url,
        data=compact_json(payload).encode(),
        headers={"Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(request, timeout=10) as response:
            return json.load(response)
    except urllib.error.HTTPError as exc:
        body = exc.read().decode(errors="replace")
        raise SystemExit(f"{url} returned HTTP {exc.code}: {body}") from exc


def required_env(key: str) -> str:
    value = os.getenv(key)
    if not value:
        raise SystemExit(f"missing {key}")
    return value


def compact_json(value: Any) -> str:
    return json.dumps(value, separators=(",", ":"), sort_keys=False)


if __name__ == "__main__":
    raise SystemExit(main())
