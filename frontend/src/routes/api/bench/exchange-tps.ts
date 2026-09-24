import { createFileRoute } from "@tanstack/react-router"

import { proxyBenchJSON } from "@/api/bench.server"
import { resolveBenchWindow } from "@/api/bench-window.server"
import {
  EXCHANGE_TPS_BUCKETS,
  exchangeTPSBucketForWindow,
  exchangeTPSWindowLimit,
  type ExchangeTPSBucket,
} from "@/api/bench"

const MAX_LIMIT = 100000

export const Route = createFileRoute("/api/bench/exchange-tps")({
  server: {
    handlers: {
      GET: async ({ request }) => {
        const url = new URL(request.url)
        const bucket = url.searchParams.get("bucket")
        const limit = Number(url.searchParams.get("limit"))
        const resolved = await resolveBenchWindow(url.searchParams.get("window"))
        if ("response" in resolved) {
          return resolved.response
        }
        const safeBucket: ExchangeTPSBucket = EXCHANGE_TPS_BUCKETS.includes(
          bucket as ExchangeTPSBucket
        )
          ? (bucket as ExchangeTPSBucket)
          : exchangeTPSBucketForWindow(resolved.window)
        const safeLimit =
          Number.isInteger(limit) && limit > 0
            ? Math.min(limit, MAX_LIMIT)
            : exchangeTPSWindowLimit(resolved.window, safeBucket)

        return proxyBenchJSON(
          `/api/exchange-tps?window=${resolved.apiWindow}&bucket=${safeBucket}&limit=${safeLimit}`
        )
      },
    },
  },
})
