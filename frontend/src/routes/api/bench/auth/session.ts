import { createFileRoute } from "@tanstack/react-router"

import { advancedAuthStatus } from "@/api/advanced-auth.server"

export const Route = createFileRoute("/api/bench/auth/session")({
  server: {
    handlers: {
      GET: async () =>
        Response.json(await advancedAuthStatus(), {
          headers: { "Cache-Control": "no-store" },
        }),
    },
  },
})
