"use client"

import { AxisBottom, AxisLeft } from "@visx/axis"
import { GridRows } from "@visx/grid"
import { Group } from "@visx/group"
import { ParentSize } from "@visx/responsive"
import { scaleLinear, scaleTime } from "@visx/scale"
import { LinePath } from "@visx/shape"
import { useMemo, useState } from "react"

import type { Sample } from "@/api/bench"
import { formatLatency, formatTime } from "@/lib/format"
import {
  confirmSampleMs,
  secondaryLabel,
  secondarySampleMs,
} from "@/lib/latency-metric"

interface Series {
  color: string
  key: string
  label: string
  points: Array<Point>
}

interface Point {
  date: Date
  label: string
  ms: number
  secondaryLabel: string
  secondaryMs?: number
}

interface HoverPoint {
  color: string
  point: Point
  x: number
  y: number
}

const COLORS = [
  "oklch(0.52 0.17 215)",
  "oklch(0.5 0.14 160)",
  "oklch(0.6 0.16 40)",
  "oklch(0.55 0.16 285)",
  "oklch(0.58 0.17 18)",
  "oklch(0.45 0.08 250)",
]

const MARGIN = { top: 18, right: 20, bottom: 34, left: 62 }

export function LatencyTimeseriesChart({ samples }: { samples: Array<Sample> }) {
  const series = useMemo(() => buildSeries(samples), [samples])
  const domain = useMemo(() => getDomains(series), [series])

  return (
    <section className="rounded-sm border border-border/80 bg-surface-1">
      <div className="flex flex-wrap items-start justify-between gap-3 border-b border-border/80 px-3 py-2">
        <div>
          <h2 className="font-sans text-sm font-semibold">Latency Timeline</h2>
          <p className="mt-1 text-[11px] text-muted-foreground">
            Confirm latency over time by venue, transport, and order type.
          </p>
        </div>
        <div className="flex flex-wrap gap-3">
          {series.slice(0, 6).map((item) => (
            <div
              key={item.key}
              className="flex items-center gap-2 text-[10px] text-muted-foreground"
            >
              <span
                className="h-px w-5"
                style={{ backgroundColor: item.color }}
              />
              <span>{item.label}</span>
            </div>
          ))}
        </div>
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
  series,
  width,
}: {
  domain: NonNullable<ReturnType<typeof getDomains>>
  height: number
  series: Array<Series>
  width: number
}) {
  const [hover, setHover] = useState<HoverPoint | null>(null)

  return (
    <div className="relative h-full w-full">
      <LatencySvg
        domain={domain}
        height={height}
        onHover={setHover}
        series={series}
        width={width}
      />
      {hover ? <PointTooltip hover={hover} width={width} /> : null}
    </div>
  )
}

function LatencySvg({
  domain,
  height,
  onHover,
  series,
  width,
}: {
  domain: NonNullable<ReturnType<typeof getDomains>>
  height: number
  onHover: (hover: HoverPoint | null) => void
  series: Array<Series>
  width: number
}) {
  if (width <= 0 || height <= 0) {
    return null
  }

  const innerWidth = Math.max(width - MARGIN.left - MARGIN.right, 1)
  const innerHeight = Math.max(height - MARGIN.top - MARGIN.bottom, 1)
  const xScale = scaleTime({
    domain: domain.x,
    range: [0, innerWidth],
  })
  const yScale = scaleLinear({
    domain: domain.y,
    nice: true,
    range: [innerHeight, 0],
  })

  return (
    <svg width={width} height={height} role="img" aria-label="Latency timeline">
      <Group left={MARGIN.left} top={MARGIN.top}>
        <GridRows
          scale={yScale}
          width={innerWidth}
          stroke="oklch(0.88 0.004 255 / 0.7)"
          strokeDasharray="3 4"
        />
        <AxisLeft
          scale={yScale}
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
          numTicks={6}
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
              strokeWidth={1.7}
            />
            {item.points.map((point) => (
              <Group
                key={`${point.date.toISOString()}:${point.ms}`}
                onPointerEnter={() =>
                  onHover({
                    color: item.color,
                    point,
                    x: MARGIN.left + xScale(point.date),
                    y: MARGIN.top + yScale(point.ms),
                  })
                }
                onPointerMove={() =>
                  onHover({
                    color: item.color,
                    point,
                    x: MARGIN.left + xScale(point.date),
                    y: MARGIN.top + yScale(point.ms),
                  })
                }
                onPointerLeave={() => onHover(null)}
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
  width,
}: {
  hover: HoverPoint
  width: number
}) {
  const left = Math.min(Math.max(hover.x + 10, 8), Math.max(width - 190, 8))
  const top = Math.max(hover.y - 62, 8)
  return (
    <div
      className="pointer-events-none absolute z-10 w-[180px] rounded-sm border border-border/80 bg-surface-1 px-2.5 py-2 text-[10px] shadow-sm"
      style={{ left, top }}
    >
      <div className="flex items-center gap-2 text-muted-foreground">
        <span
          className="h-1.5 w-1.5 rounded-full"
          style={{ backgroundColor: hover.color }}
        />
        <span className="truncate">{hover.point.label}</span>
      </div>
      <div className="mt-1 font-mono text-[10px] text-muted-foreground">
        {formatTime(hover.point.date)}
      </div>
      <div className="mt-2 grid grid-cols-[1fr_auto] gap-x-3 gap-y-1">
        <span className="text-muted-foreground">Confirm</span>
        <span className="font-mono text-foreground">
          {formatLatency(hover.point.ms)}
        </span>
        <span className="text-muted-foreground">{hover.point.secondaryLabel}</span>
        <span className="font-mono text-foreground">
          {formatLatency(hover.point.secondaryMs)}
        </span>
      </div>
    </div>
  )
}

function buildSeries(samples: Array<Sample>): Array<Series> {
  const grouped = new Map<string, Array<Point>>()

  for (const sample of samples) {
    if (!sample.ok || sample.warmup || sample.network_ns <= 0) {
      continue
    }

    const date = new Date(sample.completed_at)
    if (Number.isNaN(date.getTime())) {
      continue
    }

    const key = [
      sample.venue,
      sample.transport,
      sample.scenario,
      sample.order_type || "unknown",
      sample.measurement_mode || "ack",
    ].join(":")
    const points = grouped.get(key) ?? []
    points.push({
      date,
      label: key.replaceAll(":", " / "),
      ms: confirmSampleMs(sample),
      secondaryLabel: secondaryLabel(sample.venue),
      secondaryMs: secondarySampleMs(sample),
    })
    grouped.set(key, points)
  }

  return [...grouped.entries()]
    .map(([key, points], index) => ({
      color: COLORS[index % COLORS.length],
      key,
      label: key.replaceAll(":", " / "),
      points: points.sort((a, b) => a.date.getTime() - b.date.getTime()),
    }))
    .filter((item) => item.points.length > 0)
}

function getDomains(series: Array<Series>) {
  const points = series.flatMap((item) => item.points)

  if (points.length === 0) {
    return null
  }

  const minTime = Math.min(...points.map((point) => point.date.getTime()))
  const maxTime = Math.max(...points.map((point) => point.date.getTime()))
  const maxLatency = Math.max(...points.map((point) => point.ms))
  const timePadding = minTime === maxTime ? 60_000 : 0
  const latencyPadding = Math.max(maxLatency * 0.12, 1)

  return {
    x: [
      new Date(minTime - timePadding),
      new Date(maxTime + timePadding),
    ] as [Date, Date],
    y: [0, maxLatency + latencyPadding] as [number, number],
  }
}
