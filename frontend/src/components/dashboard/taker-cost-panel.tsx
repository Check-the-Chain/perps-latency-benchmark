"use client"

import { AxisBottom, AxisLeft } from "@visx/axis"
import { localPoint } from "@visx/event"
import { GridRows } from "@visx/grid"
import { Group } from "@visx/group"
import { ParentSize } from "@visx/responsive"
import { scaleLinear, scaleTime } from "@visx/scale"
import { LinePath } from "@visx/shape"
import { TooltipWithBounds, useTooltip } from "@visx/tooltip"
import {
  Fragment,
  type PointerEvent,
  type ReactNode,
  useEffect,
  useMemo,
  useState,
} from "react"

import type { Sample } from "@/api/bench"
import { colorForVenue } from "@/components/charts/latency-timeseries-chart"
import { VenueName } from "@/components/dashboard/venue-logo"
import {
  formatAbsBps,
  formatBps,
  formatCount,
  formatPrice,
  formatSignedUSD,
  formatTime,
  formatUSD,
} from "@/lib/format"
import {
  buildTakerCostRecords,
  stableCostWindow,
  summarizeSlippage,
  type TakerCostRecord,
} from "@/lib/taker-cost"

const CHART_MARGIN = { top: 16, right: 18, bottom: 32, left: 58 }
const ROLLING_MEDIAN_POINTS = 9

type CostChartMode = "total" | "net" | "fees"
type CostDisplayMode = "raw" | "trend" | "trend-raw"
type CostChartView = "per-round" | "cumulative"

const COST_CHART_MODES: Array<{ label: string; mode: CostChartMode }> = [
  { label: "Total cost", mode: "total" },
  { label: "Net trading cost", mode: "net" },
  { label: "Fees", mode: "fees" },
]

const COST_MODE_COPY: Record<CostChartMode, { description: string; title: string }> = {
  fees: {
    description: "Explicit trading fees paid on the open and close orders.",
    title: "Fees per Round",
  },
  net: {
    description: "Round-trip cost after removing explicit trading fees, showing spread, slippage, and price movement.",
    title: "Net Trading Cost per Round",
  },
  total: {
    description: "Full round-trip cost for one open and close sequence, including trading fees.",
    title: "Cost per Round",
  },
}

export function TakerCostPanel({ samples }: { samples: Array<Sample> }) {
  const [chartMode, setChartMode] = useState<CostChartMode>("total")
  const records = useMemo(() => buildTakerCostRecords(samples), [samples])
  const stable = useMemo(() => stableCostWindow(records), [records])
  const slippage = useMemo(() => summarizeSlippage(stable.records), [stable.records])
  const cheapestVenue = useMemo(
    () => minBy(slippage, (row) => row.meanCostUSD),
    [slippage]
  )
  const mostExpensiveVenue = useMemo(
    () => maxBy(slippage, (row) => row.meanCostUSD),
    [slippage]
  )
  const benchmarkSize = useMemo(() => formatBenchmarkSize(stable.records), [stable.records])

  if (records.length === 0) {
    return (
      <section className="rounded-sm border border-border/80 bg-surface-1 p-3 text-[11px] text-muted-foreground">
        No taker cost data is available for the selected filters.
      </section>
    )
  }

  return (
    <section className="overflow-hidden rounded-sm border border-border/80 bg-surface-1">
      <div className="border-b border-border/80 px-3 py-2">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="font-sans text-sm font-semibold">Taker Cost</h2>
            <p className="mt-1 text-[11px] text-muted-foreground">
              BTC market-order benchmark. Each venue opens a position, then closes it; costs use matched exchange fills.
            </p>
          </div>
        </div>
      </div>

      <div className="grid gap-3 border-b border-border/80 p-3 md:grid-cols-3">
        <CostStat
          label="Benchmark size"
          value={benchmarkSize}
          detail="Open once, then close once"
        />
        <CostStat
          label="Lowest avg round"
          value={formatUSD(cheapestVenue?.meanCostUSD)}
          detail={cheapestVenue ? <VenueName venue={cheapestVenue.venue} /> : "No comparable rounds"}
        />
        <CostStat
          label="Highest avg round"
          value={formatUSD(mostExpensiveVenue?.meanCostUSD)}
          detail={mostExpensiveVenue ? <VenueName venue={mostExpensiveVenue.venue} /> : "No comparable rounds"}
        />
      </div>

      <div className="grid min-w-0 gap-3 border-b border-border/80 p-3 xl:grid-cols-2">
        <CostChart
          mode={chartMode}
          onModeChange={setChartMode}
          records={stable.records}
        />
        <VenueCostBars rows={slippage} />
      </div>

      <div className="grid min-w-0 gap-3 p-3">
        <VenueCostTable rows={slippage} />
        <RecentCostTable records={[...stable.records].reverse().slice(0, 12)} />
      </div>
    </section>
  )
}

