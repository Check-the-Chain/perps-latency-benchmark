import {
  HeadContent,
  Outlet,
  Scripts,
  createRootRouteWithContext,
} from "@tanstack/react-router"
import type { QueryClient } from "@tanstack/react-query"
import type { ReactNode } from "react"

import { AppShell } from "@/components/layout/app-shell"
import { ThemeProvider } from "@/components/theme-provider"
import appCss from "../styles.css?url"

export interface RouterContext {
  queryClient: QueryClient
}

export const Route = createRootRouteWithContext<RouterContext>()({
  head: () => ({
    meta: [
      { charSet: "utf-8" },
      { name: "viewport", content: "width=device-width, initial-scale=1" },
      { name: "theme-color", media: "(prefers-color-scheme: light)", content: "#fbfbfb" },
      { name: "theme-color", media: "(prefers-color-scheme: dark)", content: "#101217" },
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
  component: RootComponent,
  notFoundComponent: NotFound,
})

function RootComponent() {
  return (
    <RootDocument>
      <AppShell>
        <Outlet />
      </AppShell>
    </RootDocument>
  )
}

function RootDocument({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <HeadContent />
      </head>
      <body>
        <ThemeProvider defaultTheme="system" storageKey="theme">
          {children}
        </ThemeProvider>
        <Scripts />
      </body>
    </html>
  )
}

function NotFound() {
  return (
    <div className="rounded-sm border border-border/80 bg-surface-1 p-6 text-sm text-muted-foreground">
      Page not found.
    </div>
  )
}
