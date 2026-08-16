import base64
import json
import sys
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))
import cancel_payload
from build_payload import EIP712_DOMAIN_TYPES, risex_action_hash
from eth_account import Account
from eth_account.messages import encode_typed_data


ACCOUNT_KEY = "0x59c6995e998f97a5a0044966f09453846546e5ef8b43c6ad99c9b8d8b5a2f2f"
SIGNER_KEY = "0x8b3a350cf5c34c9194ca85829a2df0ec3153be0318b5e2d3348e872092edffd"
ACCOUNT_ADDRESS = Account.from_key(ACCOUNT_KEY).address
SIGNER_ADDRESS = Account.from_key(SIGNER_KEY).address
ROUTER = "0xaadde0cea454f2bcb26f46ed54c5709b7bb34a7e"
DOMAIN = {"name": "RISEx", "version": "1", "chainId": 4153, "verifyingContract": "0x1111111111111111111111111111111111111111"}

OPEN_ORDER = {
    "order_id": "0x00000000000120b700000000012688e2000000000000015f",
    "resting_order_id": "36955",
    "market_id": 1,
    "client_order_id": "2060885383252642467",
}


def fixtures(open_orders):
    return {
        "/v1/auth/eip712-domain": {"data": {"name": "RISEx", "version": "1", "chain_id": "4153", "verifying_contract": "0x1111111111111111111111111111111111111111"}},
        "/v1/system/config": {"data": {"addresses": {"router": ROUTER}}},
        "/v1/nonce-state/" + ACCOUNT_ADDRESS: {"data": {"nonce_anchor": "9", "current_bitmap_index": 0}},
        "orders/open": {"data": {"orders": open_orders}},
    }


def fake_fetch_json(fixture_map):
    def _fetch(url):
        for path, payload in fixture_map.items():
            if path in url:
                return payload
        raise AssertionError(f"unexpected fetch_json call: {url}")

    return _fetch


