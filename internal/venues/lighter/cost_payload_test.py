import asyncio
import unittest

import cost_payload


class FakeClient:
    balance_poll_attempts = 1
    balance_poll_interval = 0

    def __init__(self, after):
        self.after = after

    async def balance_snapshot(self):
        return self.after

    def venue_for_sample(self, sample):
        return sample.get("venue") or "lighter"


class LighterCostPayloadTest(unittest.TestCase):
    def test_marks_balance_audit_dirty_when_account_has_existing_position(self):
        before = {
            "venue": "lighter",
            "balance_usd": 100.0,
            "positions": [{"market": "1", "size": "0.012", "unrealized_usd": 0.25}],
        }
        after = {
            "venue": "lighter",
            "balance_usd": 99.96,
            "positions": [{"market": "1", "size": "0.012", "unrealized_usd": 0.20}],
        }
        entry = {
            "order_id": "entry",
            "buy_qty": "0.001",
            "sell_qty": "0",
            "buy_value": "80",
            "sell_value": "0",
            "fee": "0.02",
            "count": 1,
        }
        exit = {
            "order_id": "exit",
            "buy_qty": "0",
            "sell_qty": "0.001",
            "buy_value": "0",
            "sell_value": "79.98",
            "fee": "0.02",
            "count": 1,
        }

        got = asyncio.run(cost_payload.build_with_reconciled_balance(
            FakeClient(after),
            {"venue": "lighter", "completed_at": "2026-05-05T10:00:00Z"},
            before,
            entry,
            exit,
            {"market_index": 1},
        ))

        self.assertFalse(got["clean"])
        self.assertEqual(got["quality_reason"], "account has an existing Lighter position; balance audit cannot isolate benchmark cost")
        self.assertTrue(got["metadata"]["balance_audit_blocked_by_position"])

    def test_keeps_flat_account_clean_when_balance_matches_fills(self):
        before = {"venue": "lighter", "balance_usd": 100.0, "positions": []}
        after = {"venue": "lighter", "balance_usd": 99.96, "positions": []}
        entry = {
            "order_id": "entry",
            "buy_qty": "0.001",
            "sell_qty": "0",
            "buy_value": "80",
            "sell_value": "0",
            "fee": "0.02",
            "count": 1,
        }
        exit = {
            "order_id": "exit",
            "buy_qty": "0",
            "sell_qty": "0.001",
            "buy_value": "0",
            "sell_value": "80",
            "fee": "0.02",
            "count": 1,
        }

        got = asyncio.run(cost_payload.build_with_reconciled_balance(
            FakeClient(after),
            {"venue": "lighter", "completed_at": "2026-05-05T10:00:00Z"},
            before,
            entry,
            exit,
            {"market_index": 1},
        ))

        self.assertTrue(got["clean"])
        self.assertAlmostEqual(got["trade_cost_usd"], 0.04)

    def test_marks_partial_private_trade_history_dirty(self):
        before = {"venue": "lighter", "balance_usd": 100.0, "positions": []}
        after = {"venue": "lighter", "balance_usd": 99.96, "positions": []}
        entry = {
            "order_id": "entry",
            "buy_qty": "0.001",
            "sell_qty": "0",
            "buy_value": "80",
            "sell_value": "0",
            "fee": "0.02",
            "count": 1,
        }
        exit = {
            "order_id": "exit",
            "buy_qty": "0",
            "sell_qty": "0.001",
            "buy_value": "0",
            "sell_value": "80",
            "fee": "0.02",
            "count": 1,
        }

        got = asyncio.run(cost_payload.build_with_reconciled_balance(
            FakeClient(after),
            {
                "venue": "lighter",
                "completed_at": "2026-05-05T10:00:00Z",
                "expected_entry_fill": {"size": 0.002},
                "expected_exit_fill": {"size": 0.002},
            },
            before,
            entry,
            exit,
            {"market_index": 1},
        ))

        self.assertFalse(got["clean"])
        self.assertEqual(got["quality_reason"], "incomplete entry or exit fill")


if __name__ == "__main__":
    unittest.main()
