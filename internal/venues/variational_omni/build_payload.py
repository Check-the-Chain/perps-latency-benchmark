#!/usr/bin/env python3
"""Build signed Variational API payloads for perps-bench.

The builder prepares HMAC-signed HTTP requests without doing network work. The
default action is an authenticated /status smoke check. Set
params.action=create_rfq only after Variational confirms live API access and the
target company IDs to use.
"""

from __future__ import annotations

import hashlib
import hmac
import json
import os
import sys
import time
import urllib.parse
from datetime import datetime, timedelta, timezone
from typing import Any


DEFAULT_BASE_URL = "https://api.variational.io/v1"


def main() -> int:
    for line in sys.stdin:
        if not line.strip():
            continue
        built = build(json.loads(line))
        print(compact_json(built), flush=True)
    return 0


def build(req: dict[str, Any]) -> dict[str, Any]:
    params = dict(req.get("params") or {})
    action = str(params.get("action", "status")).lower()

    if action == "status":
        return signed_request(params, "GET", "/status", None, metadata={"action": action, "order_type": "status"})
    if action == "create_rfq":
        payload = create_rfq_payload(params)
        metadata = {
            "action": action,
            "order_type": "rfq",
            "rfq_expires_at": payload["expires_at"],
            "speed_bump_ns": 0,
            "speed_bump_ms": 0,
            "speed_bump_source": "Variational public docs do not document a fixed order-entry speed bump",
        }
        return signed_request(params, "POST", "/rfqs/new", payload, metadata=metadata)
    if action == "accept_quote":
        payload = {
            "rfq_id": required(params, "rfq_id", "VARIATIONAL_RFQ_ID"),
            "parent_quote_id": required(params, "parent_quote_id", "VARIATIONAL_PARENT_QUOTE_ID"),
            "side": str(params.get("side", "buy")).lower(),
        }
        metadata = {
            "action": action,
            "order_type": "rfq_accept",
            "speed_bump_ns": 0,
            "speed_bump_ms": 0,
            "speed_bump_source": "Variational public docs do not document a fixed order-entry speed bump",
        }
        return signed_request(params, "POST", "/quotes/accept", payload, metadata=metadata)
    if action == "cancel_rfq":
        payload = {"id": required(params, "rfq_id", "VARIATIONAL_RFQ_ID")}
        return signed_request(params, "POST", "/rfqs/cancel", payload, metadata={"action": action, "order_type": "cancel"})

    raise SystemExit(f"unsupported Variational action {action!r}")


def create_rfq_payload(params: dict[str, Any]) -> dict[str, Any]:
    target_companies = list_param(params, "target_companies", "VARIATIONAL_TARGET_COMPANIES")
    if not target_companies:
        raise SystemExit("Variational create_rfq requires params.target_companies or VARIATIONAL_TARGET_COMPANIES")

    return {
        "structure": params.get("structure") or default_structure(params),
        "qty": str(params.get("qty", "0.001")),
        "expires_at": expires_at(params),
        "target_companies": target_companies,
    }


def default_structure(params: dict[str, Any]) -> dict[str, Any]:
    return {
        "legs": [
            {
                "instrument": {
                    "instrument_type": str(params.get("instrument_type", "perpetual_future")),
                    "underlying": str(params.get("underlying", "BTC")),
                    "settlement_asset": str(params.get("settlement_asset", "USDC")),
                },
                "ratio": int(params.get("ratio", 1)),
                "side": str(params.get("side", "buy")).lower(),
            }
        ]
    }


def signed_request(
    params: dict[str, Any],
    method: str,
    path: str,
    payload: dict[str, Any] | None,
    *,
    metadata: dict[str, Any],
) -> dict[str, Any]:
    base_url = str(params.get("base_url") or os.getenv("VARIATIONAL_BASE_URL") or DEFAULT_BASE_URL).rstrip("/")
    url = f"{base_url}{path}"
    body = None if payload is None else compact_json(payload)
    body_bytes = None if body is None else body.encode()

    key = required(params, "api_key", "VARIATIONAL_API_KEY")
    secret = required(params, "api_secret", "VARIATIONAL_API_SECRET")
    timestamp_ms = int(params.get("timestamp_ms") or time.time() * 1000)
    signature = sign(key, secret, timestamp_ms, method, url, body_bytes)

    headers = {
        "User-Agent": str(params.get("user_agent", "perps-latency-benchmark")),
        "X-Request-Timestamp-Ms": str(timestamp_ms),
        "X-Variational-Key": key,
        "X-Variational-Signature": signature,
    }
    if body is not None:
        headers["Content-Type"] = "application/json"

    built: dict[str, Any] = {
        "method": method,
        "url": url,
        "headers": headers,
        "metadata": {
            "builder": "variational-hmac",
            "base_url": base_url,
            **metadata,
        },
    }
    if body is not None:
        built["body"] = body
    return built


def sign(key: str, secret: str, timestamp_ms: int, method: str, url: str, body: bytes | None) -> str:
    parsed = urllib.parse.urlparse(url)
    path_url = parsed.path
    if parsed.query:
        path_url += "?" + parsed.query
    message = f"{key}|{timestamp_ms}|{method.upper()}|{path_url}".encode()
    secret_bytes = bytes.fromhex(secret.removeprefix("0x"))
    signer = hmac.new(secret_bytes, message, hashlib.sha256)
    if body is not None:
        signer.update(b"|")
        signer.update(body)
    return signer.hexdigest()


def expires_at(params: dict[str, Any]) -> str:
    if params.get("expires_at"):
        return str(params["expires_at"])
    seconds = int(params.get("expires_after_seconds", 30))
    return (datetime.now(timezone.utc) + timedelta(seconds=seconds)).isoformat().replace("+00:00", "Z")


def required(params: dict[str, Any], key: str, env_key: str) -> str:
    value = params.get(key) or os.getenv(env_key)
    if value in (None, ""):
        raise SystemExit(f"missing {key}; set params.{key} or {env_key}")
    return str(value)


def list_param(params: dict[str, Any], key: str, env_key: str) -> list[str]:
    value = params.get(key)
    if value in (None, ""):
        value = os.getenv(env_key)
    if value in (None, ""):
        return []
    if isinstance(value, list):
        return [str(item) for item in value if str(item)]
    return [item.strip() for item in str(value).split(",") if item.strip()]


def compact_json(value: Any) -> str:
    return json.dumps(value, separators=(",", ":"), sort_keys=False)


if __name__ == "__main__":
    raise SystemExit(main())
