import importlib.util
import os
import pathlib
import unittest
from unittest import mock

from eth_account import Account
from eth_account.messages import encode_typed_data


MODULE_PATH = pathlib.Path(__file__).with_name("build_payload.py")
spec = importlib.util.spec_from_file_location("build_payload", MODULE_PATH)
build_payload = importlib.util.module_from_spec(spec)
spec.loader.exec_module(build_payload)


PRIVATE_KEY = "0x59c6995e998f97a5a0044966f09453846546e5ef8b43c6ad99c9b8d8b5a2f2f"
ADDRESS = Account.from_key(PRIVATE_KEY).address
ENDPOINT_CONTRACT = "0x05ec92d78ed421f3d3ada77ffde167106565974e"


class NadoBuildPayloadTest(unittest.TestCase):
    def setUp(self):
        self.env = {
            "NADO_PRIVATE_KEY": PRIVATE_KEY,
            "NADO_ADDRESS": ADDRESS,
            "NADO_SUBACCOUNT": "default",
            "NADO_ENDPOINT_CONTRACT": ENDPOINT_CONTRACT,
        }

    def test_batch_builds_parallel_single_order_requests(self):
        req = {
            "scenario": "batch",
            "batch_size": 3,
            "iteration": 7,
            "params": {
                "product_id": 2,
                "symbol": "BTC-PERP",
                "side": "buy",
                "amount": "0.0014",
                "price": "77000",
                "order_type": "post_only",
                "confirmation": True,
            },
        }
        with mock.patch.dict(os.environ, self.env, clear=True):
            built = build_payload.build(req, Account, encode_typed_data)

        self.assertNotIn("body", built)
        self.assertEqual(len(built["parallel_requests"]), 3)
        self.assertEqual(len(built["metadata"]["cleanup_orders"]), 3)
        self.assertEqual(len(built["metadata"]["confirmation"]["digests"]), 3)
        self.assertFalse(built["metadata"]["native_batch_endpoint"])
        self.assertEqual(built["metadata"]["submission_model"], "parallel_http_single_orders")

        bodies = [request["body"] for request in built["parallel_requests"]]
        self.assertEqual(len(set(bodies)), 3)
        for body in bodies:
            self.assertIn('"place_order"', body)
            self.assertIn('"product_id":2', body)


if __name__ == "__main__":
    unittest.main()
