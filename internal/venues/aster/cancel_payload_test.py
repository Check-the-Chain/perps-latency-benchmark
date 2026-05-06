import sys
import unittest
from decimal import Decimal
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import cancel_payload


class FakeAsterClient:
    def __init__(self, positions=None, open_orders=None):
        self.positions = positions or []
        self.orders = open_orders or []

    def position_snapshot(self, _symbol):
        return self.positions

    def open_orders(self, _symbol):
        return self.orders


class AsterCleanupHelpersTest(unittest.TestCase):
    def test_planned_orders_derive_client_ids(self):
        params = {"run": {"run_id": "run-a", "iterations": 2, "warmups": 1, "batch_size": 2, "scenario": "batch"}}
        builder_params = {"symbol": "BTCUSDT", "side": "BUY"}

        first = cancel_payload.planned_orders(params, builder_params)
        second = cancel_payload.planned_orders(params, builder_params)

        self.assertEqual(first, second)
        self.assertEqual(len(first), 6)
        self.assertTrue(all(item["venue"] == "aster" for item in first))

    def test_cleanup_orders_filters_aster_refs(self):
        got = cancel_payload.cleanup_orders(
            {
                "cleanup_orders": [
                    {"venue": "aster", "client_order_id": "1"},
                    {"venue": "edgex", "client_order_id": "2"},
                ]
            }
        )

        self.assertEqual(got, [{"venue": "aster", "client_order_id": "1"}])

    def test_signed_position_size(self):
        got = cancel_payload.signed_position_size(
            [
                {"symbol": "BTCUSDT", "position_side": "BOTH", "position_amt": "0.003"},
                {"symbol": "BTCUSDT", "position_side": "SHORT", "position_amt": "0.001"},
                {"symbol": "ETHUSDT", "position_side": "BOTH", "position_amt": "1"},
            ],
            "BTCUSDT",
        )

        self.assertEqual(got, Decimal("0.002"))

    def test_before_run_rejects_existing_taker_position(self):
        got = cancel_payload.before_run(
            FakeAsterClient([
                {"symbol": "BTCUSDT", "position_side": "BOTH", "position_amt": "0.001"},
            ]),
            {"run": {"run_id": "run-a"}},
            {"symbol": "BTCUSDT", "neutralize_on_fill": True, "cleanup_all_open_orders": True},
        )

        self.assertFalse(got["cleanup"]["ok"])
        self.assertIn("existing position", got["cleanup"]["description"])

    def test_before_run_allows_flat_taker_account(self):
        got = cancel_payload.before_run(
            FakeAsterClient([]),
            {"run": {"run_id": "run-a"}},
            {"symbol": "BTCUSDT", "neutralize_on_fill": True, "cleanup_all_open_orders": True},
        )

        self.assertTrue(got["cleanup"]["ok"])


if __name__ == "__main__":
    unittest.main()
