"use client"

import { useQuery } from "@tanstack/react-query"
import { RefreshCw } from "lucide-react"
import { useMemo, useState } from "react"

import {
  DEFAULT_WINDOW,
  healthQueryOptions,
  latestQueryOptions,
  samplesQueryOptions,
  type Sample,
  type SummaryRow,
} from "@/api/bench"
import {
  LatencyTimeseriesChart,
  type LatencyScaleMode,
} from "@/components/charts/latency-timeseries-chart"
import {
  DashboardFilterBar,
  type DashboardFilters,
} from "@/components/dashboard/filters"
import { InfrastructurePanel } from "@/components/dashboard/infrastructure-panel"
import { LatencyTable } from "@/components/dashboard/latency-table"
import { MethodologyPanel } from "@/components/dashboard/methodology-panel"
import { MetricCard } from "@/components/dashboard/metric-card"
import { StatusPill } from "@/components/dashboard/status-pill"
import { TakerCostPanel } from "@/components/dashboard/taker-cost-panel"
import {
  formatCount,
  formatLatency,
  formatTime,
} from "@/lib/format"
import { confirmP50, confirmP95 } from "@/lib/latency-metric"

const HIDDEN_FRONTEND_VENUES = new Set(["edgex"])
const GITHUB_URL = "https://github.com/Check-the-Chain/perps-latency-benchmark"

export function DashboardPage() {
  const [filters, setFilters] = useState<DashboardFilters>({
    venues: "all",
    window: DEFAULT_WINDOW,
  })
  const [chartScale, setChartScale] = useState<LatencyScaleMode>("linear")

  const health = useQuery(healthQueryOptions())
  const latest = useQuery(latestQueryOptions(filters.window))
  const samples = useQuery(samplesQueryOptions(filters.window))
  const summaries = latest.data?.summaries ?? []
  const measurements = samples.data?.samples ?? []
  const visibleSummaries = useMemo(
    () => summaries.filter((row) => isVisibleVenue(row.venue)),
    [summaries]
  )
  const visibleMeasurements = useMemo(
    () => measurements.filter((sample) => isVisibleVenue(sample.venue)),
    [measurements]
  )
  const postOnlySourceSamples = useMemo(
    () =>
      visibleMeasurements.filter(
        (sample) =>
          sample.scenario !== "batch" && isPostOnlyOrder(sample.order_type)
      ),
    [visibleMeasurements]
  )
  const batchPostOnlySourceSamples = useMemo(
    () =>
      visibleMeasurements.filter(
        (sample) =>
          sample.scenario === "batch" && isPostOnlyOrder(sample.order_type)
      ),
    [visibleMeasurements]
  )
  const takerSourceSamples = useMemo(
    () => visibleMeasurements.filter((sample) => isTakerOrder(sample.order_type)),
    [visibleMeasurements]
  )
  const filteredSummaries = useMemo(
    () => filterSummaries(visibleSummaries, filters),
    [filters, visibleSummaries]
  )
  const postOnlySamples = useMemo(
    () => filterSamples(postOnlySourceSamples, filters),
    [filters, postOnlySourceSamples]
  )
  const batchPostOnlySamples = useMemo(
    () => filterSamples(batchPostOnlySourceSamples, filters),
    [batchPostOnlySourceSamples, filters]
  )
  const takerSamples = useMemo(
    () => filterSamples(takerSourceSamples, filters),
    [filters, takerSourceSamples]
  )
  const postOnlyVenues = useMemo(
    () => venuesForSamples(postOnlySourceSamples),
    [postOnlySourceSamples]
  )
  const batchPostOnlyVenues = useMemo(
    () => venuesForSamples(batchPostOnlySourceSamples),
    [batchPostOnlySourceSamples]
  )
  const takerVenues = useMemo(
    () => venuesForSamples(takerSourceSamples),
    [takerSourceSamples]
  )
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
              <StatusPill>
                <span
                  className={`mr-1.5 size-1.5 rounded-full ${
                    health.isError ? "bg-loss" : "bg-profit"
                  }`}
                  aria-label={health.isError ? "Feed offline" : "Feed online"}
                />
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
              onChange={setFilters}
            />
            <a
              href={GITHUB_URL}
              target="_blank"
              rel="noreferrer"
              aria-label="Open GitHub repository"
              title="Open GitHub repository"
              className="inline-flex h-8 w-8 items-center justify-center rounded-sm border border-border bg-surface-1 text-muted-foreground hover:bg-surface-2 hover:text-foreground"
            >
              <svg
                className="size-3.5"
                viewBox="0 0 24 24"
                fill="currentColor"
                aria-hidden
              >
                <path d="M12 0C5.37 0 0 5.5 0 12.28c0 5.43 3.44 10.03 8.2 11.66.6.11.82-.27.82-.59 0-.29-.01-1.26-.02-2.28-3.34.74-4.04-1.45-4.04-1.45-.55-1.42-1.34-1.8-1.34-1.8-1.09-.76.08-.74.08-.74 1.2.09 1.84 1.27 1.84 1.27 1.07 1.88 2.81 1.34 3.5 1.02.11-.79.42-1.34.76-1.64-2.67-.31-5.47-1.37-5.47-6.08 0-1.34.47-2.44 1.24-3.3-.13-.31-.54-1.56.12-3.25 0 0 1.01-.33 3.3 1.26a11.18 11.18 0 0 1 6.01 0c2.29-1.59 3.3-1.26 3.3-1.26.66 1.69.25 2.94.12 3.25.77.86 1.24 1.96 1.24 3.3 0 4.73-2.81 5.76-5.48 6.07.43.38.81 1.12.81 2.26 0 1.64-.01 2.96-.01 3.36 0 .33.21.71.82.59A12.27 12.27 0 0 0 24 12.28C24 5.5 18.63 0 12 0Z" />
              </svg>
            </a>
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

      <section className="grid gap-3 md:grid-cols-2">
        <MetricCard
          label="Best post-only p50"
          value={formatLatency(stats.fastestPostOnlyP50 ? confirmP50(stats.fastestPostOnlyP50) : undefined)}
          detail={formatWinnerDetail(stats.fastestPostOnlyP50)}
          tone="good"
        />
        <MetricCard
          label="Best taker p50"
          value={formatLatency(stats.fastestTakerP50 ? confirmP50(stats.fastestTakerP50) : undefined)}
          detail={formatWinnerDetail(stats.fastestTakerP50)}
          tone="good"
        />
      </section>

      <InfrastructurePanel />
      <LatencyTimeseriesChart
        title="Post-only Confirmation"
        description="How quickly a resting order is confirmed as placed."
        samples={postOnlySamples}
        scaleMode={chartScale}
        selectedVenues={selectedVenueList(filters.venues, postOnlyVenues)}
        venues={postOnlyVenues}
        onScaleModeChange={setChartScale}
        onVenueSelectionChange={(venues) =>
          setFilters((current) => ({ ...current, venues }))
        }
      />
      <LatencyTimeseriesChart
        title="Batch Post-only Confirmation"
        description="Five post-only orders per sample. Hyperliquid, Lighter, and Aster use native batch submits; Extended sends five single-order requests concurrently because no native batch endpoint is documented."
        samples={batchPostOnlySamples}
        scaleMode={chartScale}
        selectedVenues={selectedVenueList(filters.venues, batchPostOnlyVenues)}
        venues={batchPostOnlyVenues}
        onScaleModeChange={setChartScale}
        onVenueSelectionChange={(venues) =>
          setFilters((current) => ({ ...current, venues }))
        }
      />
      <LatencyTimeseriesChart
        title="Taker Confirmation"
        description="How quickly a marketable order is confirmed, adjusted for published venue delays."
        samples={takerSamples}
        scaleMode={chartScale}
        selectedVenues={selectedVenueList(filters.venues, takerVenues)}
        venues={takerVenues}
        onScaleModeChange={setChartScale}
        onVenueSelectionChange={(venues) =>
          setFilters((current) => ({ ...current, venues }))
        }
      />
      <TakerCostPanel samples={takerSamples} />
      <LatencyTable rows={filteredSummaries} />
      <MethodologyPanel />
    </div>
  )
}

