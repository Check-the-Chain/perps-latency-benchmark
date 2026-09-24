#!/usr/bin/env python3
import argparse
import datetime as dt
import json
import re
import sqlite3
import sys
import time
import urllib.error
import urllib.request


EXPLORER_URL = "https://rpc.hyperliquid.xyz/explorer"


def utc(ts):
    return dt.datetime.fromtimestamp(ts, dt.timezone.utc)


def floor_minute(ts):
    return int(ts) // 60 * 60


class ExplorerClient:
    def __init__(self, url, rps):
        self.url = url
        self.min_interval = 1.0 / rps
        self.last_request = 0.0
        self.backoff = 1.0
        self.requests = 0

    def block_meta(self, height):
        while True:
            now = time.monotonic()
            wait = self.min_interval - (now - self.last_request)
            if wait > 0:
                time.sleep(wait)
            self.last_request = time.monotonic()
            self.requests += 1

            body = json.dumps({"type": "blockDetails", "height": int(height)}).encode()
            req = urllib.request.Request(
                self.url,
                data=body,
                headers={"Content-Type": "application/json"},
            )
            try:
                with urllib.request.urlopen(req, timeout=20) as resp:
                    chunk = resp.read(8192).decode(errors="replace")
                self.backoff = 1.0
            except urllib.error.HTTPError as exc:
                if exc.code == 429:
                    sleep_for = min(self.backoff, 120.0)
                    print(f"{dt.datetime.now(dt.timezone.utc).isoformat()} 429 height={height}; sleeping {sleep_for:.1f}s", flush=True)
                    time.sleep(sleep_for)
                    self.backoff *= 1.7
                    continue
                raise
            except Exception as exc:
                sleep_for = min(self.backoff, 60.0)
                print(f"{dt.datetime.now(dt.timezone.utc).isoformat()} request error height={height}: {exc}; sleeping {sleep_for:.1f}s", flush=True)
                time.sleep(sleep_for)
                self.backoff *= 1.5
                continue

            if '"type":"error"' in chunk:
                msg = re.search(r'"message":"([^"]+)"', chunk)
                return None, None, msg.group(1) if msg else "error"

            block_time = re.search(r'"blockTime":(\d+)', chunk)
            num_txs = re.search(r'"numTxs":(\d+)', chunk)
            if not block_time or not num_txs:
                print(f"{dt.datetime.now(dt.timezone.utc).isoformat()} incomplete metadata height={height}; retrying", flush=True)
                time.sleep(min(self.backoff, 60.0))
                self.backoff *= 1.5
                continue
            return int(block_time.group(1)), int(num_txs.group(1)), None


def venue_id(db, venue):
    row = db.execute("SELECT id FROM venues WHERE code = ?", (venue,)).fetchone()
    if row:
        return row[0]
    cur = db.execute("INSERT INTO venues (code) VALUES (?)", (venue,))
    db.commit()
    return cur.lastrowid


def missing_minutes(db, venue, hours):
    now = floor_minute(dt.datetime.now(dt.timezone.utc).timestamp())
    start = now - hours * 3600
    max_existing = db.execute(
        """
        SELECT MAX(t)
        FROM tps_1m
        JOIN venues ON venues.id = tps_1m.v
        WHERE venues.code = ?
        """,
        (venue,),
    ).fetchone()[0]
    if max_existing is None:
        end = now - 120
    else:
        end = min(now - 120, int(max_existing) - 60)
    if end < start:
        return []

    seen = {
        row[0]
        for row in db.execute(
            """
            SELECT t
            FROM tps_1m
            JOIN venues ON venues.id = tps_1m.v
            WHERE venues.code = ? AND t BETWEEN ? AND ?
            """,
            (venue, start, end),
        )
    }
    return [t for t in range(start, end + 1, 60) if t not in seen]


def find_last_block_before(client, target_ms):
    low = 1
    high = 2_000_000_000
    best = None
    while low <= high:
        mid = (low + high) // 2
        block_time, _, err = client.block_meta(mid)
        if err is not None:
            high = mid - 1
            continue
        if block_time < target_ms:
            best = mid
            low = mid + 1
        else:
            high = mid - 1
    if best is None:
        raise RuntimeError("could not find a historical block before target")
    return best