function CostStat({
  detail,
  label,
  value,
}: {
  detail: ReactNode
  label: string
  value: string
}) {
  return (
    <div className="rounded-sm border border-border/70 bg-surface-1 p-3">
      <div className="text-[10px] uppercase text-muted-foreground">{label}</div>
      <div className="tabular mt-2 font-sans text-xl font-semibold tracking-normal">
        {value}
      </div>
      <div className="mt-1 text-[11px] text-muted-foreground">{detail}</div>
    </div>
  )
}

function CostChart({
  mode,
  onModeChange,
  records,
}: {
  mode: CostChartMode
  onModeChange: (mode: CostChartMode) => void
  records: Array<TakerCostRecord>
}) {
  const [displayMode, setDisplayMode] = useState<CostDisplayMode>("trend-raw")
  const [hideOutliers, setHideOutliers] = useState(true)
  const [view, setView] = useState<CostChartView>("per-round")
  const copy = costChartCopy(mode, view)
  const showRoundControls = view === "per-round"

  return (
    <div className="min-h-[300px] min-w-0 overflow-visible">
      <div className="mb-2 flex flex-wrap items-start justify-between gap-2">
        <div>
          <h3 className="font-sans text-xs font-semibold">{copy.title}</h3>
          <p className="mt-1 max-w-[520px] text-[10px] text-muted-foreground">{copy.description}</p>
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          {showRoundControls ? (
            <>
              <CostDisplayModeToggle value={displayMode} onChange={setDisplayMode} />
              <CostOutlierToggle checked={hideOutliers} onChange={setHideOutliers} />
            </>
          ) : null}
          <CostViewToggle value={view} onChange={setView} />
          <CostModeToggle mode={mode} onChange={onModeChange} />
        </div>
      </div>
      <div className="h-[250px] overflow-visible">
        {records.length === 0 ? (
          <div className="flex h-full items-center text-[11px] text-muted-foreground">
            No comparable cost data is available.
          </div>
        ) : (
          <ParentSize>
            {({ width, height }) => (
              <CostChartFrame
                displayMode={displayMode}
                height={height}
                hideOutliers={hideOutliers}
                mode={mode}
                records={records}
                view={view}
                width={width}
              />
            )}
          </ParentSize>
        )}
      </div>
    </div>
  )
}

function costChartCopy(mode: CostChartMode, view: CostChartView) {
  if (view === "per-round") {
    return COST_MODE_COPY[mode]
  }

  if (mode === "fees") {
    return {
      description: "Running total of explicit trading fees paid on open and close orders by venue.",
      title: "Cumulative Fees",
    }
  }
  if (mode === "net") {
    return {
      description: "Running total of round-trip cost after removing explicit trading fees.",
      title: "Cumulative Net Trading Cost",
    }
  }
  return {
    description: "Running total of full round-trip cost by venue, including trading fees.",
    title: "Cumulative Cost",
  }
}

function CostDisplayModeToggle({
  value,
  onChange,
}: {
  value: CostDisplayMode
  onChange: (mode: CostDisplayMode) => void
}) {
  const modes: Array<{ label: string; mode: CostDisplayMode }> = [
    { label: "Raw", mode: "raw" },
    { label: "Trend", mode: "trend" },
    { label: "Trend + raw", mode: "trend-raw" },
  ]

  return (
    <div className="inline-flex rounded-sm border border-border bg-surface-2 p-0.5">
      {modes.map((item) => (
        <button
          key={item.mode}
          type="button"
          onClick={() => onChange(item.mode)}
          className={`h-7 px-2 text-[10px] ${
            value === item.mode
              ? "bg-surface-1 text-foreground shadow-sm"
              : "text-muted-foreground hover:text-foreground"
          }`}
          aria-pressed={value === item.mode}
        >
          {item.label}
        </button>
      ))}
    </div>
  )
}

