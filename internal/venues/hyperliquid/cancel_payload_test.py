import unittest
from unittest import mock

import cancel_payload


class HyperliquidCancelPayloadTest(unittest.TestCase):
    def test_cancel_payload_includes_websocket_post_request(self):
        class Account:
            @staticmethod
            def from_key(_key):
                class Wallet:
                    address = "0xabc"

                return Wallet()

        def sign_l1_action(*_args):
            return {"r": "0x1", "s": "0x2", "v": 27}

        built = cancel_payload.cancel_payload(
            [{"asset": 0, "cloid": "0xabc"}],
            {"secret_key": "0x" + "1" * 64, "cleanup_nonce": 123},
            Account,
            "https://api.hyperliquid.xyz",
            sign_l1_action,
        )

        decoded = cancel_payload.json.loads(built["ws_body"])
        self.assertEqual(decoded["method"], "post")
        self.assertEqual(decoded["request"]["payload"]["action"]["type"], "cancelByCloid")

    def test_neutralize_price_uses_valid_wire_precision(self):
        self.assertEqual(
            cancel_payload.neutralize_price(
                {"price": "75000", "neutralize_slippage_bps": "100"},
                False,
            ),
            cancel_payload.Decimal("74257"),
        )

    def test_pause_for_cumulative_request_debt_sleeps_when_capacity_is_negative(self):
        with (
            mock.patch.object(cancel_payload, "post_json", return_value={"nRequestsUsed": 10, "nRequestsCap": 7, "nRequestsSurplus": 0}),
            mock.patch.object(cancel_payload.time, "sleep") as sleep,
        ):
            cancel_payload.pause_for_cumulative_request_debt({"debt_neutralize_pause_ms": 11000}, "0xabc", "https://api.hyperliquid.xyz")

        sleep.assert_called_once_with(11)

    def test_pause_for_cumulative_request_debt_skips_when_capacity_remains(self):
        with (
            mock.patch.object(cancel_payload, "post_json", return_value={"nRequestsUsed": 7, "nRequestsCap": 10, "nRequestsSurplus": 0}),
            mock.patch.object(cancel_payload.time, "sleep") as sleep,
        ):
            cancel_payload.pause_for_cumulative_request_debt({"debt_neutralize_pause_ms": 11000}, "0xabc", "https://api.hyperliquid.xyz")

        sleep.assert_not_called()


if __name__ == "__main__":
    unittest.main()