function filterSummaries(rows: Array<SummaryRow>, filters: DashboardFilters) {
  return rows.filter((row) => matchesVenue(filters.venues, row.venue))
}

function filterSamples(samples: Array<Sample>, filters: DashboardFilters) {
  return samples.filter((sample) => matchesVenue(filters.venues, sample.venue))
}

function getStats(rows: Array<SummaryRow>) {
  const measurementCount = rows.reduce((sum, row) => sum + row.count, 0)
  const ok = rows.reduce((sum, row) => sum + row.ok, 0)
  const failed = rows.reduce((sum, row) => sum + row.failed, 0)
  const rankedRows = rows.filter(isRankableSummaryRow)
  const postOnlyRows = rankedRows.filter((row) => isPostOnlyOrder(row.order_type))
  const takerRows = rankedRows.filter((row) => isTakerOrder(row.order_type))

  return {
    failed,
    failureRate: measurementCount > 0 ? failed / measurementCount : 0,
    fastestPostOnlyP50: minBy(postOnlyRows, (row) => confirmP50(row) ?? Number.NaN),
    fastestTakerP50: minBy(takerRows, (row) => confirmP50(row) ?? Number.NaN),
    measurementCount,
    ok,
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

function matchesVenue(filter: DashboardFilters["venues"], value: string) {
  return filter === "all" || filter.includes(value)
}

function selectedVenueList(
  filter: DashboardFilters["venues"],
  options: Array<string>
) {
  if (filter === "all") {
    return options
  }

  const available = new Set(options)
  return filter.filter((venue) => available.has(venue))
}

function venuesForSamples(samples: Array<Sample>) {
  return uniqueSorted(samples.map((sample) => sample.venue))
}

function uniqueSorted(values: Array<string>) {
  return [...new Set(values)].sort((a, b) => a.localeCompare(b))
}

function isVisibleVenue(venue: string) {
  return !HIDDEN_FRONTEND_VENUES.has(venue.toLowerCase())
}

function formatWinnerDetail(row: SummaryRow | null) {
  if (!row) {
    return "no matching data"
  }

  return `${formatVenue(row.venue)} / p95 ${formatLatency(confirmP95(row))} / ${formatCount(row.ok)} samples`
}

function isRankableSummaryRow(row: SummaryRow) {
  const p50 = confirmP50(row)
  return row.ok > 0 && Number.isFinite(p50) && p50 > 0
}

function orderType(value: string | undefined) {
  return value && value.length > 0 ? value : "unknown"
}

function isPostOnlyOrder(value: string | undefined) {
  return orderType(value).toLowerCase() === "post_only"
}

function isTakerOrder(value: string | undefined) {
  const normalized = orderType(value).toLowerCase()
  return ["market", "ioc", "immediate_or_cancel", "fok", "fill_or_kill"].includes(normalized)
}

function formatVenue(value: string) {
  return value
    .split(/[_-]/)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ")
}
