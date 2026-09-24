import { requireAdvancedAuth } from "@/api/advanced-auth.server"
import {
  DEFAULT_SUMMARY_WINDOW,
  DEFAULT_WINDOW,
  isPublicSummaryWindowOption,
  isPublicWindowOption,
  isSummaryWindowOption,
  isWindowOption,
  type SummaryWindowOption,
  type WindowOption,
} from "@/api/bench"

export async function resolveBenchWindow(value: string | null): Promise<
  | { apiWindow: string; window: WindowOption }
  | { response: Response }
> {
  const window = value && isWindowOption(value) ? value : DEFAULT_WINDOW
  if (isPublicWindowOption(window)) {
    return { apiWindow: benchWindowToAPIParam(window), window }
  }

  const authError = await requireAdvancedAuth()
  return authError
    ? { response: authError }
    : { apiWindow: benchWindowToAPIParam(window), window }
}

export async function resolveSummaryWindow(value: string | null): Promise<
  | { apiWindow: string; limit: number; window: SummaryWindowOption }
  | { response: Response }
> {
  const window =
    value && isSummaryWindowOption(value) ? value : DEFAULT_SUMMARY_WINDOW
  if (isPublicSummaryWindowOption(window)) {
    return {
      apiWindow: summaryWindowToAPIParam(window),
      limit: summaryWindowLimit(window),
      window,
    }
  }

  const authError = await requireAdvancedAuth()
  return authError
    ? { response: authError }
    : {
        apiWindow: summaryWindowToAPIParam(window),
        limit: summaryWindowLimit(window),
        window,
      }
}

export function benchWindowToAPIParam(window: WindowOption) {
  switch (window) {
    case "7d":
      return "168h"
    case "30d":
      return "720h"
    case "90d":
      return "2160h"
    case "365d":
      return "8760h"
    default:
      return window
  }
}

function summaryWindowToAPIParam(window: SummaryWindowOption) {
  return window === "all" ? "all" : benchWindowToAPIParam(window)
}

function summaryWindowLimit(window: SummaryWindowOption) {
  return window === "all" ? 1_000_000 : 100_000
}
