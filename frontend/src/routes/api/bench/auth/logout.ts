import { createFileRoute } from "@tanstack/react-router"

import { createAdvancedLogoutResponse } from "@/api/advanced-auth.server"

export const Route = createFileRoute("/api/bench/auth/logout")({
  server: {
    handlers: {
      POST: async () => createAdvancedLogoutResponse(),
    },
  },
})