def write_bucket(db, venue_id_value, minute, tx_count, block_count):
    now = int(time.time())
    db.execute(
        """
        INSERT INTO tps_1m (v, t, tx, blk, ord, plc, cxl, err, upd)
        VALUES (?, ?, ?, ?, 0, 0, 0, 0, ?)
        ON CONFLICT(v, t) DO UPDATE SET
          tx = excluded.tx,
          blk = excluded.blk,
          ord = excluded.ord,
          plc = excluded.plc,
          cxl = excluded.cxl,
          err = excluded.err,
          upd = excluded.upd
        """,
        (venue_id_value, minute, tx_count, block_count, now),
    )


def refresh_rollups(db, venue_id_value, touched_minutes):
    if not touched_minutes:
        return
    now = int(time.time())
    hours = sorted({minute // 3600 * 3600 for minute in touched_minutes})
    for hour in hours:
        db.execute("DELETE FROM tps_1h WHERE v = ? AND t = ?", (venue_id_value, hour))
        db.execute(
            """
            INSERT INTO tps_1h (v, t, tx, blk, ord, plc, cxl, err, upd)
            SELECT v, (t / 3600) * 3600, SUM(tx), SUM(blk), SUM(ord), SUM(plc), SUM(cxl), SUM(err), ?
            FROM tps_1m
            WHERE v = ? AND t BETWEEN ? AND ?
            GROUP BY v, (t / 3600) * 3600
            """,
            (now, venue_id_value, hour, hour + 3599),
        )


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--db", required=True)
    parser.add_argument("--hours", type=int, default=12)
    parser.add_argument("--venue", default="hyperliquid")
    parser.add_argument("--rps", type=float, default=2.0)
    parser.add_argument("--explorer-url", default=EXPLORER_URL)
    args = parser.parse_args()

    db = sqlite3.connect(args.db)
    db.execute("PRAGMA busy_timeout=10000")
    vid = venue_id(db, args.venue)

    missing = missing_minutes(db, args.venue, args.hours)
    if not missing:
        print("no missing minutes in requested window", flush=True)
        return

    missing_set = set(missing)
    first_missing = min(missing)
    latest_missing = max(missing)
    client = ExplorerClient(args.explorer_url, args.rps)
    approx_blocks = len(missing) * 880
    approx_seconds = approx_blocks / args.rps
    print(
        f"missing={len(missing)} range={utc(first_missing).isoformat()}..{utc(latest_missing).isoformat()} "
        f"approx_blocks={approx_blocks} approx_eta={dt.timedelta(seconds=int(approx_seconds))}",
        flush=True,
    )

    end_height = find_last_block_before(client, (latest_missing + 60) * 1000)
    print(f"starting_height={end_height} latest_missing={utc(latest_missing).isoformat()} requests={client.requests}", flush=True)

    current_minute = None
    minute_tx = 0
    minute_blocks = 0
    touched = []
    written = 0
    h = end_height
    started = time.monotonic()

    while h > 0:
        block_time, num_txs, err = client.block_meta(h)
        if err is not None:
            print(f"height={h} error={err}; stopping", flush=True)
            break
        minute = (block_time // 1000) // 60 * 60
        if minute < first_missing:
            break

        if current_minute is None:
            current_minute = minute
        if minute != current_minute:
            if current_minute in missing_set:
                write_bucket(db, vid, current_minute, minute_tx, minute_blocks)
                db.commit()
                touched.append(current_minute)
                written += 1
                elapsed = max(time.monotonic() - started, 1.0)
                print(
                    f"wrote {utc(current_minute).isoformat()} tx={minute_tx} blk={minute_blocks} "
                    f"written={written}/{len(missing)} height={h} rate={client.requests/elapsed:.2f}rps",
                    flush=True,
                )
                if len(touched) >= 60:
                    refresh_rollups(db, vid, touched)
                    db.commit()
                    touched.clear()
            current_minute = minute
            minute_tx = 0
            minute_blocks = 0

        if minute in missing_set:
            minute_tx += num_txs
            minute_blocks += 1
        h -= 1

    if current_minute in missing_set:
        write_bucket(db, vid, current_minute, minute_tx, minute_blocks)
        db.commit()
        touched.append(current_minute)
        written += 1
        print(f"wrote {utc(current_minute).isoformat()} tx={minute_tx} blk={minute_blocks} written={written}/{len(missing)}", flush=True)

    refresh_rollups(db, vid, touched)
    db.commit()
    remaining = missing_minutes(db, args.venue, args.hours)
    print(f"done written={written} remaining={len(remaining)} total_requests={client.requests}", flush=True)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        sys.exit(130)
