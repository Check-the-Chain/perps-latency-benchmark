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


if __name__ == "__main__":
    unittest.main()
