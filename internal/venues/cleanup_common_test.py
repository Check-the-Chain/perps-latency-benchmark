import unittest

from cleanup_common import flat_position_guard, mark_self_heal_position


class CleanupCommonTest(unittest.TestCase):
    def test_flat_position_guard_blocks_guarded_non_flat_accounts(self):
        result = flat_position_guard(
            "lighter",
            [{"market": "1", "size": "0.001"}],
            {"guard_preexisting_position": True},
        )

        self.assertIsNotNone(result)
        cleanup = result["cleanup"]
        self.assertFalse(cleanup["ok"])
        self.assertEqual(cleanup["metadata"]["guard"], "flat_position")

    def test_flat_position_guard_ignores_unguarded_non_flat_accounts(self):
        result = flat_position_guard("lighter", [{"market": "1", "size": "0.001"}], {})

        self.assertIsNone(result)

    def test_flat_position_guard_allows_explicit_opt_out(self):
        result = flat_position_guard(
            "lighter",
            [{"market": "1", "size": "0.001"}],
            {"guard_preexisting_position": True, "allow_preexisting_position": True},
        )

        self.assertIsNone(result)

    def test_mark_self_heal_position_requests_restart(self):
        result = mark_self_heal_position(
            {"metadata": {"cleanup": "neutralize_position"}},
            "hyperliquid",
            [{"coin": "BTC", "szi": "0.001"}],
        )

        self.assertTrue(result["metadata"]["self_heal_position"])
        self.assertTrue(result["metadata"]["restart_before_measurement"])
        self.assertEqual(result["metadata"]["venue"], "hyperliquid")


if __name__ == "__main__":
    unittest.main()