function CostOutlierToggle({
  checked,
  onChange,
}: {
  checked: boolean
  onChange: (checked: boolean) => void
}) {
  return (
    <label className="flex h-8 cursor-pointer items-center gap-2 rounded-sm border border-border bg-surface-1 px-2 text-[10px] text-muted-foreground hover:bg-surface-2 hover:text-foreground">
      <input
        type="checkbox"
        checked={checked}
        onChange={(event) => onChange(event.currentTarget.checked)}
        className="size-3 accent-[var(--primary)]"
      />
      <span>Hide outliers</span>
    </label>
  )
}

function CostViewToggle({
  value,
  onChange,
}: {
  value: CostChartView
  onChange: (view: CostChartView) => void
}) {
  const views: Array<{ label: string; view: CostChartView }> = [
    { label: "Per round", view: "per-round" },
    { label: "Cumulative", view: "cumulative" },
  ]

  return (
    <div className="inline-flex rounded-sm border border-border bg-surface-2 p-0.5">
      {views.map((item) => (
        <button
          key={item.view}
          type="button"
          onClick={() => onChange(item.view)}
          className={`h-7 px-2 text-[10px] ${
            value === item.view
              ? "bg-surface-1 text-foreground shadow-sm"
              : "text-muted-foreground hover:text-foreground"
          }`}
          aria-pressed={value === item.view}
        >
          {item.label}
        </button>
      ))}
    </div>
  )
}

function CostModeToggle({
  mode,
  onChange,
}: {
  mode: CostChartMode
  onChange: (mode: CostChartMode) => void
}) {
  return (
    <div className="inline-flex rounded-sm border border-border bg-surface-2 p-0.5">
      {COST_CHART_MODES.map((item) => (
        <button
          key={item.mode}
          type="button"
          onClick={() => onChange(item.mode)}
          className={`h-7 px-2 text-[10px] ${
            mode === item.mode
              ? "bg-surface-1 text-foreground shadow-sm"
              : "text-muted-foreground hover:text-foreground"
          }`}
          aria-pressed={mode === item.mode}
        >
          {item.label}
        </button>
      ))}
    </div>
  )
}

function VenueCostBars({ rows }: { rows: ReturnType<typeof summarizeSlippage> }) {
  const ordered = [...rows].sort((left, right) => left.meanCostUSD - right.meanCostUSD)
  const extent = costExtent(ordered.map((row) => row.meanCostUSD))
  const range = Math.max(extent.max - extent.min, 0.001)
  const zeroPercent = ((0 - extent.min) / range) * 100

  return (
    <div className="min-h-[300px] min-w-0 overflow-hidden">
      <div className="mb-2">
        <h3 className="font-sans text-xs font-semibold">Average Round Cost by Venue</h3>
        <p className="mt-1 text-[10px] text-muted-foreground">
          Average cost for one open and close sequence.
        </p>
      </div>
      <div className="space-y-2 pt-4">
        {ordered.map((row) => (
          <div
            key={row.venue}
            className="grid grid-cols-[132px_minmax(80px,1fr)_76px] items-center gap-2 text-[11px]"
          >
            <div className="leading-tight text-muted-foreground">
              {formatAverageCostVenue(row.venue)}
            </div>
            <div className="relative h-5 rounded-sm bg-surface-2">
              <div
                className="absolute top-0 h-5 w-px bg-border"
                style={{ left: `${zeroPercent}%` }}
              />
              <div
                className="absolute top-1/2 h-3 -translate-y-1/2 rounded-sm"
                style={{
                  backgroundColor: colorForVenue(row.venue),
                  left: `${Math.min(zeroPercent, ((row.meanCostUSD - extent.min) / range) * 100)}%`,
                  width: `${Math.max(
                    Math.abs(((row.meanCostUSD - extent.min) / range) * 100 - zeroPercent),
                    2
                  )}%`,
                }}
              />
            </div>
            <div className="text-right font-mono">{formatUSD(row.meanCostUSD)}</div>
          </div>
        ))}
      </div>
    </div>
  )
}

