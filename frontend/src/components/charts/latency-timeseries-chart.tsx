"use client"

import { AxisBottom, AxisLeft } from "@visx/axis"
import { localPoint } from "@visx/event"
import { GridRows } from "@visx/grid"
import { Group } from "@visx/group"
import { ParentSize } from "@visx/responsive"
import { scaleLinear, scaleLog, scaleTime } from "@visx/scale"
import { LinePath } from "@visx/shape"
import { TooltipWithBounds, useTooltip } from "@visx/tooltip"
import { type PointerEvent, useMemo } from "react"

import type { Sample } from "@/api/bench"
import { samplePlotDate } from "@/lib/sample-time"
import { formatLatency, formatTime } from "@/lib/format"
import { confirmSampleMs } from "@/lib/latency-metric"

interface Series {
  color: string
  key: string
  label: string
  measurementMode: string
  points: Array<Point>
  scenario: string
  strokeDasharray?: string
  transport: string
  venue: string
}

interface Point {
  date: Date
  ms: number
}

interface HoverPoint {
  color: string
  point: Point
  series: Series
}

export type LatencyScaleMode = "linear" | "log"

const COLORS = [
  "oklch(0.52 0.17 215)",
  "oklch(0.5 0.14 160)",
  "oklch(0.6 0.16 40)",
  "oklch(0.55 0.16 285)",
  "oklch(0.58 0.17 18)",
  "oklch(0.45 0.08 250)",
]

const MARGIN = { top: 18, right: 20, bottom: 34, left: 62 }

export function LatencyTimeseriesChart({
  samples,
  scaleMode,
  selectedVenues,
  venues,
  onScaleModeChange,
  onVenueSelectionChange,
  title = "Latency Timeline",
  description = "Confirmation latency over time by venue, transport, and order type.",
}: {
  samples: Array<Sample>
  scaleMode: LatencyScaleMode
  selectedVenues: Array<string>
  venues: Array<string>
  onScaleModeChange: (mode: LatencyScaleMode) => void
  onVenueSelectionChange: (venues: "all" | Array<string>) => void
  title?: string
  description?: string
}) {
  const series = useMemo(() => buildSeries(samples), [samples])
  const domain = useMemo(
    () => getDomains(series, scaleMode),
    [scaleMode, series]
  )

  return (
    <section className="rounded-sm border border-border/80 bg-surface-1">
      <div className="border-b border-border/80 px-3 py-2">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="font-sans text-sm font-semibold">{title}</h2>
            <p className="mt-1 text-[11px] text-muted-foreground">
              {description}
            </p>
          </div>
          <ScaleToggle value={scaleMode} onChange={onScaleModeChange} />
        </div>
        <ExchangeSelector
          selectedVenues={selectedVenues}
          venues={venues}
          onChange={onVenueSelectionChange}
        />
        <SeriesLegend series={series} />
      </div>
      <div className="h-[360px] px-2 py-3">
        {series.length === 0 || !domain ? (
          <div className="flex h-full items-center px-2 text-[11px] text-muted-foreground">
            No latency data is available for the selected filters.
          </div>
        ) : (
          <ParentSize>
            {({ width, height }) => (
              <LatencyFrame
                domain={domain}
                height={height}
                scaleMode={scaleMode}
                series={series}
                width={width}
              />
            )}
          </ParentSize>
        )}
      </div>
    </section>
  )
}

function LatencyFrame({
  domain,
  height,
  scaleMode,
  series,
  width,
}: {
  domain: NonNullable<ReturnType<typeof getDomains>>
  height: number
  scaleMode: LatencyScaleMode
  series: Array<Series>
  width: number
}) {
  const {
    hideTooltip,
    showTooltip,
    tooltipData,
    tooltipLeft = 0,
    tooltipOpen,
    tooltipTop = 0,
  } = useTooltip<HoverPoint>()

  return (
    <div className="relative h-full w-full">
      <LatencySvg
        domain={domain}
        height={height}
        hideTooltip={hideTooltip}
        scaleMode={scaleMode}
        series={series}
        showTooltip={showTooltip}
        width={width}
      />
      {tooltipOpen && tooltipData ? (
        <TooltipWithBounds
          left={tooltipLeft}
          top={tooltipTop}
          className="pointer-events-none z-10 w-[180px] max-w-[calc(100vw-2rem)] rounded-sm border border-border/80 bg-surface-1 px-2.5 py-2 text-[10px] shadow-sm"
        >
          <PointTooltip hover={tooltipData} />
        </TooltipWithBounds>
      ) : null}
    </div>
  )
}

