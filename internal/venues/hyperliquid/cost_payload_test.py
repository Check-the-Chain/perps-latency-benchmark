import unittest

import cost_payload


class BalanceSnapshotTest(unittest.TestCase):
    def test_auto_uses_spot_usdc_when_clearinghouse_is_empty(self):
        got = cost_payload.balance_snapshot_from_states(
            {
                "marginSummary": {"accountValue": "0.0", "totalNtlPos": "0.0"},
                "withdrawable": "0.0",
                "assetPositions": [],
                "time": 1,
            },
            {"balances": [{"coin": "USDC", "total": "85.123", "hold": "0.0"}], "time": 2},
        )

        self.assertEqual(got["balance_usd"], 85.123)
        self.assertEqual(got["metadata"]["balance_source"], "spotClearinghouseState.USDC.total")
        self.assertEqual(got["metadata"]["clearinghouse_account_value"], "0.0")

    def test_auto_uses_clearinghouse_when_account_value_exists(self):
        got = cost_payload.balance_snapshot_from_states(
            {
                "marginSummary": {"accountValue": "42.5", "totalNtlPos": "0.0"},
                "withdrawable": "42.5",
                "assetPositions": [],
            },
            {"balances": [{"coin": "USDC", "total": "85.123", "hold": "0.0"}]},
        )

        self.assertEqual(got["balance_usd"], 42.5)
        self.assertEqual(got["metadata"]["balance_source"], "clearinghouseState.marginSummary.accountValue")


class FillAggregationTest(unittest.TestCase):
    def test_aggregates_by_cloid_without_oid_lookup(self):
        got = cost_payload.aggregate_hyperliquid_fills(
            [
                {"coin": "BTC", "px": "100", "sz": "0.001", "side": "B", "fee": "0.01", "time": 1, "oid": 123, "cloid": "0xabc"},
                {"coin": "BTC", "px": "101", "sz": "0.001", "side": "B", "fee": "0.01", "time": 1, "oid": 456, "cloid": "0xdef"},
            ],
            "BTC",
            set(),
            {"0xabc"},
            "entry",
        )

        self.assertEqual(got["count"], 1)
        self.assertEqual(str(got["buy_qty"]), "0.001")
        self.assertEqual(str(got["buy_value"]), "0.100")

    def test_wait_fills_polls_until_all_cloids_are_visible(self):
        client = cost_payload.HyperliquidCostClient.__new__(cost_payload.HyperliquidCostClient)
        client.user = "0xuser"
        client.poll_attempts = 3
        client.poll_interval = 0
        client.calls = 0

        def info(_body):
            client.calls += 1
            if client.calls == 1:
                return [
                    {"oid": 1, "cloid": "0xentry"},
                ]
            return [
                {"oid": 1, "cloid": "0xentry"},
                {"oid": 2, "cloid": "0xexit"},
            ]

        client.info = info

        got = client.wait_fills(100, 200, set(), {"0xentry", "0xexit"})

        self.assertEqual(client.calls, 2)
        self.assertEqual({fill["cloid"] for fill in got}, {"0xentry", "0xexit"})


if __name__ == "__main__":
    unittest.main()
