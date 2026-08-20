#!/usr/bin/env node
// Build Bullet cleanup cancel payloads, outside the measured latency window.
//
// Signing is delegated to the official Bullet WASM SDK, matching build_payload.mjs.

import * as readline from "node:readline";
import { CancelOrderArgs, RuntimeCall, Transaction, User } from "@bulletxyz/sdk-wasm";
import { clientFor, envOrParam } from "./build_payload.mjs";

// Mirrors internal/venues/cleanup_common.py so the JS builders honour the same
// cleanup contract as the Python venues.
export function cleanupResult(attempted, ok, description, metadata) {
  const result = { attempted, ok, description };
  if (!ok) result.error = description;
  if (metadata) result.metadata = metadata;
  return { cleanup: result };
}

export function cleanupOrdersForVenue(metadata, venue) {
  return (metadata?.cleanup_orders ?? []).filter((order) => order?.venue === venue);
}

export function resultOrdersForVenue(result, venue) {
  const refs = [];
  for (const sample of result?.samples ?? []) {
    const typed = (sample?.order_refs ?? []).filter((order) => order?.venue === venue);
    refs.push(...(typed.length ? typed : cleanupOrdersForVenue(sample?.metadata ?? {}, venue)));
  }
  return refs;
}

export function normalizedIDs(refs) {
  const out = [];
  for (const ref of refs) {
    if ((ref?.venue ?? "bullet") !== "bullet") continue;
    const raw = ref?.client_order_id ?? ref?.clientOrderId ?? ref?.co;
    if (raw === undefined || raw === null || String(raw) === "") continue;
    out.push(BigInt(raw).toString());
  }
  return out;
}

export async function build(req) {
  const params = { ...(req.params ?? {}) };
  const builderParams = { ...(params.builder_params ?? {}) };
  if ((params.phase ?? "after_sample") !== "after_sample") {
    return cleanupResult(false, true,
      "Bullet cleanup prepares per-sample order.cancel transactions; stale-order discovery uses the openOrders reconciliation read");
  }

  const metadata = params.metadata ?? {};
  let refs = cleanupOrdersForVenue(metadata, "bullet");
  if (!refs.length) refs = resultOrdersForVenue(params.sample ?? {}, "bullet");
  const ids = normalizedIDs(refs);
  if (!ids.length) return cleanupResult(false, true, "no Bullet cleanup_orders");

  const client = await clientFor(builderParams);
  const symbol = String(builderParams.symbol ?? "BTC-USD").toUpperCase();
  const marketId = client.marketId(symbol);
  const cancels = ids.map((id) => new CancelOrderArgs(null, BigInt(id)));

  const tx = Transaction.builder()
    .call(RuntimeCall.exchange(User.cancelOrders(marketId, cancels)))
    .window(BigInt(builderParams.uniqueness_ms ?? Date.now()))
    .build(client)
    .toBase64();

  const wsURL = builderParams.ws_url ?? client.wsUrl();
  return {
    ws_url: wsURL,
    body: JSON.stringify({ body: tx }),
    ws_body: JSON.stringify({ method: "order.cancel", id: 1, params: { tx } }),
    metadata: {
      cancel_confirmation: {
        venue: "bullet",
        ws_url: wsURL,
        account: envOrParam(builderParams, "account", "BULLET_ACCOUNT_ADDRESS"),
        client_order_ids: ids,
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

if (import.meta.url === `file://${process.argv[1]}`) await main();
