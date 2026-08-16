import base64
import json
import sys
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))
import build_payload
from eth_account import Account
from eth_account.messages import encode_typed_data


ACCOUNT_KEY = "0x59c6995e998f97a5a0044966f09453846546e5ef8b43c6ad99c9b8d8b5a2f2f"
SIGNER_KEY = "0x8b3a350cf5c34c9194ca85829a2df0ec3153be0318b5e2d3348e872092edffd"
ACCOUNT_ADDRESS = Account.from_key(ACCOUNT_KEY).address
SIGNER_ADDRESS = Account.from_key(SIGNER_KEY).address
ROUTER = "0xaadde0cea454f2bcb26f46ed54c5709b7bb34a7e"

FIXTURES = {
    "/v1/auth/eip712-domain": {"data": {"name": "RISEx", "version": "1", "chain_id": "4153", "verifying_contract": "0x1111111111111111111111111111111111111111"}},
    "/v1/system/config": {"data": {"addresses": {"router": ROUTER}}},
    "/v1/markets": {"data": {"markets": [{"market_id": "1", "config": {"step_price": "0.1", "step_size": "0.000001"}}]}},
    "/v1/nonce-state/" + ACCOUNT_ADDRESS: {"data": {"nonce_anchor": "9", "current_bitmap_index": 0}},
    "/v1/auth/nonce": {"data": {"nonce": "1" * 64}},
}


def fake_fetch_json(url):
    for path, payload in FIXTURES.items():
        if url.endswith(path):
            return payload
    raise AssertionError(f"unexpected fetch_json call: {url}")


