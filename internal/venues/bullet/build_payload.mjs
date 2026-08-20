#!/usr/bin/env node
// Build signed Bullet order payloads for perps-bench.
//
// Signing is delegated to the official Bullet WASM SDK (@bulletxyz/sdk-wasm),
// so the wire encoding is the exchange's own code rather than a transcription.
// This script never connects to place an order; it only builds payloads, and it
// runs outside the measured latency window.

import { createHash } from "node:crypto";
import * as readline from "node:readline";
import { pathToFileURL } from "node:url";
import {
  Client, Keypair, NewOrderArgs, OrderType, RuntimeCall, Side, Transaction, User,
} from "@bulletxyz/sdk-wasm";

// Keep below 2**53. Client order ids are u64 on the wire, but the Go
// confirmation path (internal/venues/confirmutil.Text) round-trips JSON numbers
// through float64, so anything at or above 2**53 loses precision and silently
// breaks confirmation matching. The SDK does not guard this: NewOrderArgs takes
// an unbounded bigint.
const CLIENT_ORDER_ID_LIMIT = 2n ** 52n;

const SIDES = { bid: Side.Bid, buy: Side.Bid, ask: Side.Ask, sell: Side.Ask };
const ORDER_TYPES = {
  limit: OrderType.Limit,
  post_only: OrderType.PostOnly,
  fill_or_kill: OrderType.FillOrKill,
  immediate_or_cancel: OrderType.ImmediateOrCancel,
  ioc: OrderType.ImmediateOrCancel,
  post_only_slide: OrderType.PostOnlySlide,
  post_only_front: OrderType.PostOnlyFront,
};

export class BuilderError extends Error {}

function fail(message) {
  throw new BuilderError(message);
}

export function envOrParam(params, key, envKey) {
  const value = params[key];
  if (value !== undefined && value !== null && String(value) !== "") return String(value);
  const fromEnv = process.env[envKey];
  if (fromEnv) return fromEnv;
  fail(`missing ${envKey}`);
}

function networkOf(params) {
  return String(params.network ?? "mainnet").toLowerCase();
}

// One Client per (network, key). Building it fetches exchange metadata and takes
// seconds, so it happens once at startup and is reused for every later payload.
//
// Hosts come from `network` alone; `base_url` is deliberately ignored. Cleanup
// (internal/app/cleanup.go) injects a `base_url` param whenever the venue sets
// DefaultHTTPPath, which Bullet now does for HTTP submission, so the param will
// be present and unused. `network` is the only lever that moves the signing
// chain parameters, so it must stay the single source of truth.
const clients = new Map();
export async function clientFor(params) {
  const network = networkOf(params);
  const seed = envOrParam(params, "delegate_private_key", "BULLET_DELEGATE_PRIVATE_KEY");
  const key = `${network}|${seed.slice(0, 8)}`;
  if (!clients.has(key)) {
    // wasm-bindgen moves ownership: a Keypair passed here is consumed, so build
    // a fresh one and let the client act as the default signer.
    clients.set(key, await Client.builder().network(network).keypair(Keypair.fromHex(seed)).build());
  }
  return clients.get(key);
}

export function clientOrderId(params, req, offset) {
  const explicit = params.client_order_id;
  if (explicit !== undefined && explicit !== null && offset === 0) {
    const value = BigInt(explicit);
    if (value < 1n || value >= CLIENT_ORDER_ID_LIMIT) {
      fail(`client_order_id ${value} out of range [1, ${CLIENT_ORDER_ID_LIMIT}); ` +
           `ids at or above 2**53 lose precision in confirmation matching`);
    }
    return value;
  }
  const runID = params.run_id ?? "";
  let seed = `${runID}:${req.iteration ?? 0}:${params.symbol ?? "BTC-USD"}:${params.side ?? "bid"}:${offset}`;
  if (!runID) seed += `:${process.hrtime.bigint()}`;
  const digest = createHash("blake2b512").update(seed).digest().subarray(0, 8);
  return (BigInt(`0x${digest.toString("hex")}`) % (CLIENT_ORDER_ID_LIMIT - 1n)) + 1n;
}

export function priceForOffset(params, offset) {
  const base = String(params.price ?? 50000);
  if (offset <= 0) return base;
  const step = params.price_step_bps !== undefined
    ? (Number(base) * Number(params.price_step_bps)) / 10000
    : Number(params.price_step ?? 0);
  if (!step) return base;
  const away = String(params.side ?? "bid").toLowerCase().startsWith("b") ? -1 : 1;
  return String(Number(base) + away * step * offset);
}

export async function build(req) {
  const params = { ...(req.params ?? {}) };
  const client = await clientFor(params);
  const account = envOrParam(params, "account", "BULLET_ACCOUNT_ADDRESS");

  const symbol = String(params.symbol ?? "BTC-USD").toUpperCase();
  const marketId = client.marketId(symbol);
  if (marketId === undefined) fail(`bullet exchangeInfo has no market for symbol ${symbol}`);

  const side = SIDES[String(params.side ?? "bid").toLowerCase()];
  if (side === undefined) fail(`unknown bullet side ${params.side}`);
  const orderTypeName = String(params.order_type ?? "post_only").toLowerCase();
  const orderType = ORDER_TYPES[orderTypeName];
  if (orderType === undefined) fail(`unknown bullet order type ${params.order_type}`);

  const count = req.scenario === "batch" ? Number(req.batch_size ?? 1) : 1;
  const ids = [];
  const orders = [];
  for (let offset = 0; offset < count; offset++) {
    const id = clientOrderId(params, req, offset);
    ids.push(id.toString());
    orders.push(new NewOrderArgs(
      priceForOffset(params, offset),
      String(params.size ?? "0.0001"),
      side,
      orderType,
      Boolean(params.reduce_only ?? false),
      id,
    ));
  }

  // Uniqueness is left to the SDK. Window is already its default mode, seeded
  // with a microsecond timestamp; passing our own millisecond value would cut
  // resolution 1000x and two transactions signed by this credential inside one
  // millisecond would collide, the second being dropped. Cleanup shares the
  // credential, so an order and its cancel can land in the same millisecond.
  const tx = Transaction.builder()
    .call(RuntimeCall.exchange(User.placeOrders(marketId, orders, false)))
    .build(client)
    .toBase64();

  const requestID = Number(req.iteration ?? 0) + 1;
  const wsBody = JSON.stringify({ method: "order.place", id: requestID, params: { tx } });
  const httpBody = JSON.stringify({ body: tx });

  return {
    body: httpBody,
    ws_body: wsBody,
    ws_batch_body: wsBody,
    metadata: {
      builder: "bullet-wasm-sdk",
      orders: orders.length,
      run_id: params.run_id ?? null,
      order_type: orderTypeName,
      market_id: marketId,
      client_order_ids: ids,
      cleanup_orders: ids.map((id) => ({ venue: "bullet", symbol, client_order_id: id })),
      native_batch_endpoint: true,
      batch_signing_model: "single_signature_tx",
      confirmation: params.confirmation === false ? {} : {
        venue: "bullet",
        ws_url: params.ws_url ?? client.wsUrl(),
        account,
        client_order_ids: ids,
        order_type: orderTypeName,
      },
    },
  };
}

async function main() {
  const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
  for await (const line of rl) {
    if (!line.trim()) continue;
    process.stdout.write(`${JSON.stringify(await build(JSON.parse(line)))}\n`);
  }
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    await main();
  } catch (err) {
    process.stderr.write(`${err instanceof BuilderError ? err.message : err.stack}\n`);
    process.exit(1);
  }
}
