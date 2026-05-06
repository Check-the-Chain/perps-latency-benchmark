import unittest
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import build_payload


class AccountStub:
    def __init__(self, **kwargs):
        self.api_key = kwargs["api_key"]


class ConfigStub:
    class signing:
        starknet_domain = "domain"


class MarketModelStub:
    @staticmethod
    def model_validate(value):
        return value


class EnumStub:
    BUY = "BUY"
    LIMIT = "LIMIT"
    GTT = "GTT"
    ACCOUNT = "ACCOUNT"


class OrderObjectStub:
    def __init__(self, **kwargs):
        self.kwargs = kwargs

    def to_api_request_json(self, **_kwargs):
        return {
            "externalId": self.kwargs["order_external_id"],
            "price": str(self.kwargs["price"]),
            "nonce": self.kwargs["nonce"],
        }


def create_order_object_stub(**kwargs):
    return OrderObjectStub(**kwargs)


class ExtendedPayloadHelpersTest(unittest.TestCase):
    def test_order_external_id_is_stable_for_run_id(self):
        params = {"run_id": "run-a", "market": "BTC-USD", "side": "buy"}
        req = {"iteration": 3}

        first = build_payload.order_external_id(params, req)
        second = build_payload.order_external_id(params, req)

        self.assertEqual(first, second)
        self.assertTrue(first.startswith("pb-"))

    def test_explicit_external_id_wins(self):
        got = build_payload.order_external_id({"external_id": "custom"}, {"iteration": 1})
        self.assertEqual(got, "custom")

    def test_external_id_offset_is_unique(self):
        params = {"run_id": "run-a", "market": "BTC-USD", "side": "buy"}
        req = {"iteration": 3}

        first = build_payload.order_external_id(params, req, 0)
        second = build_payload.order_external_id(params, req, 1)

        self.assertNotEqual(first, second)

    def test_price_ladder_moves_post_only_buys_away(self):
        params = {"price": "75000", "price_step": "10", "side": "buy"}
        self.assertEqual(build_payload.price_for_offset(params, 0), build_payload.Decimal("75000"))
        self.assertEqual(build_payload.price_for_offset(params, 2), build_payload.Decimal("74980"))

    def test_benchmark_order_type(self):
        self.assertEqual(build_payload.benchmark_order_type("limit", "GTT", True), "post_only")
        self.assertEqual(build_payload.benchmark_order_type("market", "IOC", False), "market")
        self.assertEqual(build_payload.benchmark_order_type("limit", "IOC", False), "ioc")

    def test_batch_builds_parallel_single_order_requests(self):
        built = build_payload.build(
            {
                "scenario": "batch",
                "iteration": 2,
                "batch_size": 3,
                "params": {
                    "vault": "1",
                    "private_key": "private",
                    "public_key": "public",
                    "api_key": "api",
                    "market": "BTC-USD",
                    "market_model": {"name": "BTC-USD"},
                    "size": "0.001",
                    "price": "75000",
                    "price_step": "10",
                    "nonce_base": 100,
                    "run_id": "run-a",
                },
            },
            ConfigStub,
            ConfigStub,
            object,
            AccountStub,
            MarketModelStub,
            EnumStub,
            EnumStub,
            EnumStub,
            EnumStub,
            create_order_object_stub,
        )

        requests = built["parallel_requests"]
        self.assertEqual(len(requests), 3)
        bodies = [build_payload.json.loads(request["body"]) for request in requests]
        self.assertEqual([body["price"] for body in bodies], ["75000", "74990", "74980"])
        self.assertEqual([body["nonce"] for body in bodies], [102, 103, 104])
        self.assertEqual(built["metadata"]["native_batch_endpoint"], False)
        self.assertEqual(len(built["metadata"]["confirmation"]["external_ids"]), 3)


if __name__ == "__main__":
    unittest.main()
