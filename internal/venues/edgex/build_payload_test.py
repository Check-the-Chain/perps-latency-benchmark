import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import build_payload


class EdgeXPayloadHelpersTest(unittest.TestCase):
    def test_client_order_id_is_stable_for_run_id(self):
        params = {"run_id": "run-a", "contract_id": "10000001", "side": "BUY"}
        req = {"iteration": 7}

        first = build_payload.client_order_id(params, req, 0)
        second = build_payload.client_order_id(params, req, 0)

        self.assertEqual(first, second)
        self.assertTrue(first.isdigit())

    def test_explicit_client_order_id_wins(self):
        got = build_payload.client_order_id({"client_order_id": "123"}, {"iteration": 1}, 0)
        self.assertEqual(got, "123")

    def test_private_ws_url(self):
        got = build_payload.private_ws_url("https://pro.edgex.exchange", 42)
        self.assertEqual(got, "wss://pro.edgex.exchange/api/v1/private/ws?accountId=42")

    def test_benchmark_order_type(self):
        self.assertEqual(build_payload.benchmark_order_type("LIMIT", "POST_ONLY", False), "post_only")
        self.assertEqual(build_payload.benchmark_order_type("MARKET", "IMMEDIATE_OR_CANCEL", False), "market")
        self.assertEqual(build_payload.benchmark_order_type("LIMIT", "IMMEDIATE_OR_CANCEL", False), "immediate_or_cancel")


if __name__ == "__main__":
    unittest.main()
