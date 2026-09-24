import unittest

import cost_payload


class AsterCostPayloadTest(unittest.TestCase):
    def test_balance_snapshot_uses_equity_for_reconciliation(self):
        client = cost_payload.AsterCostClient.__new__(cost_payload.AsterCostClient)
        client.signed_get = lambda _path, _values: [
            {
                "asset": "USDT",
                "balance": "-37.90",
                "crossUnPnl": "0.05",
                "availableBalance": "58.00",
                "crossWalletBalance": "-37.90",
                "updateTime": 1778030105782,
            }
        ]

        got = client.balance_snapshot()

        self.assertEqual(got["balance_usd"], -37.85)
        self.assertEqual(got["equity_usd"], -37.85)
        self.assertEqual(got["unrealized_usd"], 0.05)
        self.assertEqual(got["metadata"]["wallet_balance"], "-37.90")

    def test_balance_snapshot_sums_multi_asset_margin(self):
        client = cost_payload.AsterCostClient.__new__(cost_payload.AsterCostClient)
        client.signed_get = lambda _path, _values: [
            {
                "asset": "USDT",
                "balance": "-112.35",
                "crossUnPnl": "0",
                "availableBalance": "27.60",
                "crossWalletBalance": "-112.35",
                "marginAvailable": True,
                "updateTime": 1778030105782,
            },
            {
                "asset": "USDC",
                "balance": "139.87",
                "crossUnPnl": "0",
                "availableBalance": "27.60",
                "crossWalletBalance": "139.87",
                "marginAvailable": True,
                "updateTime": 1778030105783,
            },
        ]

        got = client.balance_snapshot()

        self.assertEqual(got["currency"], "USD")
        self.assertEqual(got["balance_usd"], 27.52)
        self.assertEqual(got["equity_usd"], 27.52)
        self.assertEqual(got["metadata"]["wallet_balance"], "27.52")


if __name__ == "__main__":
    unittest.main()