function CostChartFrame({
  displayMode,
  height,
  hideOutliers,
  mode,
  records,
  view,
  width,
}: {
  displayMode: CostDisplayMode
  height: number
  hideOutliers: boolean
  mode: CostChartMode
  records: Array<TakerCostRecord>
  view: CostChartView
  width: number
}) {
  const {
    hideTooltip,
    showTooltip,
    tooltipData,
    tooltipLeft = 0,
    tooltipOpen,
    tooltipTop = 0,
  } = useTooltip<CostPoint>()
  const series = useMemo(() => buildCostSeries(records, mode, view), [mode, records, view])
  const displaySeries = useMemo(
    () =>
      buildDisplayCostSeries(
        series,
        view === "cumulative" ? "raw" : displayMode,
        view === "cumulative" ? false : hideOutliers
      ),
    [displayMode, hideOutliers, series, view]
  )
  useDismissTooltipOnViewportChange(hideTooltip)
  useEffect(() => {
    hideTooltip()
  }, [displayMode, displaySeries, height, hideOutliers, hideTooltip, mode, view, width])

  if (width <= 0 || height <= 0 || displaySeries.length === 0) {
    return null
  }

  const points = displaySeries.flatMap((item) => [
    ...item.linePoints,
    ...item.rawPoints,
  ])
  const minTime = Math.min(...points.map((point) => point.date.getTime()))
  const maxTime = Math.max(...points.map((point) => point.date.getTime()))
  const yDomain = paddedCostDomain(points.map((point) => point.value))
  const innerWidth = Math.max(width - CHART_MARGIN.left - CHART_MARGIN.right, 1)
  const innerHeight = Math.max(height - CHART_MARGIN.top - CHART_MARGIN.bottom, 1)
  const xTickCount = Math.max(2, Math.min(5, Math.floor(innerWidth / 150)))
  const xScale = scaleTime({
    domain: [
      new Date(minTime === maxTime ? minTime - 60_000 : minTime),
      new Date(minTime === maxTime ? maxTime + 60_000 : maxTime),
    ],
    range: [0, innerWidth],
  })
  const yScale = scaleLinear({
    domain: yDomain,
    nice: true,
    range: [innerHeight, 0],
  })
  const zeroY = yScale(0)
  const showHover = (
    event: PointerEvent<SVGElement>,
    point: CostPointBase,
    venue: string
  ) => {
    const key = costPointKey(point)
    const coords = localPoint(event) ?? {
      x: CHART_MARGIN.left + xScale(point.date),
      y: CHART_MARGIN.top + yScale(point.value),
    }
    showTooltip({
      tooltipData: {
        ...point,
        color: colorForVenue(venue),
        key,
      },
      tooltipLeft: coords.x + 10,
      tooltipTop: coords.y - 96,
    })
  }

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
        aria-label="Taker round cost"
        onPointerLeave={hideTooltip}
        onPointerCancel={hideTooltip}
        onBlur={hideTooltip}
      >
        <Group left={CHART_MARGIN.left} top={CHART_MARGIN.top}>
          <GridRows
            scale={yScale}
            numTicks={5}
            width={innerWidth}
            stroke="oklch(0.88 0.004 255 / 0.7)"
            strokeDasharray="3 4"
          />
          {zeroY >= 0 && zeroY <= innerHeight ? (
            <line
              x1={0}
              x2={innerWidth}
              y1={zeroY}
              y2={zeroY}
              stroke="oklch(0.62 0.02 253)"
              strokeWidth={1}
            />
          ) : null}
          <AxisLeft
            scale={yScale}
            numTicks={5}
            tickFormat={(value) => formatUSD(Number(value))}
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
          <rect
            width={innerWidth}
            height={innerHeight}
            fill="transparent"
            onPointerEnter={hideTooltip}
            onPointerMove={hideTooltip}
          />
          {displaySeries.map((item) => (
            <Group key={item.venue}>
              {displayMode === "trend-raw"
                ? item.rawPoints.map((point) => (
                    <Group
                      key={`${item.venue}:raw:${point.date.toISOString()}:${point.value}`}
                      onPointerEnter={(event) =>
                        showHover(event, point, item.venue)
                      }
                      onPointerMove={(event) =>
                        showHover(event, point, item.venue)
                      }
                      onPointerLeave={hideTooltip}
                    >
                      <circle
                        cx={xScale(point.date)}
                        cy={yScale(point.value)}
                        r={6}
                        fill="transparent"
                      />
                      <circle
                        cx={xScale(point.date)}
                        cy={yScale(point.value)}
                        r={1.8}
                        fill={colorForVenue(item.venue)}
                        opacity={0.28}
                      />
                    </Group>
                  ))
                : null}
              <LinePath
                data={item.linePoints}
                x={(point) => xScale(point.date)}
                y={(point) => yScale(point.value)}
                pointerEvents="none"
                stroke={colorForVenue(item.venue)}
                strokeWidth={1.7}
              />
              {item.linePoints.map((point) => (
                <Group
                  key={`${item.venue}:line:${point.date.toISOString()}:${point.value}`}
                  onPointerEnter={(event) => showHover(event, point, item.venue)}
                  onPointerMove={(event) => showHover(event, point, item.venue)}
                  onPointerLeave={hideTooltip}
                >
                  <circle cx={xScale(point.date)} cy={yScale(point.value)} r={7} fill="transparent" />
                  <circle
                    cx={xScale(point.date)}
                    cy={yScale(point.value)}
                    r={2.5}
                    fill={colorForVenue(item.venue)}
                    opacity={displayMode === "trend-raw" ? 0 : 1}
                  />
                </Group>
              ))}
            </Group>
          ))}
        </Group>
      </svg>
      {tooltipOpen && tooltipData ? (
        <TooltipWithBounds
          left={tooltipLeft}
          top={tooltipTop}
          className="pointer-events-none z-10 w-[250px] rounded-sm border border-border/80 bg-surface-1 px-2.5 py-2 text-[10px] shadow-sm"
        >
          <CostTooltip hover={tooltipData} mode={mode} view={view} />
        </TooltipWithBounds>
      ) : null}
    </div>
  )
}

