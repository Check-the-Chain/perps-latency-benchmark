import { createFileRoute } from "@tanstack/react-router"

import { proxyBenchJSON } from "@/api/bench.server"
import { isWindowOption } from "@/api/bench"

const MAX_LIMIT = 5000

export const Route = createFileRoute("/api/bench/samples")({
  server: {
    handlers: {
      GET: async ({ request }) => {
        const url = new URL(request.url)
        const window = url.searchParams.get("window")
        const limit = Number(url.searchParams.get("limit"))
        const safeWindow = window && isWindowOption(window) ? window : "15m"
        const safeLimit =
          Number.isInteger(limit) && limit > 0
            ? Math.min(limit, MAX_LIMIT)
            : 2000

        return proxyBenchJSON(`/api/samples?window=${safeWindow}&limit=${safeLimit}`)
      },
    },
  },
})
