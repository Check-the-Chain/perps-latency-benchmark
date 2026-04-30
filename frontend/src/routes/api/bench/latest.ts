import { createFileRoute } from "@tanstack/react-router"

import { proxyBenchJSON } from "@/api/bench.server"
import { isWindowOption } from "@/api/bench"

export const Route = createFileRoute("/api/bench/latest")({
  server: {
    handlers: {
      GET: async ({ request }) => {
        const url = new URL(request.url)
        const window = url.searchParams.get("window")
        const safeWindow = window && isWindowOption(window) ? window : "15m"

        return proxyBenchJSON(`/api/latest?window=${safeWindow}`)
      },
    },
  },
})
