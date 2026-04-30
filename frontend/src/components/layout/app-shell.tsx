import { Activity } from "lucide-react"
import type { ReactNode } from "react"

export function AppShell({ children }: { children: ReactNode }) {
  return (
    <div className="grid-mesh min-h-svh">
      <header className="border-b border-border/80 bg-background/92 backdrop-blur">
        <div className="mx-auto flex h-14 w-full max-w-[1480px] items-center justify-between gap-4 px-3 sm:px-5">
          <div className="flex min-w-0 items-center gap-3">
            <div className="flex size-8 items-center justify-center rounded-sm border border-border bg-surface-1">
              <Activity className="size-4 text-primary" aria-hidden />
            </div>
            <div className="min-w-0">
              <div className="truncate font-sans text-sm font-semibold">
                Perps Latency Benchmark
              </div>
              <div className="truncate text-[11px] text-muted-foreground">
                Network response latency by venue, transport, and scenario
              </div>
            </div>
          </div>
        </div>
      </header>
      <main className="mx-auto w-full max-w-[1480px] px-3 py-3 sm:px-5 sm:py-4">
        {children}
      </main>
    </div>
  )
}