function useDismissTooltipOnViewportChange(hideTooltip: () => void) {
  useEffect(() => {
    window.addEventListener("scroll", hideTooltip, true)
    window.addEventListener("resize", hideTooltip)
    window.addEventListener("blur", hideTooltip)

    return () => {
      window.removeEventListener("scroll", hideTooltip, true)
      window.removeEventListener("resize", hideTooltip)
      window.removeEventListener("blur", hideTooltip)
    }
  }, [hideTooltip])
}

interface CostPointBase {
  date: Date
  kind: "raw" | "rolling-median"
  record: TakerCostRecord
  sampleCount?: number
  value: number
  venue: string
}

interface CostPoint {
  color: string
  date: Date
  kind: "raw" | "rolling-median"
  key: string
  record: TakerCostRecord
  sampleCount?: number
  value: number
  venue: string
}

interface CostSeries {
  points: Array<CostPointBase>
  venue: string
}

function CostTooltip({
  hover,
  mode,
  view,
}: {
  hover: CostPoint
  mode: CostChartMode
  view: CostChartView
}) {
  const record = hover.record
  const rows = tooltipRows(record, mode, view, hover.value)

  return (
    <>
      <div className="flex items-center gap-2 text-muted-foreground">
        <span className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: hover.color }} />
        <VenueName venue={hover.venue} />
        <span className="ml-auto font-mono">{formatTime(hover.date)}</span>
      </div>
      <div className="mt-2 grid grid-cols-[1fr_auto] gap-x-3 gap-y-1">
        {rows.map((row) => (
          <Fragment key={row.label}>
            <span className={row.primary ? "-mx-1 rounded-sm bg-surface-2 px-1 py-0.5 font-bold text-foreground" : "text-muted-foreground"}>
              {row.label}
            </span>
            <span className={row.primary ? "-mx-1 rounded-sm bg-surface-2 px-1 py-0.5 font-mono font-bold text-foreground" : "font-mono"}>
              {formatUSD(row.value)}
            </span>
          </Fragment>
        ))}
        {hover.sampleCount ? (
          <>
            <span className="text-muted-foreground">Window</span>
            <span className="font-mono">{formatCount(hover.sampleCount)} rounds</span>
          </>
        ) : null}
        <span className="text-muted-foreground">Open fee</span>
        <span className="font-mono">{formatUSD(record.entryFeeUSD)}</span>
        <span className="text-muted-foreground">Close fee</span>
        <span className="font-mono">{formatUSD(record.exitFeeUSD)}</span>
        <span className="text-muted-foreground">Price move</span>
        <span className="font-mono">{formatSignedUSD(record.priceMoveCostUSD)}</span>
        <span className="text-muted-foreground">Book deviation</span>
        <span className="font-mono">{formatSignedUSD(record.totalSlippageUSD)}</span>
      </div>
    </>
  )
}

