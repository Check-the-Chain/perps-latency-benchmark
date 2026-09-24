import { createFileRoute } from "@tanstack/react-router"

import { proxyBenchJSON } from "@/api/bench.server"

export const Route = createFileRoute("/api/bench/health")({
  server: {
    handlers: {
      GET: async () => proxyBenchJSON("/api/health"),
    },
  },
})
