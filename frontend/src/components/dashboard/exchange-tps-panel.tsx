"use client"

import { AxisBottom, AxisLeft } from "@visx/axis"
import { GridRows } from "@visx/grid"
import { Group } from "@visx/group"
import { ParentSize } from "@visx/responsive"
import { scaleLinear, scaleLog, scaleTime } from "@visx/scale"
import { LinePath } from "@visx/shape"
import { TooltipWithBounds, useTooltip } from "@visx/tooltip"
import {
  type PointerEvent,
  useEffect,
  useMemo,
} from "react"

import type { ExchangeTPSRow } from "@/api/bench"
import {
  colorForVenue,
  type LatencyScaleMode,
} from "@/components/charts/latency-timeseries-chart"
import {
  CSVURLExportButton,
  type CSVWindowOption,
} from "@/components/dashboard/csv-export-button"
import { VenueName } from "@/components/dashboard/venue-logo"
import { formatTime } from "@/lib/format"

const MARGIN = { top: 16, right: 18, bottom: 32, left: 70 }
const TPS_POINTS_PER_PIXEL = 2

interface TPSPoint {
  date: Date
  tps: number
  txCount: number
  venue: string
}

interface TPSSeries {
  color: string
  points: Array<TPSPoint>
  venue: string
}

interface TPSAverage {
  avgTPS: number
  bucketCount: number
  color: string
  totalSeconds: number
  totalTx: number
  venue: string
}

interface TPSTooltipData {
  date: Date
  points: Array<TPSPoint & { color: string }>
}

export function ExchangeTPSPanel({
  defaultExportWindow,
  exportHrefForWindow,
  exportWindowOptions,
  isLoading = false,
  onScaleModeChange,
  onVenueSelectionChange,
  rows,
  scaleMode,
  selectedVenues,
  showExports = true,
  venues,
}: {
  defaultExportWindow?: string
  exportHrefForWindow?: (window: string) => string
  exportWindowOptions?: ReadonlyArray<CSVWindowOption>
  isLoading?: boolean
  onScaleModeChange: (mode: LatencyScaleMode) => void
  onVenueSelectionChange: (venues: "all" | Array<string>) => void
  rows: Array<ExchangeTPSRow>
  scaleMode: LatencyScaleMode
  selectedVenues: Array<string>
  showExports?: boolean
  venues: Array<string>
}) {
  const completeRows = useMemo(
    () => rows.filter((row) => row.complete && Number.isFinite(row.tps)),
    [rows]
  )
  const averages = useMemo(
    () => averageRowsByVenue(completeRows),
    [completeRows]
  )
  const series = useMemo(() => buildSeries(completeRows), [completeRows])

  if (isLoading) {
    return (
      <section
        className="flex min-h-40 flex-col items-center justify-center gap-3 rounded-sm border border-border/80 bg-surface-1 p-3 text-[11px] text-muted-foreground"
        aria-label="Loading transaction TPS data"
        role="status"
      >
        <div className="size-5 animate-spin rounded-full border-2 border-border border-t-primary" />
        <span>Loading TPS data</span>
      </section>
    )
  }

  if (completeRows.length === 0) {
    return (
      <section className="rounded-sm border border-border/80 bg-surface-1 p-3 text-[11px] text-muted-foreground">
        No TPS data is available for the selected filters.
      </section>
    )
  }

  return (
    <section className="overflow-hidden rounded-sm border border-border/80 bg-surface-1">
      <div className="border-b border-border/80 px-3 py-2">
        <div className="flex flex-wrap items-start gap-3">
          <div className="min-w-[min(100%,32rem)] flex-1">
            <h2 className="font-sans text-sm font-semibold">
              Transactions per second
            </h2>
            <p className="mt-1 text-[11px] text-muted-foreground">
              {bucketDescription(completeRows)}
            </p>
          </div>
        </div>
        <ExchangeSelector
          selectedVenues={selectedVenues}
          venues={venues}
          onChange={onVenueSelectionChange}
        />
      </div>

      <div className="grid gap-2 border-b border-border/80 p-2 sm:grid-cols-2 xl:grid-cols-4">
        {averages.map((average) => (
          <TPSStat average={average} key={average.venue} />
        ))}
      </div>

      <div className="p-3">
        <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
          <h3 className="font-sans text-xs font-semibold">TPS over time</h3>
          <div className="flex flex-wrap items-center gap-2">
            {showExports && exportHrefForWindow && exportWindowOptions ? (
              <CSVURLExportButton
                defaultWindow={defaultExportWindow}
                filenameForWindow={(window) =>
                  `transactions-per-second-${window}.csv`
                }
                hrefForWindow={exportHrefForWindow}
                windowOptions={exportWindowOptions}
              />
            ) : null}
            <ScaleToggle value={scaleMode} onChange={onScaleModeChange} />
          </div>
        </div>
        <div className="h-[300px]">
          <ParentSize>
            {({ width, height }) => (
              <TPSChart
                height={height}
                scaleMode={scaleMode}
                series={series}
                width={width}
              />
            )}
          </ParentSize>
        </div>
      </div>
    </section>
  )
}

