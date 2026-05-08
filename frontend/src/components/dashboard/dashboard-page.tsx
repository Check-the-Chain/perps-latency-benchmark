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
import { VenueName } from "@/components/dashboard/venue-logo"
import {
  formatCount,
  formatLatency,
  formatTime,
} from "@/lib/format"
import {
  cancelSampleMs,
  confirmP50,
  confirmP95,
  confirmSampleMs,
} from "@/lib/latency-metric"

const HIDDEN_FRONTEND_VENUES = new Set(["edgex"])
const GITHUB_URL = "https://github.com/Check-the-Chain/perps-latency-benchmark"
type CancelChartScenario = "single" | "batch"

export function DashboardPage() {
  const [filters, setFilters] = useState<DashboardFilters>({
    subtractNetworkFloor: false,
    venues: "all",
    window: DEFAULT_WINDOW,
  })
  const [chartScale, setChartScale] = useState<LatencyScaleMode>("log")
  const [cancelChartScenario, setCancelChartScenario] =
    useState<CancelChartScenario>("single")

  const health = useQuery(healthQueryOptions())
  const latest = useQuery(latestQueryOptions(filters.window))
  const samples = useQuery(samplesQueryOptions(filters.window))
  const measurements = samples.data?.samples ?? []
  const visibleMeasurements = useMemo(
    () => measurements.filter((sample) => isVisibleVenue(sample.venue)),
    [measurements]
  )
  const visibleSummaries = useMemo(
    () =>
      (latest.data?.summaries ?? []).filter((row) => isVisibleVenue(row.venue)),
    [latest.data?.summaries]
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
  const cancelSamples =
    cancelChartScenario === "batch" ? batchPostOnlySamples : postOnlySamples
  const confirmationValueForSample = useMemo(
    () => (sample: Sample) =>
      confirmSampleMs(sample, filters.subtractNetworkFloor),
    [filters.subtractNetworkFloor]
  )
  const cancelValueForSample = useMemo(
    () => (sample: Sample) =>
      cancelSampleMs(sample, filters.subtractNetworkFloor),
    [filters.subtractNetworkFloor]
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
  const cancelVenues =
    cancelChartScenario === "batch" ? batchPostOnlyVenues : postOnlyVenues
  const stats = useMemo(
    () => getStats(filteredSummaries, filters.subtractNetworkFloor),
    [filteredSummaries, filters.subtractNetworkFloor]
  )

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
                width="14"
                height="14"
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
          label="Best post-only p95"
          value={formatLatency(stats.fastestPostOnlyP95 ? confirmP95(stats.fastestPostOnlyP95, filters.subtractNetworkFloor) : undefined)}
          detail={formatWinnerDetail(stats.fastestPostOnlyP95, filters.subtractNetworkFloor, "p50")}
          tone="good"
        />
        <MetricCard
          label="Best taker p50"
          value={formatLatency(stats.fastestTakerP50 ? confirmP50(stats.fastestTakerP50, filters.subtractNetworkFloor) : undefined)}
          detail={formatWinnerDetail(stats.fastestTakerP50, filters.subtractNetworkFloor, "p95")}
          tone="good"
        />
      </section>

      <LatencyTable
        rows={filteredSummaries}
        subtractNetworkFloor={filters.subtractNetworkFloor}
      />
      <InfrastructurePanel />
      <LatencyTimeseriesChart
        title="Post-only Confirmation"
        description="How quickly a resting order is confirmed as placed."
        samples={postOnlySamples}
        scaleMode={chartScale}
        selectedVenues={selectedVenueList(filters.venues, postOnlyVenues)}
        venues={postOnlyVenues}
        valueForSample={confirmationValueForSample}
        onScaleModeChange={setChartScale}
        onVenueSelectionChange={(venues) =>
          setFilters((current) => ({ ...current, venues }))
        }
      />
      <LatencyTimeseriesChart
        title="Batch Post-only Confirmation"
        description="Five post-only orders per sample. Native batch venues are labeled separately from manual fanout venues that send concurrent single-order requests."
        samples={batchPostOnlySamples}
        scaleMode={chartScale}
        selectedVenues={selectedVenueList(filters.venues, batchPostOnlyVenues)}
        venues={batchPostOnlyVenues}
        valueForSample={confirmationValueForSample}
        onScaleModeChange={setChartScale}
        onVenueSelectionChange={(venues) =>
          setFilters((current) => ({ ...current, venues }))
        }
      />
      <LatencyTimeseriesChart
        title="Cancel Confirmation"
        description={
          cancelChartScenario === "batch"
            ? "Five post-only cleanup cancels per sample, measured when every cancel is confirmed through the account feed."
            : "Post-only cleanup cancel latency, measured when the cancel is confirmed through the account feed."
        }
        emptyMessage="No account-feed cancel confirmation data is available for the selected filters."
        headerActions={
          <CancelScenarioToggle
            value={cancelChartScenario}
            onChange={setCancelChartScenario}
          />
        }
        samples={cancelSamples}
        scaleMode={chartScale}
        selectedVenues={selectedVenueList(filters.venues, cancelVenues)}
        venues={cancelVenues}
        valueForSample={cancelValueForSample}
        valueLabel="Cancel confirmation"
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
        valueForSample={confirmationValueForSample}
        onScaleModeChange={setChartScale}
        onVenueSelectionChange={(venues) =>
          setFilters((current) => ({ ...current, venues }))
        }
      />
      <TakerCostPanel samples={takerSamples} />
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

function getStats(rows: Array<SummaryRow>, subtractNetworkFloor: boolean) {
  const measurementCount = rows.reduce((sum, row) => sum + row.count, 0)
  const rankedRows = rows.filter(isRankableSummaryRow)
  const postOnlyRows = rankedRows.filter((row) => isPostOnlyOrder(row.order_type))
  const takerRows = rankedRows.filter((row) => isTakerOrder(row.order_type))

  return {
    fastestPostOnlyP95: minBy(postOnlyRows, (row) => confirmP95(row, subtractNetworkFloor) ?? Number.NaN),
    fastestTakerP50: minBy(takerRows, (row) => confirmP50(row, subtractNetworkFloor) ?? Number.NaN),
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

function formatWinnerDetail(row: SummaryRow | null, subtractNetworkFloor = false, companionMetric: "p50" | "p95" = "p95") {
  if (!row) {
    return "no matching data"
  }

  const companionValue =
    companionMetric === "p50"
      ? confirmP50(row, subtractNetworkFloor)
      : confirmP95(row, subtractNetworkFloor)

  return (
    <span className="inline-flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5">
      <VenueName venue={row.venue} />
      <span>/ {companionMetric} {formatLatency(companionValue)}</span>
      <span>/ {formatCount(row.ok)} samples</span>
    </span>
  )
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

function CancelScenarioToggle({
  onChange,
  value,
}: {
  onChange: (value: CancelChartScenario) => void
  value: CancelChartScenario
}) {
  const options: Array<{ label: string; value: CancelChartScenario }> = [
    { label: "Single", value: "single" },
    { label: "Batch", value: "batch" },
  ]

  return (
    <div className="flex h-8 overflow-hidden rounded-sm border border-border bg-surface-1 text-[11px]">
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          onClick={() => onChange(option.value)}
          className={`px-2.5 ${
            value === option.value
              ? "bg-foreground text-background"
              : "text-muted-foreground hover:bg-surface-2 hover:text-foreground"
          }`}
          aria-pressed={value === option.value}
        >
          {option.label}
        </button>
      ))}
    </div>
  )
}
