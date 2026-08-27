#!/usr/bin/env node
// Build Bullet cleanup cancel payloads, outside the measured latency window.
//
// Signing is delegated to the official Bullet WASM SDK, matching build_payload.mjs.

import * as readline from "node:readline";
import { pathToFileURL } from "node:url";
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
  // internal/cleanup/command.go supplies refs at params.order_refs; the earlier
  // fallback read params.sample through resultOrdersForVenue, which expects a
  // result object with a samples array and so could never match.
  if (!refs.length) refs = (params.order_refs ?? []).filter((r) => r?.venue === "bullet");
  const ids = normalizedIDs(refs);
  if (!ids.length) return cleanupResult(false, true, "no Bullet cleanup_orders");

  const client = await clientFor(builderParams);
  const symbol = String(builderParams.symbol ?? "BTC-USD").toUpperCase();
  const marketId = client.marketId(symbol);
  // marketId is `number | undefined`, and undefined coerces to 0 across the wasm
  // boundary — which would sign a valid cancel against market 0, cancel nothing,
  // and report cleanup success while orders stayed resting.
  if (marketId === undefined) {
    return cleanupResult(false, false, `bullet exchangeInfo has no market for symbol ${symbol}`);
  }
  const cancels = ids.map((id) => new CancelOrderArgs(null, BigInt(id)));

  // Uniqueness left to the SDK; see the note in build_payload.mjs. This path
  // shares the delegate credential with order submission, so a self-supplied
  // millisecond window could collide with the order it is cancelling.
  const tx = Transaction.builder()
    .call(RuntimeCall.exchange(User.cancelOrders(marketId, cancels)))
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

if (import.meta.url === pathToFileURL(process.argv[1]).href) await main();
