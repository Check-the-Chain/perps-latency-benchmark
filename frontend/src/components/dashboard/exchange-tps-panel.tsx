"use client"

import { AxisBottom, AxisLeft } from "@visx/axis"
import { GridRows } from "@visx/grid"
import { Group } from "@visx/group"
import { ParentSize } from "@visx/responsive"
import { scaleLinear, scaleTime } from "@visx/scale"
import { LinePath } from "@visx/shape"
import { useMemo } from "react"

import type { ExchangeTPSRow, ExchangeTPSSource } from "@/api/bench"
import { colorForVenue } from "@/components/charts/latency-timeseries-chart"
import { VenueName } from "@/components/dashboard/venue-logo"
import { formatCount, formatTime } from "@/lib/format"

const MARGIN = { top: 16, right: 18, bottom: 32, left: 58 }

interface TPSPoint {
  date: Date
  tps: number
}

interface TPSSeries {
  color: string
  points: Array<TPSPoint>
  venue: string
}

export function ExchangeTPSPanel({
  isLoading = false,
  rows,
  sources,
}: {
  isLoading?: boolean
  rows: Array<ExchangeTPSRow>
  sources: Array<ExchangeTPSSource>
}) {
  const completeRows = useMemo(
    () => rows.filter((row) => row.complete && Number.isFinite(row.tps)),
    [rows]
  )
  const latestRows = useMemo(() => latestRowsByVenue(completeRows), [completeRows])
  const series = useMemo(() => buildSeries(completeRows), [completeRows])

  if (isLoading) {
    return (
      <section
        className="flex min-h-40 flex-col items-center justify-center gap-3 rounded-sm border border-border/80 bg-surface-1 p-3 text-[11px] text-muted-foreground"
        aria-label="Loading exchange TPS data"
        role="status"
      >
        <div className="size-5 animate-spin rounded-full border-2 border-border border-t-primary" />
        <span>Loading exchange TPS data</span>
      </section>
    )
  }

  if (completeRows.length === 0) {
    return (
      <section className="rounded-sm border border-border/80 bg-surface-1 p-3 text-[11px] text-muted-foreground">
        No whole-exchange TPS data is available for the selected filters.
      </section>
    )
  }

  return (
    <section className="overflow-hidden rounded-sm border border-border/80 bg-surface-1">
      <div className="border-b border-border/80 px-3 py-2">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="font-sans text-sm font-semibold">
              Whole-exchange TPS
            </h2>
            <p className="mt-1 text-[11px] text-muted-foreground">
              Minute buckets for whole-exchange transactions. Aster includes order-action counts where available.
            </p>
          </div>
          <div className="flex flex-wrap gap-2 text-[10px] text-muted-foreground">
            {sources.map((source) => (
              <span
                key={source.venue}
                className="inline-flex h-7 items-center gap-1.5 rounded-sm border border-border bg-surface-1 px-2"
              >
                <VenueName venue={source.venue} />
                <span>/ {source.quality}</span>
              </span>
            ))}
          </div>
        </div>
      </div>

      <div className="grid gap-3 border-b border-border/80 p-3 md:grid-cols-2 xl:grid-cols-4">
        {latestRows.map((row) => (
          <TPSStat key={row.venue} row={row} />
        ))}
      </div>

      <div className="grid gap-3 p-3 xl:grid-cols-[minmax(0,1fr)_360px]">
        <div className="min-h-[300px] min-w-0">
          <div className="mb-2">
            <h3 className="font-sans text-xs font-semibold">TPS over time</h3>
          </div>
          <div className="h-[260px]">
            <ParentSize>
              {({ width, height }) => (
                <TPSChart height={height} series={series} width={width} />
              )}
            </ParentSize>
          </div>
        </div>
        <RecentTPSTable rows={[...completeRows].reverse().slice(0, 10)} />
      </div>
    </section>
  )
}

function TPSStat({ row }: { row: ExchangeTPSRow }) {
  return (
    <div className="rounded-sm border border-border/70 bg-surface-1 p-3">
      <div className="text-[10px] uppercase text-muted-foreground">
        <VenueName venue={row.venue} />
      </div>
      <div className="tabular mt-2 font-sans text-2xl font-semibold tracking-normal">
        {formatTPS(row.tps)}
      </div>
      <div className="mt-1 text-[11px] text-muted-foreground">
        {formatCount(row.tx_count)} tx / {formatTime(row.bucket_start)}
      </div>
      {row.orders_per_second ? (
        <div className="mt-1 text-[11px] text-muted-foreground">
          {formatTPS(row.orders_per_second)} orders/s
        </div>
      ) : null}
    </div>
  )
}

