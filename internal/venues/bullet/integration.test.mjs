// Network-backed checks against Bullet MAINNET. These only BUILD payloads — they
// never submit, so no order is placed and no funds move. Mainnet is used rather
// than testnet because mainnet is the network that gets benchmarked, and because
// testnet currently runs a chain schema newer than the pinned SDK (see README).
// Skipped unless credentials are present, so the offline suite stays green.
import assert from "node:assert/strict";
import test from "node:test";
import { build } from "./build_payload.mjs";
import { build as cancelBuild } from "./cancel_payload.mjs";

const HAVE_CREDS = Boolean(process.env.BULLET_DELEGATE_PRIVATE_KEY && process.env.BULLET_ACCOUNT_ADDRESS);
const PARAMS = {
  network: "mainnet", symbol: "BTC-USD", side: "bid", size: "0.0001",
  price: "60000", order_type: "post_only", run_id: "integration",
};

test("builds a signed order payload for both transports", { skip: !HAVE_CREDS && "no BULLET_* credentials" }, async () => {
  const built = await build({ scenario: "single", iteration: 0, batch_size: 1, params: PARAMS });
  const ws = JSON.parse(built.ws_body);
  const http = JSON.parse(built.body);
  assert.equal(ws.method, "order.place");
  assert.ok(ws.params.tx.length > 100, "signed tx should be substantial");
  assert.equal(http.body, ws.params.tx, "HTTP and WS must carry the identical signed tx");
  assert.equal(built.metadata.market_id, 1, "BTC-USD is market 1");
  assert.equal(built.metadata.cleanup_orders[0].client_order_id, built.metadata.client_order_ids[0]);
  assert.equal(built.metadata.confirmation.venue, "bullet");
  // The confirmation feed host must follow the configured network, not a default.
  assert.equal(built.metadata.confirmation.ws_url, "wss://tradingapi.bullet.xyz/ws");
});

test("batch places every order in one signed transaction", { skip: !HAVE_CREDS && "no BULLET_* credentials" }, async () => {
  const built = await build({ scenario: "batch", iteration: 1, batch_size: 5, params: { ...PARAMS, price_step: "10" } });
  assert.equal(built.metadata.orders, 5);
  assert.equal(new Set(built.metadata.client_order_ids).size, 5, "batch ids must be distinct");
  assert.equal(built.metadata.batch_signing_model, "single_signature_tx");
});

test("cancel payload targets the ids from a build", { skip: !HAVE_CREDS && "no BULLET_* credentials" }, async () => {
  const built = await build({ scenario: "single", iteration: 2, batch_size: 1, params: PARAMS });
  const cancel = await cancelBuild({ params: { phase: "after_sample", builder_params: PARAMS, metadata: built.metadata } });
  assert.equal(JSON.parse(cancel.ws_body).method, "order.cancel");
  assert.deepEqual(cancel.metadata.cancel_confirmation.client_order_ids, built.metadata.client_order_ids);
});

test("cleanup short-circuits with no refs and ignores other venues", { skip: !HAVE_CREDS && "no BULLET_* credentials" }, async () => {
  const empty = await cancelBuild({ params: { phase: "after_sample", builder_params: PARAMS, metadata: {} } });
  assert.equal(empty.cleanup.attempted, false);
  assert.equal(empty.cleanup.ok, true);
  const foreign = await cancelBuild({ params: { phase: "after_sample", builder_params: PARAMS,
    metadata: { cleanup_orders: [{ venue: "pacifica", symbol: "BTC", client_order_id: "1" }] } } });
  assert.equal(foreign.cleanup.attempted, false);
});