function LatencySvg({
  domain,
  hideTooltip,
  height,
  scaleMode,
  series,
  showTooltip,
  width,
}: {
  domain: NonNullable<ReturnType<typeof getDomains>>
  hideTooltip: () => void
  height: number
  scaleMode: LatencyScaleMode
  series: Array<Series>
  showTooltip: (args: {
    tooltipData: HoverPoint
    tooltipLeft?: number
    tooltipTop?: number
  }) => void
  width: number
}) {
  if (width <= 0 || height <= 0) {
    return null
  }

  const innerWidth = Math.max(width - MARGIN.left - MARGIN.right, 1)
  const innerHeight = Math.max(height - MARGIN.top - MARGIN.bottom, 1)
  const xTickCount = Math.max(2, Math.min(6, Math.floor(innerWidth / 180)))
  const xScale = scaleTime({
    domain: domain.x,
    range: [0, innerWidth],
  })
  const yScale =
    scaleMode === "log"
      ? scaleLog({
          domain: domain.y,
          range: [innerHeight, 0],
        })
      : scaleLinear({
          domain: domain.y,
          nice: true,
          range: [innerHeight, 0],
        })
  const yTickValues =
    scaleMode === "log"
      ? stableLogTicks(domain.y, (value) => yScale(value), innerHeight)
      : undefined
  const showPointTooltip = (
    event: PointerEvent<SVGElement>,
    item: Series,
    point: Point
  ) => {
    const coords = localPoint(event) ?? {
      x: MARGIN.left + xScale(point.date),
      y: MARGIN.top + yScale(point.ms),
    }
    showTooltip({
      tooltipData: {
        color: item.color,
        point,
        series: item,
      },
      tooltipLeft: coords.x + 10,
      tooltipTop: coords.y - 62,
    })
  }

  return (
    <svg
      width={width}
      height={height}
      role="img"
      aria-label="Latency timeline"
      onPointerLeave={hideTooltip}
      onPointerCancel={hideTooltip}
      onBlur={hideTooltip}
    >
      <Group left={MARGIN.left} top={MARGIN.top}>
        <GridRows
          scale={yScale}
          tickValues={yTickValues}
          numTicks={5}
          width={innerWidth}
          stroke="oklch(0.88 0.004 255 / 0.7)"
          strokeDasharray="3 4"
        />
        <AxisLeft
          scale={yScale}
          tickValues={yTickValues}
          numTicks={5}
          tickFormat={(value) => formatLatency(Number(value))}
          tickLabelProps={() => ({
            fill: "oklch(0.48 0.015 253)",
            fontFamily: "JetBrains Mono Variable",
            fontSize: 10,
            textAnchor: "end",
          })}
          stroke="oklch(0.9 0.004 255)"
          tickStroke="oklch(0.9 0.004 255)"
        />
        <AxisBottom
          top={innerHeight}
          scale={xScale}
          numTicks={xTickCount}
          tickFormat={(value) => formatTime(new Date(value.valueOf()))}
          tickLabelProps={() => ({
            fill: "oklch(0.48 0.015 253)",
            fontFamily: "JetBrains Mono Variable",
            fontSize: 10,
            textAnchor: "middle",
          })}
          stroke="oklch(0.9 0.004 255)"
          tickStroke="oklch(0.9 0.004 255)"
        />
        {series.map((item) => (
          <Group key={item.key}>
            <LinePath
              data={item.points}
              x={(point) => xScale(point.date)}
              y={(point) => yScale(point.ms)}
              stroke={item.color}
              strokeDasharray={item.strokeDasharray}
              strokeWidth={1.7}
            />
            {item.points.map((point) => (
              <Group
                key={`${point.date.toISOString()}:${point.ms}`}
                onPointerEnter={(event) => showPointTooltip(event, item, point)}
                onPointerMove={(event) => showPointTooltip(event, item, point)}
                onPointerLeave={hideTooltip}
              >
                <circle
                  cx={xScale(point.date)}
                  cy={yScale(point.ms)}
                  r={7}
                  fill="transparent"
                />
                <circle
                  cx={xScale(point.date)}
                  cy={yScale(point.ms)}
                  r={2.4}
                  fill={item.color}
                />
              </Group>
            ))}
          </Group>
        ))}
      </Group>
    </svg>
  )
}

function PointTooltip({
  hover,
}: {
  hover: HoverPoint
}) {
  return (
    <>
      <div className="flex items-center gap-2 text-muted-foreground">
        <span
          className="h-1.5 w-1.5 rounded-full"
          style={{ backgroundColor: hover.color }}
        />
        <span className="min-w-0 break-words">{hover.series.label}</span>
      </div>
      <div className="mt-1 font-mono text-[10px] text-muted-foreground">
        {formatTime(hover.point.date)}
      </div>
      <div className="mt-2 grid grid-cols-[1fr_auto] gap-x-3 gap-y-1">
        <span className="-mx-1 rounded-sm bg-surface-2 px-1 py-0.5 font-bold text-foreground">
          Latency
        </span>
        <span className="-mx-1 rounded-sm bg-surface-2 px-1 py-0.5 font-mono font-bold text-foreground">
          {formatLatency(hover.point.ms)}
        </span>
      </div>
    </>
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
              ? "bg-foreground text-background"
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
            <span>{venue}</span>
          </label>
        )
      })}
    </div>
  )
}

