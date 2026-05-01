import json
import unittest

import build_payload


class AccountStub:
    @staticmethod
    def from_key(_key):
        return type("Wallet", (), {"address": "0xabc"})()


class InfoStub:
    def __init__(self, *_args, **_kwargs):
        pass

    def all_mids(self):
        return {"BTC": "100000"}


class CloidStub:
    @staticmethod
    def from_int(value):
        return value


def order_request_to_order_wire(order, asset):
    wire = dict(order)
    wire["asset"] = asset
    return wire


def order_wires_to_order_action(order_wires, _builder, _grouping):
    return {"type": "order", "orders": order_wires}


def sign_l1_action(*_args):
    return {"r": "0x1", "s": "0x2", "v": 27}


class HyperliquidBuildPayloadTest(unittest.TestCase):
    def test_market_order_metadata_and_dynamic_limit_price(self):
        built = build_payload.build(
            {
                "scenario": "single",
                "iteration": 7,
                "transport": "websocket",
                "params": {
                    "secret_key": "0x1",
                    "symbol": "BTC",
                    "asset": 0,
                    "side": "buy",
                    "size": "0.0002",
                    "price": "75000",
                    "price_from_mid": True,
                    "price_offset_bps": "100",
                    "order_type": "market",
                    "tif": "Ioc",
                },
            },
            AccountStub,
            InfoStub,
            "https://api.hyperliquid.xyz",
            order_request_to_order_wire,
            order_wires_to_order_action,
            sign_l1_action,
            CloidStub,
        )

        body = json.loads(built["body"])
        order = body["action"]["orders"][0]
        self.assertEqual(built["metadata"]["order_type"], "market")
        self.assertEqual(built["metadata"]["time_in_force"], "Ioc")
        self.assertEqual(built["metadata"]["confirmation"]["user"], "0xabc")
        self.assertEqual(order["limit_px"], 101000.0)
        self.assertIn('"method":"post"', built["ws_body"])

    def test_post_only_order_metadata(self):
        self.assertEqual(
            build_payload.benchmark_order_type({"tif": "Alo"}),
            "post_only",
        )

    def test_dynamic_price_rounds_to_valid_hyperliquid_precision(self):
        self.assertEqual(
            build_payload.valid_price(build_payload.Decimal("98765.4321"), True),
            build_payload.Decimal("98766"),
        )
        self.assertEqual(
            build_payload.valid_price(build_payload.Decimal("98765.4321"), False),
            build_payload.Decimal("98765"),
        )
        self.assertEqual(
            build_payload.valid_price(build_payload.Decimal("1234.567"), True),
            build_payload.Decimal("1234.6"),
        )


if __name__ == "__main__":
    unittest.main()
