const latencyFormatter = new Intl.NumberFormat("en", {
  maximumFractionDigits: 1,
  minimumFractionDigits: 0,
})

const countFormatter = new Intl.NumberFormat("en")

const timeFormatter = new Intl.DateTimeFormat("en", {
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
})

export function formatLatency(ms: number | null | undefined) {
  if (ms === null || ms === undefined || !Number.isFinite(ms)) {
    return "-"
  }

  return `${latencyFormatter.format(ms)} ms`
}

export function formatCount(value: number | null | undefined) {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return "-"
  }

  return countFormatter.format(value)
}

export function formatPercent(value: number) {
  if (!Number.isFinite(value)) {
    return "-"
  }

  return `${Math.round(value * 100)}%`
}

export function formatTime(value: string | Date | null | undefined) {
  if (!value) {
    return "-"
  }

  const date = typeof value === "string" ? new Date(value) : value
  if (Number.isNaN(date.getTime())) {
    return "-"
  }

  return timeFormatter.format(date)
}

export function nsToMs(value: number) {
  return value / 1_000_000
}
