#!/usr/bin/env python3
"""One-time RISEx session-signer registration.

This is a manual setup utility, not wired into the benchmark's Definition or
CleanupCommand — run it once per (account, signer) pair before any benchmark
run against that account. It performs a REAL mainnet action (POST
/v1/auth/register-signer) unless --dry-run is passed.

Usage:
    RISEX_PRIVATE_KEY=0x... RISEX_SIGNER_PRIVATE_KEY=0x... \
        uv run --with eth-account --with eth-utils python \
        internal/venues/risex/register_signer.py --dry-run

    # after reviewing the printed request, actually submit it:
    RISEX_PRIVATE_KEY=0x... RISEX_SIGNER_PRIVATE_KEY=0x... \
        uv run --with eth-account --with eth-utils python \
        internal/venues/risex/register_signer.py --confirm-live

Both keys should come from a local .env.risex.local file or your shell
environment, never pasted into a chat/log. RISEX_PRIVATE_KEY is the main
account (must already hold deposited collateral); RISEX_SIGNER_PRIVATE_KEY is
a *separate* freshly generated key that will become the session signer used
by build_payload.py/cancel_payload.py.

Nonce note: nonce_anchor must be exactly (current + 1) -- confirmed against
live RISEx mainnet, which reverts with InvalidNonceAnchor otherwise. This
script fetches that anchor fresh via GET /v1/nonce-state/{account}, same as
build_payload.py's first call. Run this script to completion before starting
any benchmark run against the same account, so build_payload.py's own fresh
fetch sees this registration's already-consumed anchor and picks the next
one correctly.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request
from typing import Any

from build_payload import (
    DEFAULT_BASE_URL,
    EIP712_DOMAIN_TYPES,
    env_or_param,
    fetch_json,
    hex0x,
    parse_eip712_domain,
    starting_nonce_anchor,
)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--base-url", default=None, help="override RISEX_BASE_URL / default mainnet host")
    parser.add_argument("--expiry-days", type=int, default=30, help="session key expiration, in days from now (default 30)")
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--dry-run", action="store_true", help="build and print the request without submitting it")
    mode.add_argument("--confirm-live", action="store_true", help="actually submit the registration to RISEx mainnet")
    args = parser.parse_args()

    try:
        from eth_account import Account
        from eth_account.messages import encode_typed_data
    except ImportError as exc:
        raise SystemExit("missing dependency; run with `uv run --with eth-account --with eth-utils python ...`") from exc

    base_url = (args.base_url or os.getenv("RISEX_BASE_URL", DEFAULT_BASE_URL)).rstrip("/")
    params: dict[str, Any] = {}
    account = Account.from_key(env_or_param(params, "private_key", "RISEX_PRIVATE_KEY"))
    signer = Account.from_key(env_or_param(params, "signer_private_key", "RISEX_SIGNER_PRIVATE_KEY"))

    if account.address.lower() == signer.address.lower():
        raise SystemExit("RISEX_PRIVATE_KEY and RISEX_SIGNER_PRIVATE_KEY must be different keys")

    domain = fetch_json(base_url + "/v1/auth/eip712-domain")["data"]
    eip712_domain = parse_eip712_domain(domain)
    anchor = starting_nonce_anchor(base_url, account.address, params)
    expiration = int(time.time()) + args.expiry_days * 86400
    message = "RISEx session key"

    register_typed = {
        "types": {
            "EIP712Domain": EIP712_DOMAIN_TYPES,
            "RegisterSigner": [
                {"name": "account", "type": "address"},
                {"name": "signer", "type": "address"},
                {"name": "message", "type": "string"},
                {"name": "expiration", "type": "uint32"},
                {"name": "nonceAnchor", "type": "uint48"},
                {"name": "nonceBitmap", "type": "uint8"},
            ],
        },
        "primaryType": "RegisterSigner",
        "domain": eip712_domain,
        "message": {
            "account": account.address,
            "signer": signer.address,
            "message": message,
            "expiration": expiration,
            "nonceAnchor": anchor,
            "nonceBitmap": 0,
        },
    }
    verify_typed = {
        "types": {
            "EIP712Domain": EIP712_DOMAIN_TYPES,
            "VerifySigner": [
                {"name": "account", "type": "address"},
                {"name": "nonceAnchor", "type": "uint48"},
                {"name": "nonceBitmap", "type": "uint8"},
            ],
        },
        "primaryType": "VerifySigner",
        "domain": eip712_domain,
        "message": {"account": account.address, "nonceAnchor": anchor, "nonceBitmap": 0},
    }

    account_signature = account.sign_message(encode_typed_data(full_message=register_typed)).signature.hex()
    signer_signature = signer.sign_message(encode_typed_data(full_message=verify_typed)).signature.hex()

    body = {
        "account": account.address,
        "signer": signer.address,
        "message": message,
        "nonce_anchor": str(anchor),
        "nonce_bitmap_index": 0,
        "expiration": str(expiration),
        "account_signature": hex0x(account_signature),
        "signer_signature": hex0x(signer_signature),
    }

    print(f"account (main):    {account.address}")
    print(f"signer (session):  {signer.address}")
    print(f"base_url:          {base_url}")
    print(f"nonce_anchor/bit:  {anchor}/0")
    print(f"expires:           {expiration} ({args.expiry_days} days from now)")
    print()
    print("Request body:")
    print(json.dumps(body, indent=2))

    if args.dry_run:
        print()
        print("--dry-run: not submitted. Re-run with --confirm-live to actually register this signer on mainnet.")
        return 0

    print()
    print(f"Submitting to {base_url}/v1/auth/register-signer ...")
    response = post_json(base_url + "/v1/auth/register-signer", body)
    print(json.dumps(response, indent=2))
    return 0


def post_json(url: str, body: dict[str, Any]) -> Any:
    data = json.dumps(body).encode()
    request = urllib.request.Request(url, data=data, headers={"Content-Type": "application/json", "User-Agent": "perps-latency-benchmark"}, method="POST")
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            return json.loads(response.read().decode())
    except urllib.error.HTTPError as exc:
        body_text = exc.read().decode(errors="replace")
        try:
            return json.loads(body_text)
        except json.JSONDecodeError:
            return {"http_status": exc.code, "body": body_text}


if __name__ == "__main__":
    sys.exit(main())
