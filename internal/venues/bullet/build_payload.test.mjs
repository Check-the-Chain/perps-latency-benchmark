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

test("websocket request id is a positive u64 even for negative warmup iterations", async () => {
  // Warmup iterations are negative (-warmups..-1). Deriving the correlation id
  // from `iteration` produced id:-1 on the first warmup, which Bullet rejected
  // as unparseable because the field is a u64.
  const { nextRequestID } = await import("./build_payload.mjs");
  const seen = [];
  for (let i = 0; i < 5; i++) seen.push(nextRequestID());
  for (const id of seen) {
    assert.equal(typeof id, "number", "must be JSON-serializable; JSON.stringify throws on BigInt");
    assert.ok(Number.isInteger(id) && id > 0, `request id ${id} must be a positive integer`);
    assert.ok(id < Number.MAX_SAFE_INTEGER, `request id ${id} must stay exactly representable`);
  }
  // The frame is JSON-serialized, so prove the id survives it.
  assert.equal(JSON.parse(JSON.stringify({ id: seen[0] })).id, seen[0]);
  assert.equal(new Set(seen.map(String)).size, seen.length, "ids must be distinct");
});
