import { createFileRoute } from "@tanstack/react-router"

import { proxyBenchJSONWithFallback } from "@/api/bench.server"
import { DEFAULT_WINDOW, isWindowOption } from "@/api/bench"

const MAX_LIMIT = 10000

export const Route = createFileRoute("/api/bench/latency-series")({
  server: {
    handlers: {
      GET: async ({ request }) => {
        const url = new URL(request.url)
        const window = url.searchParams.get("window")
        const limit = Number(url.searchParams.get("limit"))
        const safeWindow = window && isWindowOption(window) ? window : DEFAULT_WINDOW
        const safeLimit =
          Number.isInteger(limit) && limit > 0
            ? Math.min(limit, MAX_LIMIT)
            : 2000

        return proxyBenchJSONWithFallback(
          `/api/dashboard/latency-series?window=${safeWindow}&limit=${safeLimit}`,
          [
            `/api/dashboard/samples?window=${safeWindow}&limit=${safeLimit}`,
            `/api/samples?window=${safeWindow}&limit=${safeLimit}`,
          ]
        )
      },
    },
  },
})
