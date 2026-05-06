import hashlib
import hmac
import sys
import unittest
import urllib.parse
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import build_payload


class VariationalPayloadTest(unittest.TestCase):
    key = "api-key"
    secret = "01" * 32

    def test_status_builds_signed_get(self):
        built = build_payload.build(
            {
                "venue": "variational_omni",
                "transport": "http",
                "scenario": "single",
                "iteration": 0,
                "batch_size": 1,
                "params": {
                    "api_key": self.key,
                    "api_secret": self.secret,
                    "timestamp_ms": 1707254051670,
                    "base_url": "https://api.test/v1",
                },
            }
        )

        self.assertEqual(built["method"], "GET")
        self.assertEqual(built["url"], "https://api.test/v1/status")
        self.assertNotIn("body", built)
        self.assertEqual(built["headers"]["X-Variational-Key"], self.key)
        self.assertEqual(
            built["headers"]["X-Variational-Signature"],
            self.expected_signature("GET", "/v1/status", None),
        )
        self.assertEqual(built["metadata"]["order_type"], "status")

    def test_create_rfq_builds_signed_post(self):
        built = build_payload.build(
            {
                "venue": "variational_omni",
                "transport": "http",
                "scenario": "single",
                "iteration": 0,
                "batch_size": 1,
                "params": {
                    "action": "create_rfq",
                    "api_key": self.key,
                    "api_secret": self.secret,
                    "timestamp_ms": 1707254051670,
                    "base_url": "https://api.test/v1",
                    "target_companies": ["company-a"],
                    "expires_at": "2026-01-01T00:00:00Z",
                    "qty": "0.001",
                    "underlying": "BTC",
                },
            }
        )

        body = built["body"]
        decoded = build_payload.json.loads(body)
        self.assertEqual(decoded["target_companies"], ["company-a"])
        self.assertEqual(decoded["structure"]["legs"][0]["instrument"]["instrument_type"], "perpetual_future")
        self.assertEqual(decoded["structure"]["legs"][0]["instrument"]["underlying"], "BTC")
        self.assertEqual(built["metadata"]["order_type"], "rfq")
        self.assertEqual(built["metadata"]["speed_bump_ns"], 0)
        self.assertEqual(
            built["headers"]["X-Variational-Signature"],
            self.expected_signature("POST", "/v1/rfqs/new", body.encode()),
        )

    def test_accept_quote_builds_expected_payload(self):
        built = build_payload.build(
            {
                "venue": "variational_omni",
                "transport": "http",
                "scenario": "single",
                "iteration": 0,
                "batch_size": 1,
                "params": {
                    "action": "accept_quote",
                    "api_key": self.key,
                    "api_secret": self.secret,
                    "timestamp_ms": 1707254051670,
                    "base_url": "https://api.test/v1",
                    "rfq_id": "rfq-1",
                    "parent_quote_id": "quote-1",
                    "side": "sell",
                },
            }
        )

        self.assertEqual(built["url"], "https://api.test/v1/quotes/accept")
        self.assertEqual(
            build_payload.json.loads(built["body"]),
            {"rfq_id": "rfq-1", "parent_quote_id": "quote-1", "side": "sell"},
        )

    def test_create_rfq_requires_target_companies(self):
        with self.assertRaises(SystemExit):
            build_payload.build(
                {
                    "venue": "variational_omni",
                    "transport": "http",
                    "scenario": "single",
                    "iteration": 0,
                    "batch_size": 1,
                    "params": {
                        "action": "create_rfq",
                        "api_key": self.key,
                        "api_secret": self.secret,
                    },
                }
            )

    def expected_signature(self, method, path, body):
        signer = hmac.new(
            bytes.fromhex(self.secret),
            f"{self.key}|1707254051670|{method}|{path}".encode(),
            hashlib.sha256,
        )
        if body is not None:
            signer.update(b"|")
            signer.update(body)
        return signer.hexdigest()


if __name__ == "__main__":
    unittest.main()
