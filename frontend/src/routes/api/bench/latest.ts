import { createFileRoute } from "@tanstack/react-router"

import { proxyBenchJSON } from "@/api/bench.server"
import { resolveSummaryWindow } from "@/api/bench-window.server"

export const Route = createFileRoute("/api/bench/latest")({
  server: {
    handlers: {
      GET: async ({ request }) => {
        const url = new URL(request.url)
        const resolved = await resolveSummaryWindow(url.searchParams.get("window"))
        if ("response" in resolved) {
          return resolved.response
        }

        return proxyBenchJSON(
          `/api/latest?window=${resolved.apiWindow}&limit=${resolved.limit}`
        )
      },
    },
  },
})
