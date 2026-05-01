import asyncio
import json
import tempfile
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

    last_init = None

    def __init__(self, **kwargs):
        self.__class__.last_init = kwargs
        self.nonce = 123
        pass

    def get_api_key_nonce(self, api_key_index, _nonce):
        nonce = self.nonce
        self.nonce += 1
        return api_key_index, nonce

    def sign_create_order(self, **kwargs):
        self.__class__.last_order = kwargs
        return 14, json.dumps({"nonce": kwargs["nonce"], "order_type": kwargs["order_type"]}), "0x1", None

    def create_auth_token_with_expiry(self, **_kwargs):
        return "auth-token", None

    async def close(self):
        pass


class LighterStub:
    SignerClient = SignerClientStub


class LighterBuildPayloadTest(unittest.TestCase):
    def setUp(self):
        SignerClientStub.last_init = None
        SignerClientStub.last_order = None

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
        self.assertEqual(built["metadata"]["confirmation"]["order_indices"], [built["metadata"]["cleanup_orders"][0]["order_index"]])
        self.assertEqual(body["tx_type"], ["14"])
        self.assertEqual(ws_body["type"], "jsonapi/sendtx")
        self.assertEqual(SignerClientStub.last_order["order_expiry"], SignerClientStub.DEFAULT_IOC_EXPIRY)

    def test_post_only_order_uses_maker_key_by_default(self):
        old_env = {}
        for key, value in {
            "LIGHTER_ACCOUNT_INDEX": "9",
            "LIGHTER_MAKER_API_KEY_INDEX": "5",
            "LIGHTER_MAKER_PRIVATE_KEY": "maker-secret",
            "LIGHTER_API_KEY_INDEX": "4",
            "LIGHTER_PRIVATE_KEY": "default-secret",
        }.items():
            old_env[key] = build_payload.os.environ.get(key)
            build_payload.os.environ[key] = value
        try:
            built = asyncio.run(
                build_payload.build(
                    {
                        "scenario": "single",
                        "params": {
                            "market_index": 1,
                            "base_amount": 100,
                            "price": 750000,
                        },
                    },
                    LighterStub,
                )
            )
        finally:
            for key, value in old_env.items():
                if value is None:
                    build_payload.os.environ.pop(key, None)
                else:
                    build_payload.os.environ[key] = value

        self.assertEqual(built["metadata"]["order_type"], "post_only")
        self.assertEqual(built["metadata"]["api_key_role"], "maker")
        self.assertEqual(SignerClientStub.last_init["api_private_keys"], {5: "maker-secret"})
        self.assertEqual(SignerClientStub.last_order["api_key_index"], 5)

    def test_market_order_uses_taker_key_by_default(self):
        old_env = {}
        for key, value in {
            "LIGHTER_ACCOUNT_INDEX": "9",
            "LIGHTER_TAKER_API_KEY_INDEX": "6",
            "LIGHTER_TAKER_PRIVATE_KEY": "taker-secret",
            "LIGHTER_API_KEY_INDEX": "4",
            "LIGHTER_PRIVATE_KEY": "default-secret",
        }.items():
            old_env[key] = build_payload.os.environ.get(key)
            build_payload.os.environ[key] = value
        try:
            built = asyncio.run(
                build_payload.build(
                    {
                        "scenario": "single",
                        "params": {
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
        finally:
            for key, value in old_env.items():
                if value is None:
                    build_payload.os.environ.pop(key, None)
                else:
                    build_payload.os.environ[key] = value

        self.assertEqual(built["metadata"]["order_type"], "market")
        self.assertEqual(built["metadata"]["api_key_role"], "taker")
        self.assertEqual(SignerClientStub.last_init["api_private_keys"], {6: "taker-secret"})
        self.assertEqual(SignerClientStub.last_order["api_key_index"], 6)

    def test_file_nonce_allocator_is_monotonic(self):
        with tempfile.NamedTemporaryFile() as file:
            client = SignerClientStub()

            self.assertEqual(build_payload.next_nonce(client, 4, file.name), 123)
            self.assertEqual(build_payload.next_nonce(client, 4, file.name), 124)


if __name__ == "__main__":
    unittest.main()
