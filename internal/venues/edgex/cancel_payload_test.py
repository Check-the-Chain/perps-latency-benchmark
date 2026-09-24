import sys
import unittest
from decimal import Decimal
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import cancel_payload


class EdgeXCleanupHelpersTest(unittest.TestCase):
    def test_planned_orders_derive_client_ids_from_run_id(self):
        params = {"run": {"run_id": "run-a", "iterations": 2, "warmups": 1}}
        builder_params = {"contract_id": "10000001", "side": "BUY"}

        first = cancel_payload.planned_orders(params, builder_params)
        second = cancel_payload.planned_orders(params, builder_params)

        self.assertEqual(first, second)
        self.assertEqual(len(first), 3)
        self.assertTrue(all(item["client_order_id"].isdigit() for item in first))

    def test_cleanup_orders_filters_edge_x_refs(self):
        got = cancel_payload.cleanup_orders(
            {
                "cleanup_orders": [
                    {"venue": "edgex", "client_order_id": "1"},
                    {"venue": "extended", "client_order_id": "2"},
                ]
            }
        )

        self.assertEqual(got, [{"venue": "edgex", "client_order_id": "1"}])

    def test_signed_position_size(self):
        got = cancel_payload.signed_position_size(
            [
                {"contract_id": "10000001", "side": "LONG", "size": "0.003"},
                {"contract_id": "10000001", "side": "SHORT", "size": "0.001"},
                {"contract_id": "10000002", "side": "LONG", "size": "1"},
            ],
            "10000001",
        )

        self.assertEqual(got, Decimal("0.002"))


if __name__ == "__main__":
    unittest.main()
