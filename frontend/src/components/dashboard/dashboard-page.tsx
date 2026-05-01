"use client"

import { useQuery } from "@tanstack/react-query"
import { RefreshCw } from "lucide-react"
import { useMemo, useState } from "react"

import {
  healthQueryOptions,
  latestQueryOptions,
  samplesQueryOptions,
  type Sample,
  type SummaryRow,
} from "@/api/bench"
import { LatencyTimeseriesChart } from "@/components/charts/latency-timeseries-chart"
import {
  DashboardFilterBar,
  type DashboardFilters,
} from "@/components/dashboard/filters"
import { LatencyTable } from "@/components/dashboard/latency-table"
import { MetricCard } from "@/components/dashboard/metric-card"
import { StatusPill } from "@/components/dashboard/status-pill"
import {
  formatCount,
  formatLatency,
  formatPercent,
  formatTime,
} from "@/lib/format"

export function DashboardPage() {
  const [filters, setFilters] = useState<DashboardFilters>({
    orderType: "all",
    scenario: "all",
    transport: "all",
    venue: "all",
    window: "15m",
  })

  const health = useQuery(healthQueryOptions())
  const latest = useQuery(latestQueryOptions(filters.window))
  const samples = useQuery(samplesQueryOptions(filters.window))
  const summaries = latest.data?.summaries ?? []
  const measurements = samples.data?.samples ?? []
  const filteredSummaries = useMemo(
    () => filterSummaries(summaries, filters),
    [filters, summaries]
  )
  const filteredSamples = useMemo(
    () => filterSamples(measurements, filters),
    [filters, measurements]
  )
  const options = useMemo(() => getFilterOptions(summaries), [summaries])
  const stats = useMemo(() => getStats(filteredSummaries), [filteredSummaries])

  return (
    <div className="space-y-3">
      <section className="rounded-sm border border-border/80 bg-surface-1 p-3">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h1 className="font-sans text-lg font-semibold">
              Live Benchmark Results
            </h1>
            <div className="mt-2 flex flex-wrap gap-2">
              <StatusPill tone={health.isError ? "bad" : "good"}>
                Feed {health.isError ? "offline" : "online"}
              </StatusPill>
              <StatusPill>
                Updated {formatTime(latest.data?.updated_at)}
              </StatusPill>
              <StatusPill>
                {formatCount(stats.measurementCount)} measurements
              </StatusPill>
            </div>
          </div>
          <div className="flex flex-wrap items-center justify-end gap-2">
            <DashboardFilterBar
              filters={filters}
              scenarios={options.scenarios}
              orderTypes={options.orderTypes}
              transports={options.transports}
              venues={options.venues}
              onChange={setFilters}
            />
            <button
              type="button"
              onClick={() => {
                void latest.refetch()
                void samples.refetch()
                void health.refetch()
              }}
              className="inline-flex h-8 items-center gap-2 rounded-sm border border-border bg-surface-1 px-2 text-[11px] text-foreground"
            >
              <RefreshCw className="size-3.5" aria-hidden />
              Refresh
            </button>
          </div>
        </div>
      </section>

      <section className="grid gap-3 md:grid-cols-3">
        <MetricCard
          label="Fastest p50"
          value={formatLatency(stats.fastestP50?.p50_ms)}
          detail={formatRowLabel(stats.fastestP50)}
          tone="good"
        />
        <MetricCard
          label="Fastest p95"
          value={formatLatency(stats.fastestP95?.p95_ms)}
          detail={formatRowLabel(stats.fastestP95)}
          tone="good"
        />
        <MetricCard
          label="Error rate"
          value={formatPercent(stats.failureRate)}
          detail={`${formatCount(stats.failed)} rejected or errored`}
          tone={stats.failed > 0 ? "warning" : "neutral"}
        />
      </section>

      <LatencyTimeseriesChart samples={filteredSamples} />
      <LatencyTable rows={filteredSummaries} />
    </div>
  )
}

function filterSummaries(rows: Array<SummaryRow>, filters: DashboardFilters) {
  return rows.filter(
    (row) =>
      matches(filters.venue, row.venue) &&
      matches(filters.transport, row.transport) &&
      matches(filters.scenario, row.scenario) &&
      matches(filters.orderType, orderType(row.order_type))
  )
}

function filterSamples(samples: Array<Sample>, filters: DashboardFilters) {
  return samples.filter(
    (sample) =>
      matches(filters.venue, sample.venue) &&
      matches(filters.transport, sample.transport) &&
      matches(filters.scenario, sample.scenario) &&
      matches(filters.orderType, orderType(sample.order_type))
  )
}

function getFilterOptions(rows: Array<SummaryRow>) {
  return {
    venues: uniqueSorted(rows.map((row) => row.venue)),
    transports: uniqueSorted(rows.map((row) => row.transport)),
    scenarios: uniqueSorted(rows.map((row) => row.scenario)),
    orderTypes: uniqueSorted(rows.map((row) => orderType(row.order_type))),
  }
}

function getStats(rows: Array<SummaryRow>) {
  const measurementCount = rows.reduce((sum, row) => sum + row.count, 0)
  const failed = rows.reduce((sum, row) => sum + row.failed, 0)

  return {
    failed,
    failureRate: measurementCount > 0 ? failed / measurementCount : 0,
    fastestP50: minBy(rows, (row) => row.p50_ms),
    fastestP95: minBy(rows, (row) => row.p95_ms),
    measurementCount,
  }
}

function minBy<T>(items: Array<T>, score: (item: T) => number) {
  let best: T | null = null
  let bestScore = Number.POSITIVE_INFINITY

  for (const item of items) {
    const itemScore = score(item)
    if (Number.isFinite(itemScore) && itemScore < bestScore) {
      best = item
      bestScore = itemScore
    }
  }

  return best
}

function matches(filter: string, value: string) {
  return filter === "all" || filter === value
}

function uniqueSorted(values: Array<string>) {
  return [...new Set(values)].sort((a, b) => a.localeCompare(b))
}

function formatRowLabel(row: SummaryRow | null) {
  if (!row) {
    return "no matching data"
  }

  return `${row.venue} / ${row.transport} / ${row.scenario} / ${orderType(row.order_type)} / ${measurementLabel(row.measurement_mode)}`
}

function orderType(value: string | undefined) {
  return value && value.length > 0 ? value : "unknown"
}

function measurementLabel(value: string | undefined) {
  return value === "ws_confirmation" ? "WS confirmation" : "Submit response"
}