function SeriesLegend({ series }: { series: Array<Series> }) {
  if (series.length === 0) {
    return null
  }

  return (
    <div className="mt-2 flex max-h-16 flex-wrap gap-x-3 gap-y-1 overflow-y-auto pr-1">
      {series.map((item) => (
        <div
          key={item.key}
          className="flex min-w-0 items-center gap-2 text-[10px] text-muted-foreground"
        >
          <svg width="20" height="6" aria-hidden>
            <line
              x1="0"
              y1="3"
              x2="20"
              y2="3"
              stroke={item.color}
              strokeDasharray={item.strokeDasharray}
              strokeWidth="2"
            />
          </svg>
          <span className="max-w-[240px] truncate">{item.label}</span>
          <span className="font-mono text-[9px] text-muted-foreground/80">
            {item.points.length} pts
          </span>
        </div>
      ))}
    </div>
  )
}

function buildSeries(samples: Array<Sample>): Array<Series> {
  const grouped = new Map<string, Array<Point>>()
  const metadata = new Map<
    string,
    {
      measurementMode: string
      orderType: string
      scenario: string
      transport: string
      venue: string
    }
  >()

  for (const sample of samples) {
    if (!sample.ok || sample.warmup || sample.network_ns <= 0) {
      continue
    }

    const date = samplePlotDate(sample)
    if (!date) {
      continue
    }

    const ms = confirmSampleMs(sample)
    if (!Number.isFinite(ms) || ms <= 0) {
      continue
    }

    const orderType = sample.order_type || "unknown"
    const measurementMode = sample.measurement_mode || "ack"
    const key = [
      sample.venue,
      sample.transport,
      sample.scenario,
      orderType,
      measurementMode,
    ].join(":")
    const points = grouped.get(key) ?? []
    points.push({
      date,
      ms,
    })
    grouped.set(key, points)
    metadata.set(key, {
      measurementMode,
      orderType,
      scenario: sample.scenario,
      transport: sample.transport,
      venue: sample.venue,
    })
  }

  return [...grouped.entries()]
    .map(([key, points]) => {
      const meta = metadata.get(key)
      const venue = meta?.venue ?? key.split(":")[0] ?? "unknown"
      const transport = meta?.transport ?? "unknown"

      return {
        color: colorForVenue(venue),
        key,
        label: formatSeriesLabel(meta, key),
        measurementMode: meta?.measurementMode ?? "ack",
        points: points.sort((a, b) => a.date.getTime() - b.date.getTime()),
        scenario: meta?.scenario ?? "unknown",
        strokeDasharray: strokeForTransport(transport),
        transport,
        venue,
      }
    })
    .filter((item) => item.points.length > 0)
    .sort((left, right) => left.label.localeCompare(right.label))
}

function getDomains(series: Array<Series>, scaleMode: LatencyScaleMode) {
  const points = series.flatMap((item) => item.points)

  if (points.length === 0) {
    return null
  }

  const minTime = Math.min(...points.map((point) => point.date.getTime()))
  const maxTime = Math.max(...points.map((point) => point.date.getTime()))
  const minLatency = Math.min(...points.map((point) => point.ms))
  const maxLatency = Math.max(...points.map((point) => point.ms))
  const timePadding = minTime === maxTime ? 60_000 : 0
  const latencyPadding = Math.max(maxLatency * 0.12, 1)
  const minLogLatency = Math.max(minLatency * 0.72, 0.1)

  return {
    x: [
      new Date(minTime - timePadding),
      new Date(maxTime + timePadding),
    ] as [Date, Date],
    y:
      scaleMode === "log"
        ? ([minLogLatency, maxLatency + latencyPadding] as [number, number])
        : ([0, maxLatency + latencyPadding] as [number, number]),
  }
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
    const hasRoom = selected.every((existing) => Math.abs(position(existing) - y) >= minGap)
    if (hasRoom) {
      selected.push(value)
    }
  }

  return selected.sort((left, right) => left - right)
}

function formatSeriesLabel(
  meta:
    | {
        measurementMode: string
        orderType: string
        scenario: string
        transport: string
        venue: string
      }
    | undefined,
  fallback: string
) {
  if (!meta) {
    return fallback.split(":")[0] ?? fallback
  }

  return formatVenue(meta.venue)
}

function formatVenue(value: string) {
  return value
    .split(/[_-]/)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ")
}

function strokeForTransport(transport: string) {
  const normalized = transport.toLowerCase()
  if (normalized.includes("ws") || normalized.includes("websocket")) {
    return "5 4"
  }
  return undefined
}

export function colorForVenue(venue: string) {
  const known = KNOWN_VENUE_COLORS[venue.toLowerCase()]
  if (known) {
    return known
  }

  let hash = 0
  for (let index = 0; index < venue.length; index += 1) {
    hash = (hash * 31 + venue.charCodeAt(index)) >>> 0
  }
  return COLORS[hash % COLORS.length]
}

const KNOWN_VENUE_COLORS: Record<string, string> = {
  aster: "oklch(0.68 0.17 65)",
  edgex: "oklch(0.6 0.16 40)",
  extended: "oklch(0.58 0.17 18)",
  grvt: "oklch(0.45 0.08 250)",
  hyperliquid: "oklch(0.54 0.17 150)",
  lighter: "oklch(0.55 0.17 245)",
  lighter_free: "oklch(0.66 0.16 220)",
  variational_omni: "oklch(0.62 0.15 75)",
}
