import sys
import unittest
import urllib.parse
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import build_payload


class AsterPayloadHelpersTest(unittest.TestCase):
    private_key = "0x" + ("1" * 64)
    signer = "0x19E7E376E7C213B7E7e7e46cc70A5dD086DAff2A"
    user = "0x63DD5aCC6b1aa0f563956C0e534DD30B6dcF7C4e"

    def test_sign_values_appends_eip712_signature(self):
        body = build_payload.sign_values(
            {
                "symbol": "BTCUSDT",
                "side": "BUY",
                "type": "LIMIT",
                "quantity": "1",
                "price": "9000",
                "timeInForce": "GTC",
                "nonce": "1748310859508867",
                "user": self.user,
                "signer": self.signer,
            },
            self.private_key,
        )

        payload, signature = body.rsplit("&signature=", 1)
        self.assertTrue(signature.startswith("0x"))
        self.assertEqual(len(signature), 132)
        self.assertEqual(payload, urllib.parse.urlencode(urllib.parse.parse_qsl(payload)))

    def test_private_key_derives_signer(self):
        self.assertEqual(build_payload.signer_address(self.private_key), self.signer)

    def test_client_order_id_is_stable_for_run_id(self):
        params = {"run_id": "run-a", "symbol": "BTCUSDT", "side": "BUY"}
        req = {"iteration": 7}

        first = build_payload.client_order_id(params, req, 0)
        second = build_payload.client_order_id(params, req, 0)

        self.assertEqual(first, second)
        self.assertLessEqual(len(first), 36)
        self.assertTrue(first.startswith("pb_"))

    def test_build_single_with_listen_key_avoids_network(self):
        built = build_payload.build(
            {
                "venue": "aster",
                "transport": "http",
                "scenario": "single",
                "iteration": 0,
                "batch_size": 1,
                "params": {
                    "user": self.user,
                    "signer": self.signer,
                    "private_key": self.private_key,
                    "symbol": "BTCUSDT",
                    "side": "BUY",
                    "type": "LIMIT",
                    "time_in_force": "GTX",
                    "quantity": "0.001",
                    "price": "75000",
                    "run_id": "run-a",
                    "listen_key": "listen",
                },
            }
        )

        parsed = urllib.parse.parse_qs(built["body"])
        self.assertEqual(parsed["timeInForce"], ["GTX"])
        self.assertEqual(parsed["user"], [self.user])
        self.assertEqual(parsed["signer"], [self.signer])
        self.assertIn("nonce", parsed)
        self.assertEqual(built["metadata"]["order_type"], "post_only")
        self.assertEqual(built["metadata"]["confirmation"]["ws_url"], "wss://fstream.asterdex.com/ws/listen")

    def test_batch_body_wraps_orders(self):
        built = build_payload.build(
            {
                "venue": "aster",
                "transport": "http",
                "scenario": "batch",
                "iteration": 0,
                "batch_size": 2,
                "params": {
                    "user": self.user,
                    "signer": self.signer,
                    "private_key": self.private_key,
                    "symbol": "BTCUSDT",
                    "side": "BUY",
                    "type": "LIMIT",
                    "time_in_force": "GTX",
                    "quantity": "0.001",
                    "price": "75000",
                    "run_id": "run-a",
                    "listen_key": "listen",
                },
            }
        )

        parsed = urllib.parse.parse_qs(built["body"])
        self.assertIn("batchOrders", parsed)
        orders = build_payload.json.loads(parsed["batchOrders"][0])
        self.assertEqual(len(orders), 2)
        self.assertEqual([order["price"] for order in orders], ["75000", "75000"])

    def test_batch_uses_price_ladder(self):
        built = build_payload.build(
            {
                "venue": "aster",
                "transport": "http",
                "scenario": "batch",
                "iteration": 0,
                "batch_size": 3,
                "params": {
                    "user": self.user,
                    "signer": self.signer,
                    "private_key": self.private_key,
                    "symbol": "BTCUSDT",
                    "side": "BUY",
                    "type": "LIMIT",
                    "time_in_force": "GTX",
                    "quantity": "0.001",
                    "price": "75000",
                    "price_step": "10",
                    "run_id": "run-a",
                    "listen_key": "listen",
                },
            }
        )

        parsed = urllib.parse.parse_qs(built["body"])
        orders = build_payload.json.loads(parsed["batchOrders"][0])
        self.assertEqual([order["price"] for order in orders], ["75000", "74990", "74980"])


if __name__ == "__main__":
    unittest.main()
