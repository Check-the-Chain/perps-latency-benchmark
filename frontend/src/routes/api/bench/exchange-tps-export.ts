import { createFileRoute } from "@tanstack/react-router"

import { fetchBenchJSON } from "@/api/bench.server"
import {
  exchangeTPSBucketForWindow,
  exchangeTPSWindowLimit,
  type ExchangeTPSResponse,
} from "@/api/bench"
import { resolveBenchWindow } from "@/api/bench-window.server"

export const Route = createFileRoute("/api/bench/exchange-tps-export")({
  server: {
    handlers: {
      GET: async ({ request }) => {
        const url = new URL(request.url)
        const resolved = await resolveBenchWindow(url.searchParams.get("window"))
        if ("response" in resolved) {
          return resolved.response
        }

        const bucket = exchangeTPSBucketForWindow(resolved.window)
        const data = await fetchBenchJSON<ExchangeTPSResponse>(
          `/api/exchange-tps?window=${resolved.apiWindow}&bucket=${bucket}&limit=${exchangeTPSWindowLimit(resolved.window, bucket)}`
        )
        const venues = selectedVenues(url.searchParams.get("venues"))
        const filteredData = venues
          ? {
              ...data,
              series: data.series.filter((row) => venues.has(row.venue)),
            }
          : data
        const headers = new Headers()
        headers.set("Content-Type", "text/csv; charset=utf-8")
        headers.set(
          "Content-Disposition",
          `attachment; filename="exchange-tps-${resolved.window}-${bucket}.csv"`
        )
        return new Response(exchangeTPSCSV(filteredData), { headers })
      },
    },
  },
})

function selectedVenues(value: string | null) {
  if (value === null) {
    return null
  }
  return new Set(
    value
      .split(",")
      .map((venue) => venue.trim())
      .filter(Boolean)
  )
}

function exchangeTPSCSV(data: ExchangeTPSResponse) {
  const header = [
    "bucket_start",
    "bucket_seconds",
    "venue",
    "tps",
    "tx_count",
    "block_count",
    "order_count",
    "orders_per_second",
    "place_count",
    "cancel_count",
  ]
  const rows = data.series.map((row) =>
    [
      row.bucket_start,
      row.bucket_seconds,
      row.venue,
      row.tps,
      row.tx_count,
      row.block_count ?? "",
      row.order_count ?? "",
      row.orders_per_second ?? "",
      row.place_count ?? "",
      row.cancel_count ?? "",
    ]
      .map(csvCell)
      .join(",")
  )
  return [header.join(","), ...rows].join("\n")
}

function csvCell(value: string | number) {
  const text = String(value)
  return /[",\n]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text
}