function TPSStat({ average }: { average: TPSAverage }) {
  return (
    <div className="flex min-h-12 items-center justify-between gap-3 rounded-sm border border-border/70 bg-surface-1 px-2.5 py-2">
      <div className="flex min-w-0 items-center gap-2 text-[10px] uppercase text-muted-foreground">
        <span
          className="size-1.5 shrink-0 rounded-full"
          style={{ backgroundColor: average.color }}
        />
        <VenueName venue={average.venue} />
      </div>
      <div className="tabular shrink-0 font-sans text-xl font-semibold tracking-normal">
        {formatTPS(average.avgTPS)}
      </div>
    </div>
  )
}

function TPSChart({
  height,
  scaleMode,
  series,
  width,
}: {
  height: number
  scaleMode: LatencyScaleMode
  series: Array<TPSSeries>
  width: number
}) {
  const {
    hideTooltip,
    showTooltip,
    tooltipData,
    tooltipLeft = 0,
    tooltipOpen,
    tooltipTop = 0,
  } = useTooltip<TPSTooltipData>()

  useEffect(() => {
    hideTooltip()
  }, [hideTooltip, series, width, height])
  const innerWidth = Math.max(width - MARGIN.left - MARGIN.right, 1)
  const plotSeries = useMemo(
    () => downsampleTPSSeries(series, innerWidth),
    [innerWidth, series]
  )
  const points = useMemo(
    () => series.flatMap((item) => item.points),
    [series]
  )
  const tooltipDates = useMemo(() => uniqueSortedDates(points), [points])

  if (width <= 0 || height <= 0 || series.length === 0) {
    return null
  }

  const minDate = minBy(points, (point) => point.date.getTime())?.date
  const maxDate = maxBy(points, (point) => point.date.getTime())?.date
  const maxTPS = maxBy(points, (point) => point.tps)?.tps ?? 1
  const minPositiveTPS = minBy(
    points.filter((point) => point.tps > 0),
    (point) => point.tps
  )?.tps ?? 1
  if (!minDate || !maxDate) {
    return null
  }

  const innerHeight = Math.max(height - MARGIN.top - MARGIN.bottom, 1)
  const xScale = scaleTime({
    domain: minDate.getTime() === maxDate.getTime()
      ? [new Date(minDate.getTime() - 60_000), new Date(maxDate.getTime() + 60_000)]
      : [minDate, maxDate],
    range: [0, innerWidth],
  })
  const yDomainMax = Math.max(maxTPS * 1.08, 1)
  const yDomainMin = Math.max(minPositiveTPS * 0.8, 0.1)
  const yScale =
    scaleMode === "log"
      ? scaleLog({
          domain: [Math.min(yDomainMin, yDomainMax / 10), yDomainMax],
          range: [innerHeight, 0],
        })
      : scaleLinear({
          domain: [0, yDomainMax],
          nice: true,
          range: [innerHeight, 0],
        })
  const yTickValues =
    scaleMode === "log"
      ? stableLogTicks(
          [Math.min(yDomainMin, yDomainMax / 10), yDomainMax],
          (value) => yScale(value),
          innerHeight
        )
      : undefined
  const showTPSAtPointer = (event: PointerEvent<SVGRectElement>) => {
    const bounds = event.currentTarget.getBoundingClientRect()
    const plotX = event.clientX - bounds.left
    const date = nearestDateAtX(tooltipDates, xScale.invert(plotX).getTime())
    if (!date) {
      hideTooltip()
      return
    }
    const tooltipPoints = series
      .map((item) => {
        const point = findPointAtTime(item.points, date.getTime())
        return point ? { ...point, color: item.color } : null
      })
      .filter((point): point is TPSPoint & { color: string } => point !== null)
      .sort((left, right) => right.tps - left.tps)

    if (tooltipPoints.length === 0) {
      hideTooltip()
      return
    }
    showTooltip({
      tooltipData: {
        date,
        points: tooltipPoints,
      },
      tooltipLeft: MARGIN.left + xScale(date) + 12,
      tooltipTop: MARGIN.top + 10,
    })
  }
  const hoverX = tooltipData ? xScale(tooltipData.date) : null

  return (
    <div
      className="relative h-full w-full"
      onPointerLeave={hideTooltip}
      onMouseLeave={hideTooltip}
      onBlur={hideTooltip}
    >
      <svg
        width={width}
        height={height}
        role="img"
        aria-label="TPS over time"
      >
        <Group left={MARGIN.left} top={MARGIN.top}>
          <GridRows
            scale={yScale}
            tickValues={yTickValues}
            numTicks={5}
            width={innerWidth}
            stroke="var(--chart-grid)"
            strokeDasharray="3 4"
          />
          <AxisLeft
            scale={yScale}
            tickValues={yTickValues}
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
          {plotSeries.map((item) => (
            <LinePath
              key={item.venue}
              data={item.points}
              x={(point) => xScale(point.date)}
              y={(point) => yScale(scaleTPS(point.tps, scaleMode, yDomainMin))}
              stroke={item.color}
              strokeWidth={1.8}
              pointerEvents="none"
            />
          ))}
          <rect
            width={innerWidth}
            height={innerHeight}
            fill="transparent"
            onPointerEnter={showTPSAtPointer}
            onPointerMove={showTPSAtPointer}
            onPointerLeave={hideTooltip}
            onPointerCancel={hideTooltip}
          />
          {tooltipOpen && hoverX !== null ? (
            <>
              <line
                x1={hoverX}
                x2={hoverX}
                y1={0}
                y2={innerHeight}
                stroke="var(--chart-axis)"
                strokeDasharray="3 4"
                pointerEvents="none"
              />
              {tooltipData?.points.map((point) => (
                <circle
                  key={point.venue}
                  cx={hoverX}
                  cy={yScale(scaleTPS(point.tps, scaleMode, yDomainMin))}
                  r={3.5}
                  fill={point.color}
                  stroke="var(--chart-point-stroke)"
                  strokeWidth={1.2}
                  pointerEvents="none"
                />
              ))}
            </>
          ) : null}
        </Group>
      </svg>
      {tooltipOpen && tooltipData ? (
        <TooltipWithBounds
          left={tooltipLeft}
          top={tooltipTop}
          className="pointer-events-none z-10 w-[230px] max-w-[calc(100vw-2rem)] rounded-sm border border-border/80 bg-surface-1 px-2.5 py-2 text-[10px] shadow-sm"
        >
          <TPSTooltip data={tooltipData} />
        </TooltipWithBounds>
      ) : null}
    </div>
  )
}