function TPSChart({
  height,
  series,
  width,
}: {
  height: number
  series: Array<TPSSeries>
  width: number
}) {
  if (width <= 0 || height <= 0 || series.length === 0) {
    return null
  }

  const points = series.flatMap((item) => item.points)
  const minDate = minBy(points, (point) => point.date.getTime())?.date
  const maxDate = maxBy(points, (point) => point.date.getTime())?.date
  const maxTPS = maxBy(points, (point) => point.tps)?.tps ?? 1
  if (!minDate || !maxDate) {
    return null
  }

  const innerWidth = Math.max(width - MARGIN.left - MARGIN.right, 1)
  const innerHeight = Math.max(height - MARGIN.top - MARGIN.bottom, 1)
  const xScale = scaleTime({
    domain: minDate.getTime() === maxDate.getTime()
      ? [new Date(minDate.getTime() - 60_000), new Date(maxDate.getTime() + 60_000)]
      : [minDate, maxDate],
    range: [0, innerWidth],
  })
  const yScale = scaleLinear({
    domain: [0, Math.max(maxTPS * 1.08, 1)],
    nice: true,
    range: [innerHeight, 0],
  })

  return (
    <svg width={width} height={height} role="img" aria-label="Whole-exchange TPS over time">
      <Group left={MARGIN.left} top={MARGIN.top}>
        <GridRows
          scale={yScale}
          numTicks={5}
          width={innerWidth}
          stroke="var(--chart-grid)"
          strokeDasharray="3 4"
        />
        <AxisLeft
          scale={yScale}
          numTicks={5}
          tickFormat={(value) => formatTPS(Number(value))}
          tickLabelProps={() => ({
            fill: "var(--chart-text)",
            fontFamily: "JetBrains Mono Variable",
            fontSize: 10,
            textAnchor: "end",
          })}
          stroke="var(--chart-axis)"
          tickStroke="var(--chart-axis)"
        />
        <AxisBottom
          top={innerHeight}
          scale={xScale}
          numTicks={Math.max(2, Math.min(6, Math.floor(innerWidth / 150)))}
          tickFormat={(value) => formatTime(new Date(value.valueOf()))}
          tickLabelProps={() => ({
            fill: "var(--chart-text)",
            fontFamily: "JetBrains Mono Variable",
            fontSize: 10,
            textAnchor: "middle",
          })}
          stroke="var(--chart-axis)"
          tickStroke="var(--chart-axis)"
        />
        {series.map((item) => (
          <LinePath
            key={item.venue}
            data={item.points}
            x={(point) => xScale(point.date)}
            y={(point) => yScale(point.tps)}
            stroke={item.color}
            strokeWidth={1.8}
            pointerEvents="none"
          />
        ))}
      </Group>
    </svg>
  )
}

function RecentTPSTable({ rows }: { rows: Array<ExchangeTPSRow> }) {
  return (
    <div className="min-w-0 overflow-hidden rounded-sm border border-border/70">
      <div className="border-b border-border/70 px-3 py-2">
        <h3 className="font-sans text-xs font-semibold">Recent buckets</h3>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[340px] text-left text-[11px]">
          <thead className="text-[10px] uppercase text-muted-foreground">
            <tr className="border-b border-border/70">
              <th className="px-3 py-2 font-medium">Venue</th>
              <th className="px-3 py-2 font-medium">Time</th>
              <th className="px-3 py-2 text-right font-medium">TPS</th>
              <th className="px-3 py-2 text-right font-medium">Tx</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={`${row.venue}-${row.bucket_start}`} className="border-b border-border/40 last:border-0">
                <td className="px-3 py-2">
                  <VenueName venue={row.venue} />
                </td>
                <td className="px-3 py-2 font-mono text-muted-foreground">
                  {formatTime(row.bucket_start)}
                </td>
                <td className="px-3 py-2 text-right font-mono">
                  {formatTPS(row.tps)}
                </td>
                <td className="px-3 py-2 text-right font-mono text-muted-foreground">
                  {formatCount(row.tx_count)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function buildSeries(rows: Array<ExchangeTPSRow>) {
  const byVenue = new Map<string, Array<TPSPoint>>()
  for (const row of rows) {
    const date = new Date(row.bucket_start)
    if (Number.isNaN(date.getTime()) || !Number.isFinite(row.tps)) {
      continue
    }
    const points = byVenue.get(row.venue) ?? []
    points.push({ date, tps: row.tps })
    byVenue.set(row.venue, points)
  }

  return [...byVenue.entries()]
    .map(([venue, points]) => ({
      color: colorForVenue(venue),
      points: points.sort((left, right) => left.date.getTime() - right.date.getTime()),
      venue,
    }))
    .sort((left, right) => left.venue.localeCompare(right.venue))
}

function latestRowsByVenue(rows: Array<ExchangeTPSRow>) {
  const latest = new Map<string, ExchangeTPSRow>()
  for (const row of rows) {
    const existing = latest.get(row.venue)
    if (!existing || row.bucket_start > existing.bucket_start) {
      latest.set(row.venue, row)
    }
  }
  return [...latest.values()].sort((left, right) => left.venue.localeCompare(right.venue))
}

function formatTPS(value: number | null | undefined) {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return "-"
  }
  return new Intl.NumberFormat("en", {
    maximumFractionDigits: value >= 100 ? 0 : 1,
    minimumFractionDigits: 0,
  }).format(value)
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

function maxBy<T>(items: Array<T>, score: (item: T) => number) {
  let best: T | null = null
  let bestScore = Number.NEGATIVE_INFINITY
  for (const item of items) {
    const itemScore = score(item)
    if (Number.isFinite(itemScore) && itemScore > bestScore) {
      best = item
      bestScore = itemScore
    }
  }
  return best
}
