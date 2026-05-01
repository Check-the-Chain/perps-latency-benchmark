import asyncio
import json
import unittest
from urllib.parse import parse_qs

import build_payload


class SignerClientStub:
    ORDER_TYPE_LIMIT = 0
    ORDER_TYPE_MARKET = 1
    ORDER_TIME_IN_FORCE_IMMEDIATE_OR_CANCEL = 0
    ORDER_TIME_IN_FORCE_POST_ONLY = 2
    DEFAULT_IOC_EXPIRY = 0
    DEFAULT_28_DAY_ORDER_EXPIRY = 999

    last_order = None

    def __init__(self, **_kwargs):
        pass

    def get_api_key_nonce(self, api_key_index, _nonce):
        return api_key_index, 123

    def sign_create_order(self, **kwargs):
        self.__class__.last_order = kwargs
        return 14, json.dumps({"nonce": kwargs["nonce"], "order_type": kwargs["order_type"]}), "0x1", None

    async def close(self):
        pass


class LighterStub:
    SignerClient = SignerClientStub


class LighterBuildPayloadTest(unittest.TestCase):
    def test_market_order_metadata_and_ws_body(self):
        built = asyncio.run(
            build_payload.build(
                {
                    "scenario": "single",
                    "iteration": 3,
                    "params": {
                        "api_key_index": 4,
                        "account_index": 9,
                        "private_key": "secret",
                        "market_index": 1,
                        "base_amount": 100,
                        "price": 750000,
                        "order_type": SignerClientStub.ORDER_TYPE_MARKET,
                        "time_in_force": SignerClientStub.ORDER_TIME_IN_FORCE_IMMEDIATE_OR_CANCEL,
                    },
                },
                LighterStub,
            )
        )

        body = parse_qs(built["body"])
        ws_body = json.loads(built["ws_body"])

        self.assertEqual(built["metadata"]["order_type"], "market")
        self.assertEqual(body["tx_type"], ["14"])
        self.assertEqual(ws_body["type"], "jsonapi/sendtx")
        self.assertEqual(SignerClientStub.last_order["order_expiry"], SignerClientStub.DEFAULT_IOC_EXPIRY)


if __name__ == "__main__":
    unittest.main()
