import { createFileRoute } from "@tanstack/react-router"

import { createAdvancedLoginResponse } from "@/api/advanced-auth.server"

export const Route = createFileRoute("/api/bench/auth/login")({
  server: {
    handlers: {
      POST: async ({ request }) => createAdvancedLoginResponse(request),
    },
  },
})
