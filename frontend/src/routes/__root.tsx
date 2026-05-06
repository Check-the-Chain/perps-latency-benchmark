import {
  HeadContent,
  Outlet,
  Scripts,
  createRootRouteWithContext,
} from "@tanstack/react-router"
import type { QueryClient } from "@tanstack/react-query"

import { AppShell } from "@/components/layout/app-shell"
import appCss from "../styles.css?url"

export interface RouterContext {
  queryClient: QueryClient
}

export const Route = createRootRouteWithContext<RouterContext>()({
  head: () => ({
    meta: [
      { charSet: "utf-8" },
      { name: "viewport", content: "width=device-width, initial-scale=1" },
      { name: "theme-color", content: "#fcfcfc" },
      { title: "Perps Latency Benchmark" },
      {
        name: "description",
        content: "Live network latency benchmark dashboard for perps venues.",
      },
    ],
    links: [
      { rel: "stylesheet", href: appCss },
      { rel: "icon", type: "image/svg+xml", href: "/favicon.svg" },
    ],
  }),
  shellComponent: RootDocument,
  component: RootLayout,
  notFoundComponent: NotFound,
})

function RootDocument({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <head>
        <HeadContent />
      </head>
      <body>
        {children}
        <Scripts />
      </body>
    </html>
  )
}

function RootLayout() {
  return (
    <AppShell>
      <Outlet />
    </AppShell>
  )
}

function NotFound() {
  return (
    <div className="rounded-sm border border-border/80 bg-surface-1 p-6 text-sm text-muted-foreground">
      Page not found.
    </div>
  )
}
