import json
import os
import unittest

import build_payload


class PacificaBuildPayloadTest(unittest.TestCase):
    def setUp(self):
        # Public example key used only for deterministic local signing tests.
        os.environ["PACIFICA_PRIVATE_KEY"] = "2Z2Wn4kN5ZNhZzuFTQSyTiN4ixX8U6ew5wPDJbHngZaC3zF3uWNj4dQ63cnGfXpw1cESZPCqvoZE7VURyuj9kf8b"

    def tearDown(self):
        os.environ.pop("PACIFICA_PRIVATE_KEY", None)
        os.environ.pop("PACIFICA_ACCOUNT", None)
        os.environ.pop("PACIFICA_AGENT_WALLET", None)

    def test_builds_websocket_limit_order_with_confirmation_metadata(self):
        built = build_payload.build(
            {
                "scenario": "single",
                "transport": "websocket",
                "iteration": 2,
                "params": {
                    "run_id": "test-run",
                    "symbol": "BTC",
                    "side": "buy",
                    "amount": "0.001",
                    "price": "75000",
                    "tif": "ALO",
                    "timestamp": 1749223025396,
                },
            }
        )

        ws_body = json.loads(built["ws_body"])
        order = ws_body["params"]["create_order"]
        self.assertEqual(order["symbol"], "BTC")
        self.assertEqual(order["side"], "bid")
        self.assertEqual(order["tif"], "ALO")
        self.assertIn("signature", order)
        self.assertEqual(built["metadata"]["order_type"], "post_only")
        self.assertEqual(built["metadata"]["speed_bump_ms"], 0)
        self.assertEqual(built["metadata"]["confirmation"]["venue"], "pacifica")
        self.assertEqual(built["metadata"]["cleanup_orders"][0]["client_order_id"], order["client_order_id"])

    def test_builds_batch_order_actions(self):
        built = build_payload.build(
            {
                "scenario": "batch",
                "transport": "websocket",
                "iteration": 0,
                "batch_size": 2,
                "params": {
                    "run_id": "test-run",
                    "symbol": "BTC",
                    "side": "ask",
                    "amount": "0.001",
                    "price": "75000",
                    "price_step": "10",
                    "tif": "ALO",
                    "timestamp": 1749223025396,
                },
            }
        )

        body = json.loads(built["ws_batch_body"])
        actions = body["params"]["batch_orders"]["actions"]
        self.assertEqual(len(actions), 2)
        self.assertEqual(actions[0]["type"], "Create")
        self.assertEqual(actions[1]["data"]["price"], "75010")
        self.assertEqual(built["metadata"]["orders"], 2)

    def test_agent_wallet_is_set_when_signing_key_differs_from_account(self):
        os.environ["PACIFICA_ACCOUNT"] = "DifferentAccount111111111111111111111111111111"
        built = build_payload.build(
            {
                "scenario": "single",
                "transport": "websocket",
                "iteration": 0,
                "params": {
                    "symbol": "BTC",
                    "side": "bid",
                    "amount": "0.001",
                    "price": "75000",
                    "timestamp": 1749223025396,
                },
            }
        )

        order = json.loads(built["ws_body"])["params"]["create_order"]
        self.assertEqual(order["account"], os.environ["PACIFICA_ACCOUNT"])
        self.assertTrue(order["agent_wallet"])


if __name__ == "__main__":
    unittest.main()