class RisexCancelPayloadTest(unittest.TestCase):
    def setUp(self):
        cancel_payload._ROUTER_CONTEXT = {}
        self.env = {"RISEX_PRIVATE_KEY": ACCOUNT_KEY, "RISEX_SIGNER_PRIVATE_KEY": SIGNER_KEY}

    def build(self, params, open_orders=None):
        req = {"params": params}
        with mock.patch.object(cancel_payload, "fetch_json", side_effect=fake_fetch_json(fixtures(open_orders or []))):
            with mock.patch.dict("os.environ", self.env, clear=True):
                return cancel_payload.build(req, Account, encode_typed_data)

    def recovered_signer(self, permit, resting_order_id):
        action_hash = risex_action_hash(b"RISE_PERPS_CANCEL_ORDER_V1", 1, int(resting_order_id))
        typed_data = {
            "types": {
                "EIP712Domain": EIP712_DOMAIN_TYPES,
                "VerifyWitness": [
                    {"name": "account", "type": "address"},
                    {"name": "target", "type": "address"},
                    {"name": "hash", "type": "bytes32"},
                    {"name": "nonceAnchor", "type": "uint48"},
                    {"name": "nonceBitmap", "type": "uint8"},
                    {"name": "deadline", "type": "uint32"},
                ],
            },
            "primaryType": "VerifyWitness",
            "domain": DOMAIN,
            "message": {
                "account": permit["account"],
                "target": ROUTER,
                "hash": action_hash,
                "nonceAnchor": int(permit["nonce_anchor"]),
                "nonceBitmap": permit["nonce_bitmap_index"],
                "deadline": permit["deadline"],
            },
        }
        sig = base64.b64decode(permit["signature"])
        r = int.from_bytes(sig[0:32], "big")
        s_bytes = bytearray(sig[32:64])
        v = 28 if (s_bytes[0] & 0x80) else 27
        s_bytes[0] &= 0x7F
        s = int.from_bytes(bytes(s_bytes), "big")
        return Account.recover_message(encode_typed_data(full_message=typed_data), vrs=(v, r, s))

    def test_after_sample_no_refs_is_a_noop(self):
        built = self.build({"phase": "after_sample", "order_refs": []})
        self.assertEqual(built, {"cleanup": {"attempted": False, "ok": True, "description": "no RISEx cleanup_orders"}})

    def test_after_sample_cancels_resolved_order(self):
        built = self.build(
            {
                "phase": "after_sample",
                "order_refs": [{"venue": "risex", "client_order_id": OPEN_ORDER["client_order_id"], "market_id": 1}],
                "builder_params": {},
            },
            open_orders=[OPEN_ORDER],
        )
        body = json.loads(built["body"])
        self.assertEqual(body["order_id"], OPEN_ORDER["order_id"])
        self.assertEqual(built["metadata"]["orders_remaining"], 0)
        self.assertEqual(self.recovered_signer(body["permit"], OPEN_ORDER["resting_order_id"]), SIGNER_ADDRESS)

    def test_after_sample_reports_remaining_when_multiple_match(self):
        second = dict(OPEN_ORDER, order_id="0xdeadbeef", resting_order_id="36956", client_order_id="1")
        built = self.build(
            {
                "phase": "after_sample",
                "order_refs": [
                    {"venue": "risex", "client_order_id": OPEN_ORDER["client_order_id"], "market_id": 1},
                    {"venue": "risex", "client_order_id": "1", "market_id": 1},
                ],
                "builder_params": {},
            },
            open_orders=[OPEN_ORDER, second],
        )
        self.assertEqual(built["metadata"]["orders_remaining"], 1)

    def test_after_run_sweeps_open_orders_left_on_account(self):
        # This is the exact scenario a crashed/killed benchmark run leaves
        # behind: no order_refs (nothing tracked this run), but a real order
        # is still resting on the account from earlier. before_run/after_run
        # must find and cancel it rather than no-op just because there are
        # no refs -- see build()'s phase dispatch.
        built = self.build({"phase": "after_run", "builder_params": {"market_id": 1}}, open_orders=[OPEN_ORDER])
        body = json.loads(built["body"])
        self.assertEqual(body["order_id"], OPEN_ORDER["order_id"])
        self.assertEqual(built["metadata"]["phase"], "after_run")
        self.assertEqual(self.recovered_signer(body["permit"], OPEN_ORDER["resting_order_id"]), SIGNER_ADDRESS)

    def test_after_run_reports_no_sweep_needed_when_clean(self):
        built = self.build({"phase": "after_run", "builder_params": {"market_id": 1}}, open_orders=[])
        self.assertEqual(built, {"cleanup": {"attempted": False, "ok": True, "description": "no RISEx orders open on market_id 1 at after_run"}})

    def test_before_run_uses_market_id_from_builder_params(self):
        built = self.build({"phase": "before_run", "builder_params": {"market_id": 7}}, open_orders=[])
        self.assertIn("market_id 7", built["cleanup"]["description"])

    def test_router_context_is_cached_across_calls(self):
        calls = []

        def counting_fetch(url):
            calls.append(url)
            return fake_fetch_json(fixtures([OPEN_ORDER]))(url)

        with mock.patch.object(cancel_payload, "fetch_json", side_effect=counting_fetch):
            with mock.patch.dict("os.environ", self.env, clear=True):
                cancel_payload.build({"params": {"phase": "after_run", "builder_params": {"market_id": 1}}}, Account, encode_typed_data)
                cancel_payload.build({"params": {"phase": "after_run", "builder_params": {"market_id": 1}}}, Account, encode_typed_data)

        domain_calls = [url for url in calls if "eip712-domain" in url]
        self.assertEqual(len(domain_calls), 1, "router_context should fetch domain/router once and cache it, not once per cleanup call")


if __name__ == "__main__":
    unittest.main()
