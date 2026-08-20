import assert from "node:assert/strict";
import test from "node:test";
import { BuilderError, clientOrderId, priceForOffset } from "./build_payload.mjs";

const CLIENT_ORDER_ID_LIMIT = 2n ** 52n;

test("generated client order ids stay below the float64 precision limit", () => {
  // The Go confirmation path round-trips these through float64. Anything at or
  // above 2**53 silently breaks matching, so this bound is load-bearing.
  for (let iteration = 0; iteration < 50; iteration++) {
    const id = clientOrderId({ run_id: "precision", symbol: "BTC-USD" }, { iteration }, 0);
    assert.ok(id > 0n, "id must be positive");
    assert.ok(id < 2n ** 53n, `id ${id} must stay below 2**53`);
  }
});

test("run_id makes ids deterministic and offset makes them distinct", () => {
  const req = { iteration: 3 };
  const params = { run_id: "fixed", symbol: "BTC-USD", side: "bid" };
  assert.equal(clientOrderId(params, req, 0), clientOrderId(params, req, 0));
  const batch = [0, 1, 2, 3, 4].map((offset) => clientOrderId(params, req, offset));
  assert.equal(new Set(batch.map(String)).size, batch.length, "batch ids must be distinct");
});

test("explicit client order id passes through unchanged when in range", () => {
  assert.equal(clientOrderId({ client_order_id: 42 }, { iteration: 0 }, 0), 42n);
});

test("explicit client order id at or above the limit is rejected", () => {
  assert.throws(
    () => clientOrderId({ client_order_id: CLIENT_ORDER_ID_LIMIT.toString() }, { iteration: 0 }, 0),
    BuilderError,
  );
  // Daniel's Date.now() << 16 scheme lands here; it silently corrupts matching.
  assert.throws(
    () => clientOrderId({ client_order_id: (BigInt(Date.now()) << 16n).toString() }, { iteration: 0 }, 0),
    BuilderError,
  );
});

test("explicit client order id of zero is rejected", () => {
  assert.throws(() => clientOrderId({ client_order_id: 0 }, { iteration: 0 }, 0), BuilderError);
});

test("batch price ladder steps away from the spread on both sides", () => {
  const bid = { price: "50000", price_step: "10", side: "bid" };
  assert.equal(priceForOffset(bid, 0), "50000");
  assert.equal(priceForOffset(bid, 2), "49980", "bids must ladder down, away from the ask");
  const ask = { price: "50000", price_step: "10", side: "ask" };
  assert.equal(priceForOffset(ask, 2), "50020", "asks must ladder up, away from the bid");
});

test("price_step_bps is applied relative to price", () => {
  assert.equal(priceForOffset({ price: "50000", price_step_bps: "10", side: "bid" }, 1), "49950");
});