function ScaleToggle({
  value,
  onChange,
}: {
  value: LatencyScaleMode
  onChange: (value: LatencyScaleMode) => void
}) {
  return (
    <div className="flex h-8 overflow-hidden rounded-sm border border-border bg-surface-1 text-[11px]">
      {(["linear", "log"] as const).map((mode) => (
        <button
          key={mode}
          type="button"
          onClick={() => onChange(mode)}
          className={`px-2.5 capitalize ${
            value === mode
              ? "bg-primary/15 text-foreground ring-1 ring-inset ring-primary/40"
              : "text-muted-foreground hover:bg-surface-2 hover:text-foreground"
          }`}
          aria-pressed={value === mode}
        >
          {mode}
        </button>
      ))}
    </div>
  )
}

function ExchangeSelector({
  selectedVenues,
  venues,
  onChange,
}: {
  selectedVenues: Array<string>
  venues: Array<string>
  onChange: (venues: "all" | Array<string>) => void
}) {
  const selected = new Set(selectedVenues)
  const allSelected =
    venues.length > 0 && selectedVenues.length === venues.length

  if (venues.length === 0) {
    return null
  }

  return (
    <div className="mt-2 flex flex-wrap items-center gap-2">
      <span className="text-[10px] uppercase tracking-[0.08em] text-muted-foreground">
        Exchanges
      </span>
      <button
        type="button"
        onClick={() => onChange(allSelected ? [] : "all")}
        className="h-7 rounded-sm border border-border bg-surface-1 px-2 text-[10px] text-muted-foreground hover:bg-surface-2 hover:text-foreground"
      >
        {allSelected ? "Clear" : "All"}
      </button>
      {venues.map((venue) => {
        const checked = selected.has(venue)
        return (
          <label
            key={venue}
            className={`flex h-7 cursor-pointer items-center gap-2 rounded-sm border px-2 text-[10px] ${
              checked
                ? "border-border bg-surface-2 text-foreground"
                : "border-border/70 bg-surface-1 text-muted-foreground"
            }`}
          >
            <input
              type="checkbox"
              checked={checked}
              onChange={() => {
                const next = checked
                  ? selectedVenues.filter((item) => item !== venue)
                  : [...selectedVenues, venue]
                onChange(next.length === venues.length ? "all" : next)
              }}
              className="size-3 accent-[var(--primary)]"
            />
            <span
              className="size-1.5 rounded-full"
              style={{ backgroundColor: colorForVenue(venue) }}
            />
            <VenueName venue={venue} />
          </label>
        )
      })}
    </div>
  )
}

