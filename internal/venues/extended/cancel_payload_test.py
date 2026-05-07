import asyncio
import unittest
from unittest.mock import AsyncMock, patch

import cancel_payload


class ExtendedCancelPayloadTest(unittest.TestCase):
    def test_planned_orders_include_batch_offsets(self):
        got = cancel_payload.planned_orders(
            {"run": {"run_id": "run-a", "scenario": "batch", "iterations": 2, "warmups": 1, "batch_size": 3}},
            {"market": "BTC-USD", "side": "buy"},
        )

        self.assertEqual(len(got), 9)
        self.assertEqual(len({item["external_id"] for item in got}), 9)

    def test_cancel_request_uses_mass_cancel_for_multiple_external_ids(self):
        got = cancel_payload.cancel_request(
            {"api_key": "api", "base_url": "https://api.starknet.extended.exchange"},
            [{"external_id": "a"}, {"external_id": "b"}],
            {},
        )

        self.assertEqual(got["method"], "POST")
        self.assertTrue(got["url"].endswith("/api/v1/user/order/massCancel"))
        self.assertEqual(cancel_payload.json.loads(got["body"])["externalOrderIds"], ["a", "b"])

    def test_cancel_request_uses_external_id_delete_for_single_order(self):
        got = cancel_payload.cancel_request(
            {"api_key": "api", "base_url": "https://api.starknet.extended.exchange"},
            [{"external_id": "a"}],
            {},
        )

        self.assertEqual(got["method"], "DELETE")
        self.assertIn("/api/v1/user/order?externalId=a", got["url"])

    def test_neutralize_price_rounds_to_tick(self):
        self.assertEqual(
            cancel_payload.fallback_neutralize_price(
                {"price": "83000", "neutralize_slippage_bps": "100", "min_price_change": "0.1"},
                False,
            ),
            cancel_payload.Decimal("82178.2"),
        )

    def test_neutralize_price_from_orderbook_crosses_book(self):
        book = {
            "data": {
                "bid": [{"price": "80000"}],
                "ask": [{"price": "80010"}],
            }
        }
        params = {"neutralize_price_buffer_bps": "150", "min_price_change": "1"}

        self.assertEqual(
            cancel_payload.neutralize_price_from_orderbook(book, params, False),
            cancel_payload.Decimal("78817"),
        )
        self.assertEqual(
            cancel_payload.neutralize_price_from_orderbook(book, params, True),
            cancel_payload.Decimal("81211"),
        )

    def test_explicit_neutralize_price_wins(self):
        self.assertEqual(
            asyncio.run(cancel_payload.neutralize_price({"neutralize_price": "79000"}, False)),
            cancel_payload.Decimal("79000"),
        )

    def test_before_run_flattens_open_position_before_benchmark(self):
        with patch.object(
            cancel_payload,
            "position_snapshot",
            AsyncMock(side_effect=[
                [{"market": "BTC-USD", "side": "LONG", "size": "0.005"}],
                [],
            ]),
        ), patch.object(
            cancel_payload,
            "neutralize_position",
            AsyncMock(return_value={"body": "{}", "headers": {}, "metadata": {"cleanup_orders": [{"external_id": "close"}]}}),
        ), patch.object(
            cancel_payload,
            "submit_order_payload",
            return_value=(True, ""),
        ):
            result = asyncio.run(cancel_payload.before_run(object(), {}, {"neutralize_on_fill": True}))

        self.assertTrue(result["cleanup"]["ok"])
        self.assertEqual(result["cleanup"]["metadata"]["position_after"], [])

    def test_before_run_reports_maintenance_without_starting_benchmark(self):
        with patch.object(
            cancel_payload,
            "position_snapshot",
            AsyncMock(return_value=[{"market": "BTC-USD", "side": "LONG", "size": "0.005"}]),
        ), patch.object(
            cancel_payload,
            "neutralize_position",
            AsyncMock(return_value={"body": "{}", "headers": {}, "metadata": {}}),
        ), patch.object(
            cancel_payload,
            "submit_order_payload",
            return_value=(False, "Maintenance mode"),
        ):
            result = asyncio.run(cancel_payload.before_run(object(), {}, {"neutralize_on_fill": True}))

        self.assertFalse(result["cleanup"]["ok"])
        self.assertIn("Maintenance mode", result["cleanup"]["error"])

    def test_wait_for_position_delta_polls_past_stale_flat_snapshot(self):
        with patch.object(
            cancel_payload,
            "position_snapshot",
            AsyncMock(side_effect=[
                [],
                [{"market": "BTC-USD", "side": "LONG", "size": "0.001"}],
            ]),
        ):
            result = asyncio.run(cancel_payload.wait_for_position_delta(
                object(),
                {"market": "BTC-USD", "position_reconciliation_poll_attempts": 2, "position_reconciliation_poll_interval_ms": 0},
                [],
                "BTC-USD",
            ))

        self.assertEqual(result, [{"market": "BTC-USD", "side": "LONG", "size": "0.001"}])


if __name__ == "__main__":
    unittest.main()