class RisexBuildPayloadTest(unittest.TestCase):
    def setUp(self):
        build_payload._CONTEXT = {}
        self.env = {
            "RISEX_PRIVATE_KEY": ACCOUNT_KEY,
            "RISEX_SIGNER_PRIVATE_KEY": SIGNER_KEY,
        }
        self.fetch_patch = mock.patch.object(build_payload, "fetch_json", side_effect=fake_fetch_json)
        self.fetch_patch.start()
        self.addCleanup(self.fetch_patch.stop)

    def build(self, params):
        req = {"scenario": "single", "iteration": 0, "params": params}
        with mock.patch.dict("os.environ", self.env, clear=True):
            return build_payload.build(req, Account, encode_typed_data)

    def recovered_signer(self, built):
        """Recompute the action hash and EIP-712 digest independently of
        build_payload's own internals, and recover the signer from the
        produced signature. A wrong header_flags/action-hash computation
        (the actual bug this session found live) still produces a
        structurally valid, plausible-looking payload -- it just recovers
        to the wrong signer. Structural assertions alone would have missed
        that; this catches it.
        """
        body = json.loads(built["body"])
        permit = body["permit"]

        header_flags = 0x01
        if body["builder_id"]:
            header_flags |= 0x02
        if int(body["client_order_id"]):
            header_flags |= 0x04
        if body["ttl_units"]:
            header_flags |= 0x10
        order_data = build_payload.pack_order_data(body["market_id"], body["size_steps"], body["price_ticks"], body["side"], body["post_only"], body["order_type"], body["time_in_force"])
        hash_words = [header_flags, order_data, body["builder_id"]]
        if body["builder_fee_bps"] > 0:
            hash_words.append(body["builder_fee_bps"])
        hash_words.extend([int(body["client_order_id"]), body["ttl_units"]])
        action_hash = build_payload.risex_action_hash(b"RISE_PERPS_PLACE_ORDER_V1", *hash_words)

        typed_data = {
            "types": {
                "EIP712Domain": build_payload.EIP712_DOMAIN_TYPES,
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
            "domain": {"name": "RISEx", "version": "1", "chainId": 4153, "verifyingContract": "0x1111111111111111111111111111111111111111"},
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

    def test_permit_signature_recovers_to_registered_signer(self):
        built = self.build({
            "market_id": 1,
            "side": "buy",
            "amount": "0.001",
            "price": "60000",
            "post_only": True,
        })
        self.assertEqual(self.recovered_signer(built), SIGNER_ADDRESS)

    def test_sell_side_signature_also_recovers(self):
        built = self.build({
            "market_id": 1,
            "side": "sell",
            "amount": "0.001",
            "price": "60000",
            "post_only": True,
        })
        self.assertEqual(self.recovered_signer(built), SIGNER_ADDRESS)
        self.assertEqual(json.loads(built["body"])["side"], 1)

    def test_market_order_type_rejected(self):
        with self.assertRaises(SystemExit):
            self.build({"market_id": 1, "side": "buy", "amount": "0.001", "price": "60000", "order_type": "market"})

    def test_single_scenario_returns_one_order_no_parallel_requests(self):
        built = self.build({"market_id": 1, "side": "buy", "amount": "0.001", "price": "60000"})
        self.assertNotIn("parallel_requests", built)
        self.assertIn("body", built)

    def test_batch_scenario_is_refused_not_faked(self):
        # RISEx has no native batch endpoint, and concurrent single-order
        # submission is confirmed unreliable here (unlike Nado/Extended,
        # whose time-based nonces tolerate it): refuse outright, matching
        # edgeX's precedent, rather than silently produce misleading
        # latency numbers for orders that mostly fail server-side. See
        # README.md's batch section.
        req = {"scenario": "batch", "batch_size": 3, "iteration": 1, "params": {"market_id": 1, "side": "buy", "amount": "0.001", "price": "60000"}}
        with mock.patch.dict("os.environ", self.env, clear=True):
            with self.assertRaises(SystemExit):
                build_payload.build(req, Account, encode_typed_data)


class PackOrderDataTest(unittest.TestCase):
    def test_rejects_unsupported_order_type(self):
        with self.assertRaises(SystemExit):
            build_payload.pack_order_data(1, 100, 600000, 0, True, 0, 0)

    def test_rejects_unsupported_time_in_force(self):
        with self.assertRaises(SystemExit):
            build_payload.pack_order_data(1, 100, 600000, 0, True, 1, 1)

    def test_buy_post_only_flags(self):
        order_data = build_payload.pack_order_data(1, 200, 600000, 0, True, 1, 0)
        flags = (order_data >> 6) & 0xFF
        self.assertEqual(flags, 0b00100010)

    def test_sell_flag_bit_set(self):
        order_data = build_payload.pack_order_data(1, 200, 600000, 1, True, 1, 0)
        flags = (order_data >> 6) & 0xFF
        self.assertTrue(flags & 0b1)


class HelperTest(unittest.TestCase):
    def test_parse_nonce_handles_bare_hex(self):
        self.assertEqual(build_payload.parse_nonce("ff"), 255)
        self.assertEqual(build_payload.parse_nonce("0xff"), 255)
        self.assertEqual(build_payload.parse_nonce(255), 255)

    def test_hex0x_adds_prefix_once(self):
        self.assertEqual(build_payload.hex0x("ab"), "0xab")
        self.assertEqual(build_payload.hex0x("0xab"), "0xab")

    def test_side_code(self):
        self.assertEqual(build_payload.side_code({"side": "buy"}), 0)
        self.assertEqual(build_payload.side_code({"side": "sell"}), 1)
        self.assertEqual(build_payload.side_code({}), 0)

    def test_resolve_account_address_prefers_private_key(self):
        params = {"private_key": ACCOUNT_KEY, "account_address": "0x" + "1" * 40}
        self.assertEqual(build_payload.resolve_account_address(params, Account), ACCOUNT_ADDRESS)

    def test_resolve_account_address_falls_back_to_address(self):
        address = "0x00000000000000000000000000000000000000ab"
        with mock.patch.dict("os.environ", {}, clear=True):
            got = build_payload.resolve_account_address({"account_address": address}, Account)
        self.assertEqual(got.lower(), address.lower())

    def test_resolve_account_address_requires_one(self):
        with mock.patch.dict("os.environ", {}, clear=True):
            with self.assertRaises(SystemExit):
                build_payload.resolve_account_address({}, Account)


if __name__ == "__main__":
    unittest.main()
