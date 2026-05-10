import { createFileRoute } from "@tanstack/react-router"

import { proxyBenchJSON } from "@/api/bench.server"
import {
  DEFAULT_EXCHANGE_TPS_BUCKET,
  EXCHANGE_TPS_BUCKETS,
  DEFAULT_WINDOW,
  isWindowOption,
} from "@/api/bench"

const MAX_LIMIT = 20000

export const Route = createFileRoute("/api/bench/exchange-tps")({
  server: {
    handlers: {
      GET: async ({ request }) => {
        const url = new URL(request.url)
        const window = url.searchParams.get("window")
        const bucket = url.searchParams.get("bucket")
        const limit = Number(url.searchParams.get("limit"))
        const safeWindow = window && isWindowOption(window) ? window : DEFAULT_WINDOW
        const safeBucket = EXCHANGE_TPS_BUCKETS.includes(
          bucket as (typeof EXCHANGE_TPS_BUCKETS)[number]
        )
          ? bucket
          : DEFAULT_EXCHANGE_TPS_BUCKET
        const safeLimit =
          Number.isInteger(limit) && limit > 0
            ? Math.min(limit, MAX_LIMIT)
            : 10000

        return proxyBenchJSON(
          `/api/exchange-tps?window=${safeWindow}&bucket=${safeBucket}&limit=${safeLimit}`
        )
      },
    },
  },
})
