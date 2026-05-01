"use client"

import { AxisBottom, AxisLeft } from "@visx/axis"
import { GridRows } from "@visx/grid"
import { Group } from "@visx/group"
import { ParentSize } from "@visx/responsive"
import { scaleLinear, scaleTime } from "@visx/scale"
import { LinePath } from "@visx/shape"
import { useMemo } from "react"

import type { Sample } from "@/api/bench"
import { formatLatency, formatTime, nsToMs } from "@/lib/format"

interface Series {
  color: string
  key: string
  label: string
  points: Array<Point>
}

interface Point {
  date: Date
  ms: number
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
            Confirmed order updates over time by venue, transport, and order
            type when available.
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
              <LatencySvg
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

function LatencySvg({
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
              <circle
                key={`${point.date.toISOString()}:${point.ms}`}
                cx={xScale(point.date)}
                cy={yScale(point.ms)}
                r={1.8}
                fill={item.color}
              />
            ))}
          </Group>
        ))}
      </Group>
    </svg>
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
    points.push({ date, ms: nsToMs(sample.network_ns) })
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
