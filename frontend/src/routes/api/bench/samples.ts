import { createFileRoute } from "@tanstack/react-router"

import { proxyBenchJSONWithFallback } from "@/api/bench.server"
import { resolveBenchWindow } from "@/api/bench-window.server"

const MAX_LIMIT = 10000

export const Route = createFileRoute("/api/bench/samples")({
  server: {
    handlers: {
      GET: async ({ request }) => {
        const url = new URL(request.url)
        const limit = Number(url.searchParams.get("limit"))
        const resolved = await resolveBenchWindow(url.searchParams.get("window"))
        if ("response" in resolved) {
          return resolved.response
        }
        const safeLimit =
          Number.isInteger(limit) && limit > 0
            ? Math.min(limit, MAX_LIMIT)
            : 2000

        return proxyBenchJSONWithFallback(
          `/api/dashboard/samples?window=${resolved.apiWindow}&limit=${safeLimit}`,
          `/api/samples?window=${resolved.apiWindow}&limit=${safeLimit}`
        )
      },
    },
  },
})