function costPointKey(point: CostPointBase) {
  return `${point.venue}:${point.date.toISOString()}:${point.value}`
}

function tooltipRows(
  record: TakerCostRecord,
  mode: CostChartMode,
  view: CostChartView,
  primaryValue: number
) {
  const rows = [
    { label: "Total cost", mode: "total" as const, value: record.tradeCostUSD },
    { label: "Net trading cost", mode: "net" as const, value: netTradingCostUSD(record) },
    { label: "Trading fees", mode: "fees" as const, value: tradingFeesUSD(record) },
  ]
  return rows.map((row) => ({
    ...row,
    primary: row.mode === mode,
    label: row.mode === mode && view === "cumulative" ? `Cumulative ${row.label.toLowerCase()}` : row.label,
    value: row.mode === mode ? primaryValue : row.value,
  }))
}

function paddedCostDomain(values: Array<number>): [number, number] {
  const extent = costExtent(values)
  const span = Math.max(extent.max - extent.min, Math.abs(extent.max), Math.abs(extent.min), 0.001)
  const padding = span * 0.16
  return [
    Math.min(extent.min - padding, 0),
    Math.max(extent.max + padding, 0),
  ]
}

function costExtent(values: Array<number>) {
  const finite = values.filter(Number.isFinite)
  if (finite.length === 0) {
    return { max: 0.001, min: 0 }
  }
  const min = Math.min(...finite, 0)
  const max = Math.max(...finite, 0)
  if (min === max) {
    const padding = Math.max(Math.abs(min), 0.001)
    return { max: max + padding, min: min - padding }
  }
  return { max, min }
}

