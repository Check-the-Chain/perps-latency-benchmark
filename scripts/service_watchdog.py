#!/usr/bin/env python3
"""Check and optionally restart perps benchmark services."""

from __future__ import annotations

import argparse
import datetime as dt
import sqlite3
import subprocess
import sys


DEFAULT_UNITS = [
    "perps-bench-api.service",
    "perps-bench-funding.service",
    "perps-bench-exchange-tps-hyperliquid.service",
    "perps-bench-exchange-tps-lighter.service",
    "perps-bench-hyperliquid-websocket.service",
    "perps-bench-hyperliquid-batch5.service",
    "perps-bench-lighter-websocket.service",
    "perps-bench-lighter-batch5.service",
]

DEFAULT_FRESHNESS = [
    ("hyperliquid", "single", "websocket", 180),
    ("hyperliquid", "batch", "websocket", 420),
    ("lighter", "single", "websocket", 180),
    ("lighter", "batch", "websocket", 420),
]


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--store", default="data/bench.db")
    parser.add_argument("--unit", action="append", default=[])
    parser.add_argument("--restart-failed", action="store_true")
    parser.add_argument("--skip-systemd", action="store_true")
    parser.add_argument("--skip-samples", action="store_true")
    args = parser.parse_args()

    problems: list[str] = []
    units = args.unit or DEFAULT_UNITS
    if not args.skip_systemd:
        problems.extend(check_units(units, args.restart_failed))
    if not args.skip_samples:
        problems.extend(check_samples(args.store, DEFAULT_FRESHNESS))

    for problem in problems:
        print(f"ERROR {problem}", file=sys.stderr)
    if problems:
        return 1
    print("OK perps services healthy")
    return 0


def check_units(units: list[str], restart_failed: bool) -> list[str]:
    problems: list[str] = []
    for unit in units:
        state = run(["systemctl", "is-active", unit], check=False).strip()
        if state == "active":
            continue
        problems.append(f"{unit} is {state or 'unknown'}")
        if restart_failed:
            run(["sudo", "systemctl", "reset-failed", unit], check=False)
            run(["sudo", "systemctl", "start", unit], check=False)
    return problems


def check_samples(path: str, expected: list[tuple[str, str, str, int]]) -> list[str]:
    problems: list[str] = []
    now = dt.datetime.now(dt.timezone.utc)
    con = sqlite3.connect(f"file:{path}?mode=ro", uri=True, timeout=2)
    try:
        for venue, scenario, transport, max_age_seconds in expected:
            row = con.execute(
                """
                SELECT completed_at
                FROM samples
                WHERE venue = ? AND scenario = ? AND transport = ? AND ok = 1
                ORDER BY id DESC
                LIMIT 1
                """,
                (venue, scenario, transport),
            ).fetchone()
            if row is None:
                problems.append(f"{venue}/{scenario}/{transport} has no accepted sample")
                continue
            completed_at = parse_time(row[0])
            age = (now - completed_at).total_seconds()
            if age > max_age_seconds:
                problems.append(
                    f"{venue}/{scenario}/{transport} latest accepted sample is {age:.0f}s old"
                )
    finally:
        con.close()
    return problems


def parse_time(value: str) -> dt.datetime:
    value = value.strip()
    if value.endswith("Z"):
        value = value[:-1] + "+00:00"
    return dt.datetime.fromisoformat(value).astimezone(dt.timezone.utc)


def run(command: list[str], check: bool = True) -> str:
    completed = subprocess.run(
        command,
        check=check,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    return completed.stdout


if __name__ == "__main__":
    raise SystemExit(main())