function scaleTPS(
  value: number,
  scaleMode: LatencyScaleMode,
  minPositiveTPS: number
) {
  if (scaleMode === "log") {
    return Math.max(value, minPositiveTPS)
  }
  return value
}

function TPSTooltip({ data }: { data: TPSTooltipData }) {
  return (
    <>
      <div className="font-mono text-[10px] text-muted-foreground">
        {formatTime(data.date)}
      </div>
      <div className="mt-2 grid gap-1.5">
        {data.points.map((point) => (
          <div
            key={point.venue}
            className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3"
          >
            <div className="flex min-w-0 items-center gap-2 text-muted-foreground">
              <span
                className="size-1.5 shrink-0 rounded-full"
                style={{ backgroundColor: point.color }}
              />
              <VenueName venue={point.venue} />
            </div>
            <div className="font-mono font-bold text-foreground">
              {formatTPS(point.tps)}
            </div>
          </div>
        ))}
      </div>
    </>
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
    points.push({ date, tps: row.tps, txCount: row.tx_count, venue: row.venue })
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

function downsampleTPSSeries(series: Array<TPSSeries>, width: number) {
  const maxPoints = Math.max(240, Math.floor(width * TPS_POINTS_PER_PIXEL))
  return series.map((item) => ({
    ...item,
    points: downsampleTPSPoints(item.points, maxPoints),
  }))
}

function downsampleTPSPoints(points: Array<TPSPoint>, maxPoints: number) {
  if (points.length <= maxPoints || maxPoints < 4) {
    return points
  }
  const firstTime = points[0].date.getTime()
  const lastTime = points[points.length - 1].date.getTime()
  const span = Math.max(lastTime - firstTime, 1)
  const bucketCount = Math.max(2, Math.floor(maxPoints / 2))
  const buckets = new Map<number, { high: TPSPoint; low: TPSPoint }>()

  for (const point of points) {
    const bucket = Math.min(
      bucketCount - 1,
      Math.floor(((point.date.getTime() - firstTime) / span) * bucketCount)
    )
    const current = buckets.get(bucket)
    if (!current) {
      buckets.set(bucket, { high: point, low: point })
      continue
    }
    if (point.tps < current.low.tps) {
      current.low = point
    }
    if (point.tps > current.high.tps) {
      current.high = point
    }
  }

  const selected = new Map<number, TPSPoint>()
  for (const point of [points[0], points[points.length - 1]]) {
    selected.set(point.date.getTime(), point)
  }
  for (const bucket of buckets.values()) {
    selected.set(bucket.low.date.getTime(), bucket.low)
    selected.set(bucket.high.date.getTime(), bucket.high)
  }
  return [...selected.values()].sort(
    (left, right) => left.date.getTime() - right.date.getTime()
  )
}

function averageRowsByVenue(rows: Array<ExchangeTPSRow>) {
  const byVenue = new Map<string, TPSAverage>()
  for (const row of rows) {
    const date = new Date(row.bucket_start)
    if (
      Number.isNaN(date.getTime()) ||
      !Number.isFinite(row.tps) ||
      !Number.isFinite(row.bucket_seconds) ||
      row.bucket_seconds <= 0
    ) {
      continue
    }
    const current = byVenue.get(row.venue) ?? {
      avgTPS: 0,
      bucketCount: 0,
      color: colorForVenue(row.venue),
      totalSeconds: 0,
      totalTx: 0,
      venue: row.venue,
    }
    current.bucketCount++
    current.totalSeconds += row.bucket_seconds
    current.totalTx += row.tx_count
    current.avgTPS = current.totalTx / current.totalSeconds
    byVenue.set(row.venue, current)
  }
  return [...byVenue.values()].sort((left, right) => left.venue.localeCompare(right.venue))
}

function bucketDescription(rows: Array<ExchangeTPSRow>) {
  const bucketSeconds = rows[0]?.bucket_seconds
  if (bucketSeconds === 3600) {
    return "Hourly transaction buckets by exchange."
  }
  return "Minute transaction buckets by exchange."
}

function uniqueSortedDates(points: Array<TPSPoint>) {
  return [...new Set(points.map((point) => point.date.getTime()))].sort(
    (left, right) => left - right
  )
}

function nearestDateAtX(times: Array<number>, targetMs: number) {
  if (times.length === 0) {
    return null
  }
  let low = 0
  let high = times.length - 1
  while (low < high) {
    const mid = Math.floor((low + high) / 2)
    if (times[mid] < targetMs) {
      low = mid + 1
    } else {
      high = mid
    }
  }
  const previous = Math.max(0, low - 1)
  const best =
    Math.abs(times[previous] - targetMs) <= Math.abs(times[low] - targetMs)
      ? times[previous]
      : times[low]
  return new Date(best)
}

function findPointAtTime(points: Array<TPSPoint>, targetMs: number) {
  let low = 0
  let high = points.length - 1
  while (low <= high) {
    const mid = Math.floor((low + high) / 2)
    const value = points[mid].date.getTime()
    if (value === targetMs) {
      return points[mid]
    }
    if (value < targetMs) {
      low = mid + 1
    } else {
      high = mid - 1
    }
  }
  return null
}

function stableLogTicks(
  domain: [number, number],
  position: (value: number) => number,
  height: number
) {
  const [min, max] = domain
  if (min <= 0 || max <= 0 || min >= max) {
    return []
  }

  const minExponent = Math.floor(Math.log10(min))
  const maxExponent = Math.ceil(Math.log10(max))
  const candidates: Array<number> = []

  for (let exponent = minExponent; exponent <= maxExponent; exponent += 1) {
    const decade = 10 ** exponent
    for (const multiplier of [1, 2, 5]) {
      const value = multiplier * decade
      if (value >= min && value <= max) {
        candidates.push(value)
      }
    }
  }

  if (candidates.length <= 1) {
    return candidates
  }

  const minGap = height < 280 ? 34 : 42
  const selected: Array<number> = []

  for (const value of candidates.sort((left, right) => right - left)) {
    const y = position(value)
    const hasRoom = selected.every(
      (existing) => Math.abs(position(existing) - y) >= minGap
    )
    if (hasRoom) {
      selected.push(value)
    }
  }

  return selected.sort((left, right) => left - right)
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