function VenueCostTable({ rows }: { rows: ReturnType<typeof summarizeSlippage> }) {
  return (
    <div className="min-w-0 overflow-hidden rounded-sm border border-border/70">
      <div className="border-b border-border/70 px-3 py-2">
        <h3 className="font-sans text-xs font-semibold">Venue Cost Comparison</h3>
        <p className="mt-1 text-[10px] text-muted-foreground">
          Book miss compares execution with the depth-weighted visible book at send time. Positive is worse; negative is price improvement.
        </p>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[760px] border-collapse text-left text-[11px]">
          <thead className="bg-surface-2 text-muted-foreground">
            <tr>
              <HeaderCell>Venue</HeaderCell>
              <HeaderCell align="right">Runs</HeaderCell>
              <HeaderCell align="right">Avg round</HeaderCell>
              <HeaderCell align="right">Avg fill</HeaderCell>
              <HeaderCell align="right">Fee / fill</HeaderCell>
              <HeaderCell align="right">Price move</HeaderCell>
              <HeaderCell align="right">Book miss p95</HeaderCell>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 ? (
              <tr>
                <td colSpan={7} className="px-3 py-6 text-muted-foreground">
                  No comparable cost data is available.
                </td>
              </tr>
            ) : (
              [...rows].sort((left, right) => left.meanCostUSD - right.meanCostUSD).map((row) => (
                <tr key={row.venue} className="border-t border-border/70">
                  <BodyCell className="font-medium">
                    <VenueName venue={row.venue} />
                  </BodyCell>
                  <BodyCell align="right">{formatCount(row.cleanCount)}</BodyCell>
                  <BodyCell align="right">{formatUSD(row.meanCostUSD)}</BodyCell>
                  <BodyCell align="right">{formatUSD(row.meanFillCostUSD)}</BodyCell>
                  <BodyCell align="right">{formatUSD(row.feeMeanPerFillUSD)}</BodyCell>
                  <BodyCell align="right">{formatSignedUSD(row.priceMoveMeanUSD)}</BodyCell>
                  <BodyCell align="right">{formatAbsBps(row.totalSlippageP95Bps)}</BodyCell>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function RecentCostTable({ records }: { records: Array<TakerCostRecord> }) {
  return (
    <div className="min-w-0 overflow-hidden rounded-sm border border-border/70">
      <div className="border-b border-border/70 px-3 py-2">
        <h3 className="font-sans text-xs font-semibold">Recent Taker Rounds</h3>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[880px] border-collapse text-left text-[11px]">
          <thead className="bg-surface-2 text-muted-foreground">
            <tr>
              <HeaderCell>Time</HeaderCell>
              <HeaderCell>Venue</HeaderCell>
              <HeaderCell align="right">Cost</HeaderCell>
              <HeaderCell align="right">Open fill</HeaderCell>
              <HeaderCell align="right">Open book</HeaderCell>
              <HeaderCell align="right">Open dev</HeaderCell>
              <HeaderCell align="right">Close fill</HeaderCell>
              <HeaderCell align="right">Close book</HeaderCell>
              <HeaderCell align="right">Close dev</HeaderCell>
              <HeaderCell align="right">Fees</HeaderCell>
            </tr>
          </thead>
          <tbody>
            {records.map((record) => (
              <tr key={`${record.venue}:${record.date.toISOString()}`} className="border-t border-border/70">
                <BodyCell>{formatTime(record.date)}</BodyCell>
                <BodyCell className="font-medium">
                  <VenueName venue={record.venue} />
                </BodyCell>
                <BodyCell align="right">{formatUSD(record.tradeCostUSD)}</BodyCell>
                <BodyCell align="right">{formatPrice(record.entryActualPrice)}</BodyCell>
                <BodyCell align="right">{formatPrice(record.entryExpectedPrice)}</BodyCell>
                <BodyCell align="right">{formatBps(record.entrySlippageBps)}</BodyCell>
                <BodyCell align="right">{formatPrice(record.exitActualPrice)}</BodyCell>
                <BodyCell align="right">{formatPrice(record.exitExpectedPrice)}</BodyCell>
                <BodyCell align="right">{formatBps(record.exitSlippageBps)}</BodyCell>
                <BodyCell align="right">{formatUSD(record.entryFeeUSD + record.exitFeeUSD)}</BodyCell>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function HeaderCell({
  align = "left",
  children,
}: {
  align?: "left" | "right"
  children: ReactNode
}) {
  return (
    <th className="px-3 py-2 font-normal" style={{ textAlign: align }} scope="col">
      {children}
    </th>
  )
}

function BodyCell({
  align = "left",
  children,
  className,
}: {
  align?: "left" | "right"
  children: ReactNode
  className?: string
}) {
  return (
    <td className={`tabular px-3 py-2 ${className ?? ""}`} style={{ textAlign: align }}>
      {children}
    </td>
  )
}

function buildCostSeries(
  records: Array<TakerCostRecord>,
  mode: CostChartMode,
  view: CostChartView
): Array<CostSeries> {
  const runningTotals = new Map<string, number>()
  const points = records.map((record) => {
    const roundValue = costValue(record, mode)
    const value =
      view === "cumulative"
        ? (runningTotals.get(record.venue) ?? 0) + roundValue
        : roundValue

    if (view === "cumulative") {
      runningTotals.set(record.venue, value)
    }

    return {
      date: record.date,
      kind: "raw" as const,
      record,
      value,
      venue: record.venue,
    }
  })
  const groups = new Map<string, typeof points>()
  for (const point of points) {
    groups.set(point.venue, [...(groups.get(point.venue) ?? []), point])
  }
  return [...groups.entries()]
    .map(([venue, venuePoints]) => ({
      points: venuePoints.sort((left, right) => left.date.getTime() - right.date.getTime()),
      venue,
    }))
    .sort((left, right) => left.venue.localeCompare(right.venue))
}

function buildDisplayCostSeries(
  series: Array<CostSeries>,
  displayMode: CostDisplayMode,
  hideOutliers: boolean
) {
  return series
    .map((item) => {
      const points = hideOutliers ? withoutCostOutliers(item.points) : item.points
      const trendPoints = rollingMedianCostPoints(points, ROLLING_MEDIAN_POINTS)

      return {
        ...item,
        linePoints: displayMode === "raw" ? points : trendPoints,
        rawPoints: displayMode === "trend-raw" ? points : [],
      }
    })
    .filter((item) => item.linePoints.length > 0 || item.rawPoints.length > 0)
}

function rollingMedianCostPoints(
  points: Array<CostPointBase>,
  windowSize: number
): Array<CostPointBase> {
  return points.map((point, index) => {
    const start = Math.max(0, index - windowSize + 1)
    const window = points.slice(start, index + 1)

    return {
      ...point,
      kind: "rolling-median",
      sampleCount: window.length,
      value: numericMedian(window.map((item) => item.value)),
    }
  })
}

function withoutCostOutliers(points: Array<CostPointBase>) {
  if (points.length < 8) {
    return points
  }

  const values = points.map((point) => point.value).sort((a, b) => a - b)
  const q1 = numericQuantile(values, 0.25)
  const q3 = numericQuantile(values, 0.75)
  const iqr = q3 - q1
  const padding = iqr > 0 ? 1.5 * iqr : Math.max(Math.abs(q3), 0.001) * 3
  const lowerFence = q1 - padding
  const upperFence = q3 + padding

  return points.filter(
    (point) => point.value >= lowerFence && point.value <= upperFence
  )
}

function costValue(record: TakerCostRecord, mode: CostChartMode) {
  if (mode === "fees") {
    return tradingFeesUSD(record)
  }
  if (mode === "net") {
    return netTradingCostUSD(record)
  }
  return record.tradeCostUSD
}

function netTradingCostUSD(record: TakerCostRecord) {
  return record.tradeCostUSD - tradingFeesUSD(record)
}

function tradingFeesUSD(record: TakerCostRecord) {
  return record.entryFeeUSD + record.exitFeeUSD
}

function numericMedian(values: Array<number>) {
  return numericQuantile(
    [...values].sort((a, b) => a - b),
    0.5
  )
}

function numericQuantile(sortedValues: Array<number>, q: number) {
  if (sortedValues.length === 0) {
    return 0
  }
  if (sortedValues.length === 1) {
    return sortedValues[0]
  }

  const index = (sortedValues.length - 1) * q
  const lower = Math.floor(index)
  const upper = Math.ceil(index)
  const weight = index - lower

  return sortedValues[lower] * (1 - weight) + sortedValues[upper] * weight
}

function formatAverageCostVenue(value: string) {
  if (value === "lighter") {
    return (
      <span className="inline-flex min-w-0 items-start gap-2">
        <VenueName label="Lighter Premium" venue={value} />
        <span className="block text-muted-foreground/80">base tier</span>
      </span>
    )
  }

  return <VenueName venue={value} />
}

function formatBenchmarkSize(records: Array<TakerCostRecord>) {
  const qty = median(records.map((record) => record.cost.entry_qty ?? 0).filter((value) => value > 0))
  if (!qty) {
    return "-"
  }
  return `${formatQty(qty)} BTC`
}

function formatQty(value: number) {
  return new Intl.NumberFormat("en", {
    maximumFractionDigits: 6,
    minimumFractionDigits: 0,
  }).format(value)
}

function minBy<T>(items: Array<T>, score: (item: T) => number) {
  let best: T | undefined
  let bestScore = Number.POSITIVE_INFINITY
  for (const item of items) {
    const value = score(item)
    if (Number.isFinite(value) && value < bestScore) {
      best = item
      bestScore = value
    }
  }
  return best
}

function maxBy<T>(items: Array<T>, score: (item: T) => number) {
  let best: T | undefined
  let bestScore = Number.NEGATIVE_INFINITY
  for (const item of items) {
    const value = score(item)
    if (Number.isFinite(value) && value > bestScore) {
      best = item
      bestScore = value
    }
  }
  return best
}

function median(values: Array<number>) {
  if (values.length === 0) {
    return undefined
  }
  const sorted = [...values].sort((left, right) => left - right)
  return sorted[Math.floor(sorted.length / 2)]
}
