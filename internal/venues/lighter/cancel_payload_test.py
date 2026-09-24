import unittest
import asyncio
import os
from decimal import Decimal

import cancel_payload


class SignerClientStub:
    last_init = None

    def __init__(self, **kwargs):
        self.__class__.last_init = kwargs
        self.api_client = object()

    async def close(self):
        pass


class LighterStub:
    SignerClient = SignerClientStub

    class AccountApi:
        def __init__(self, _api_client):
            pass


class LighterCancelPayloadTest(unittest.TestCase):
    def setUp(self):
        SignerClientStub.last_init = None

    def test_neutralize_base_amount_scales_position_size(self):
        self.assertEqual(
            cancel_payload.neutralize_base_amount(Decimal("0.00100"), {}),
            100,
        )

    def test_neutralize_base_amount_rejects_zero_after_scaling(self):
        with self.assertRaises(SystemExit):
            cancel_payload.neutralize_base_amount(Decimal("0.000001"), {"base_amount_decimals": 5})

    def test_neutralize_price_prefers_side_specific_price(self):
        self.assertEqual(
            cancel_payload.neutralize_price(
                {"price": 1500000, "neutralize_buy_price": 1500000, "neutralize_sell_price": 500000},
                False,
            ),
            500000,
        )

    def test_cancel_orders_include_websocket_batch_payload(self):
        class Client:
            def create_auth_token_with_expiry(self, **_kwargs):
                return "auth", None

            def sign_cancel_order(self, **_kwargs):
                return 14, '{"order":"cancel"}', "", None

        built = asyncio.run(cancel_payload.cancel_orders(
            Client(),
            [
                {"market_index": 1, "order_index": 11},
                {"market_index": 1, "order_index": 12},
            ],
            {"cleanup_nonce": 100, "account_index": 7, "market_index": 1},
            2,
            {},
        ))

        decoded = cancel_payload.json.loads(built["ws_body"])
        self.assertEqual(decoded["type"], "jsonapi/sendtxbatch")
        self.assertEqual(built["metadata"]["orders"], 2)
        self.assertEqual(built["metadata"]["cancel_confirmation"]["order_indices"], [11, 12])

    def test_cleanup_builder_uses_prefixed_account_environment(self):
        old_env = {key: os.environ.get(key) for key in (
            "LIGHTER_ACCOUNT_INDEX",
            "LIGHTER_API_KEY_INDEX",
            "LIGHTER_PRIVATE_KEY",
            "LIGHTER_FREE_ACCOUNT_INDEX",
            "LIGHTER_FREE_API_KEY_INDEX",
            "LIGHTER_FREE_PRIVATE_KEY",
        )}
        os.environ.update({
            "LIGHTER_ACCOUNT_INDEX": "1",
            "LIGHTER_API_KEY_INDEX": "2",
            "LIGHTER_PRIVATE_KEY": "main-secret",
            "LIGHTER_FREE_ACCOUNT_INDEX": "724248",
            "LIGHTER_FREE_API_KEY_INDEX": "4",
            "LIGHTER_FREE_PRIVATE_KEY": "free-secret",
        })
        try:
            built = asyncio.run(cancel_payload.build({
                "params": {
                    "phase": "after_sample",
                    "metadata": {},
                    "builder_params": {"env_prefix": "LIGHTER_FREE"},
                },
            }, LighterStub))
        finally:
            for key, value in old_env.items():
                if value is None:
                    os.environ.pop(key, None)
                else:
                    os.environ[key] = value

        self.assertEqual(built["cleanup"]["ok"], True)
        self.assertEqual(SignerClientStub.last_init["account_index"], 724248)
        self.assertEqual(SignerClientStub.last_init["api_private_keys"], {4: "free-secret"})

    def test_cleanup_builder_uses_maker_key_for_post_only_role(self):
        old_env = {key: os.environ.get(key) for key in (
            "LIGHTER_ACCOUNT_INDEX",
            "LIGHTER_API_KEY_INDEX",
            "LIGHTER_PRIVATE_KEY",
            "LIGHTER_MAKER_API_KEY_INDEX",
            "LIGHTER_MAKER_PRIVATE_KEY",
        )}
        os.environ.update({
            "LIGHTER_ACCOUNT_INDEX": "723853",
            "LIGHTER_API_KEY_INDEX": "4",
            "LIGHTER_PRIVATE_KEY": "default-secret",
            "LIGHTER_MAKER_API_KEY_INDEX": "5",
            "LIGHTER_MAKER_PRIVATE_KEY": "maker-secret",
        })
        try:
            built = asyncio.run(cancel_payload.build({
                "params": {
                    "phase": "after_sample",
                    "metadata": {},
                    "builder_params": {"api_key_role": "maker"},
                },
            }, LighterStub))
        finally:
            for key, value in old_env.items():
                if value is None:
                    os.environ.pop(key, None)
                else:
                    os.environ[key] = value

        self.assertEqual(built["cleanup"]["ok"], True)
        self.assertEqual(SignerClientStub.last_init["account_index"], 723853)
        self.assertEqual(SignerClientStub.last_init["api_private_keys"], {5: "maker-secret"})

    def test_cleanup_builder_uses_taker_key_for_ioc_role(self):
        old_env = {key: os.environ.get(key) for key in (
            "LIGHTER_ACCOUNT_INDEX",
            "LIGHTER_API_KEY_INDEX",
            "LIGHTER_PRIVATE_KEY",
            "LIGHTER_TAKER_API_KEY_INDEX",
            "LIGHTER_TAKER_PRIVATE_KEY",
        )}
        os.environ.update({
            "LIGHTER_ACCOUNT_INDEX": "723853",
            "LIGHTER_API_KEY_INDEX": "4",
            "LIGHTER_PRIVATE_KEY": "default-secret",
            "LIGHTER_TAKER_API_KEY_INDEX": "6",
            "LIGHTER_TAKER_PRIVATE_KEY": "taker-secret",
        })
        try:
            built = asyncio.run(cancel_payload.build({
                "params": {
                    "phase": "after_sample",
                    "metadata": {},
                    "builder_params": {
                        "api_key_role": "taker",
                        "order_type": 1,
                        "time_in_force": 0,
                    },
                },
            }, LighterStub))
        finally:
            for key, value in old_env.items():
                if value is None:
                    os.environ.pop(key, None)
                else:
                    os.environ[key] = value

        self.assertEqual(built["cleanup"]["ok"], True)
        self.assertEqual(SignerClientStub.last_init["account_index"], 723853)
        self.assertEqual(SignerClientStub.last_init["api_private_keys"], {6: "taker-secret"})


if __name__ == "__main__":
    unittest.main()
