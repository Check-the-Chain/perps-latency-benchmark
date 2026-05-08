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
  type ReactNode,
  useEffect,
  useId,
  useMemo,
  useState,
} from "react"

import type { Sample } from "@/api/bench"
import { VenueName, formatVenueLabel } from "@/components/dashboard/venue-logo"
import { samplePlotDate } from "@/lib/sample-time"
import { formatLatency, formatTime } from "@/lib/format"
import { confirmSampleMs } from "@/lib/latency-metric"

interface Series {
  color: string
  key: string
  label: string
  points: Array<Point>
  strokeDasharray?: string
  venue: string
}

interface DisplaySeries extends Series {
  bandPoints: Array<BandPoint>
  linePoints: Array<Point>
  outlierCount: number
  rawPoints: Array<Point>
}

interface Point {
  date: Date
  kind: "raw" | "rolling-median"
  ms: number
  sampleCount?: number
}

interface BandPoint {
  date: Date
  lowerMS: number
  upperMS: number
}

interface HoverPoint {
  color: string
  point: Point
  series: Series
}

interface ScaledHoverPoint extends HoverPoint {
  x: number
  y: number
}

export type LatencyScaleMode = "linear" | "log"
type LatencyDisplayMode = "raw" | "trend" | "trend-raw"
type DateScale = (value: Date) => number
type NumberScale = (value: number) => number

const COLORS = [
  "oklch(0.52 0.17 215)",
  "oklch(0.5 0.14 160)",
  "oklch(0.6 0.16 40)",
  "oklch(0.55 0.16 285)",
  "oklch(0.58 0.17 18)",
  "oklch(0.45 0.08 250)",
]

const MARGIN = { top: 18, right: 20, bottom: 34, left: 62 }
const TREND_WINDOW_MS = 30 * 60 * 1000
const TREND_STEP_MS = 5 * 60 * 1000
const TREND_LABEL = "30m median"
const TOOLTIP_HIT_RADIUS_PX = 28

export function LatencyTimeseriesChart({
  samples,
  isLoading = false,
  scaleMode,
  selectedVenues,
  venues,
  onScaleModeChange,
  onVenueSelectionChange,
  title = "Latency Timeline",
  description = "Confirmation latency over time by venue.",
  emptyMessage = "No latency data is available for the selected filters.",
  headerActions,
  valueForSample = confirmSampleMs,
  valueLabel = "Latency",
}: {
  samples: Array<Sample>
  isLoading?: boolean
  scaleMode: LatencyScaleMode
  selectedVenues: Array<string>
  venues: Array<string>
  onScaleModeChange: (mode: LatencyScaleMode) => void
  onVenueSelectionChange: (venues: "all" | Array<string>) => void
  title?: string
  description?: string
  emptyMessage?: string
  headerActions?: ReactNode
  valueForSample?: (sample: Sample) => number | undefined
  valueLabel?: string
}) {
  const [displayMode, setDisplayMode] =
    useState<LatencyDisplayMode>("trend-raw")
  const [hideOutliers, setHideOutliers] = useState(true)
  const series = useMemo(
    () => buildSeries(samples, valueForSample),
    [samples, valueForSample]
  )
  const displaySeries = useMemo(
    () =>
      buildDisplaySeries(series, {
        displayMode,
        hideOutliers,
      }),
    [displayMode, hideOutliers, series]
  )
  const domain = useMemo(
    () => getDomains(displaySeries, scaleMode),
    [displaySeries, scaleMode]
  )

  return (
    <section className="rounded-sm border border-border/80 bg-surface-1">
      <div className="border-b border-border/80 px-3 py-2">
        <div className="flex flex-wrap items-start gap-3">
          <div className="min-w-[min(100%,32rem)] flex-1">
            <h2 className="font-sans text-sm font-semibold">{title}</h2>
            <p className="mt-1 text-[11px] text-muted-foreground">
              {description}
            </p>
          </div>
          <div className="ml-auto flex flex-wrap items-center justify-end gap-2">
            {headerActions}
            <DisplayModeToggle
              value={displayMode}
              onChange={setDisplayMode}
            />
            <OutlierToggle
              checked={hideOutliers}
              onChange={setHideOutliers}
            />
            <ScaleToggle value={scaleMode} onChange={onScaleModeChange} />
          </div>
        </div>
        <ExchangeSelector
          selectedVenues={selectedVenues}
          venues={venues}
          onChange={onVenueSelectionChange}
        />
        <SeriesLegend series={displaySeries} hideOutliers={hideOutliers} />
      </div>
      <div className="h-[360px] px-2 py-3">
        {isLoading ? (
          <ChartLoadingState />
        ) : displaySeries.length === 0 || !domain ? (
          <div className="flex h-full items-center px-2 text-[11px] text-muted-foreground">
            {emptyMessage}
          </div>
        ) : (
          <ParentSize>
            {({ width, height }) => (
              <LatencyFrame
                domain={domain}
                height={height}
                displayMode={displayMode}
                scaleMode={scaleMode}
                series={displaySeries}
                valueLabel={valueLabel}
                width={width}
              />
            )}
          </ParentSize>
        )}
      </div>
    </section>
  )
}

function ChartLoadingState() {
  return (
    <div
      className="flex h-full flex-col justify-between px-2 py-1"
      aria-label="Loading latency chart"
      role="status"
    >
      <div className="flex items-center justify-between text-[11px] text-muted-foreground">
        <span>Loading latency data</span>
        <span>Fetching latest samples</span>
      </div>
      <div className="relative h-[280px] overflow-hidden rounded-sm border border-border/60 bg-surface-2/40">
        <div className="absolute inset-x-0 top-1/4 border-t border-dashed border-border/70" />
        <div className="absolute inset-x-0 top-1/2 border-t border-dashed border-border/70" />
        <div className="absolute inset-x-0 top-3/4 border-t border-dashed border-border/70" />
        <div className="absolute left-[8%] top-[30%] h-1.5 w-[68%] animate-pulse rounded-full bg-primary/25" />
        <div className="absolute left-[18%] top-[48%] h-1.5 w-[58%] animate-pulse rounded-full bg-profit/20 [animation-delay:120ms]" />
        <div className="absolute left-[28%] top-[64%] h-1.5 w-[50%] animate-pulse rounded-full bg-warning/20 [animation-delay:240ms]" />
      </div>
    </div>
  )
}

function LatencyFrame({
  domain,
  displayMode,
  height,
  scaleMode,
  series,
  valueLabel,
  width,
}: {
  domain: NonNullable<ReturnType<typeof getDomains>>
  displayMode: LatencyDisplayMode
  height: number
  scaleMode: LatencyScaleMode
  series: Array<DisplaySeries>
  valueLabel: string
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
  useDismissTooltipOnViewportChange(hideTooltip)
  useEffect(() => {
    hideTooltip()
  }, [displayMode, domain, height, hideTooltip, scaleMode, series, width])

  return (
    <div
      className="relative h-full w-full"
      onPointerLeave={hideTooltip}
      onMouseLeave={hideTooltip}
      onBlur={hideTooltip}
    >
      <LatencySvg
        domain={domain}
        displayMode={displayMode}
        height={height}
        hideTooltip={hideTooltip}
        hover={tooltipData}
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
          <PointTooltip hover={tooltipData} valueLabel={valueLabel} />
        </TooltipWithBounds>
      ) : null}
    </div>
  )
}

function LatencySvg({
  domain,
  displayMode,
  hideTooltip,
  height,
  hover,
  scaleMode,
  series,
  showTooltip,
  width,
}: {
  domain: NonNullable<ReturnType<typeof getDomains>>
  displayMode: LatencyDisplayMode
  hideTooltip: () => void
  height: number
  hover?: HoverPoint
  scaleMode: LatencyScaleMode
  series: Array<DisplaySeries>
  showTooltip: (args: {
    tooltipData: HoverPoint
    tooltipLeft?: number
    tooltipTop?: number
  }) => void
  width: number
}) {
  const rawClipId = useId()
  const clipId = rawClipId.replace(/:/g, "")

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
  const showNearestTooltip = (event: PointerEvent<SVGRectElement>) => {
    const bounds = event.currentTarget.getBoundingClientRect()
    const plotX = event.clientX - bounds.left
    const plotY = event.clientY - bounds.top
    const nearest = nearestHoverPoint(
      series,
      plotX,
      plotY,
      xScale,
      yScale
    )
    if (!nearest) {
      hideTooltip()
      return
    }
    showTooltip({
      tooltipData: {
        color: nearest.color,
        point: nearest.point,
        series: nearest.series,
      },
      tooltipLeft: MARGIN.left + nearest.x + 10,
      tooltipTop: MARGIN.top + nearest.y - 62,
    })
  }
  const activePoint = hover
    ? scaledHoverPoint(hover, xScale, yScale)
    : null

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
      <defs>
        <clipPath id={clipId}>
          <rect
            x={0}
            y={0}
            width={innerWidth}
            height={innerHeight}
          />
        </clipPath>
      </defs>
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
          tickFormat={(value) => formatLatency(Number(value))}
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
          numTicks={xTickCount}
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
        <Group clipPath={`url(#${clipId})`}>
          <rect
            width={innerWidth}
            height={innerHeight}
            fill="transparent"
            onPointerEnter={hideTooltip}
          />
          {series.map((item) => (
            <Group key={item.key}>
              {displayMode !== "raw" && item.bandPoints.length > 1 ? (
                <path
                  d={bandAreaPath(item.bandPoints, xScale, yScale)}
                  fill={item.color}
                  opacity={0.12}
                  pointerEvents="none"
                />
              ) : null}
              {displayMode === "trend-raw" && item.rawPoints.length > 0 ? (
                <path
                  d={pointMarkerPath(item.rawPoints, xScale, yScale, 1.8)}
                  fill={item.color}
                  opacity={0.28}
                  pointerEvents="none"
                />
              ) : null}
              {displayMode !== "raw" ? (
                <LinePath
                  data={item.linePoints}
                  x={(point) => xScale(point.date)}
                  y={(point) => yScale(point.ms)}
                  pointerEvents="none"
                  stroke={item.color}
                  strokeDasharray={item.strokeDasharray}
                  strokeWidth={1.7}
                />
              ) : null}
              {displayMode !== "trend-raw" && item.linePoints.length > 0 ? (
                <path
                  d={pointMarkerPath(item.linePoints, xScale, yScale, 2.4)}
                  fill={item.color}
                  pointerEvents="none"
                />
              ) : null}
            </Group>
          ))}
          <rect
            width={innerWidth}
            height={innerHeight}
            fill="transparent"
            onPointerEnter={showNearestTooltip}
            onPointerMove={showNearestTooltip}
            onPointerLeave={hideTooltip}
            onPointerCancel={hideTooltip}
          />
          {activePoint ? (
            <circle
              cx={activePoint.x}
              cy={activePoint.y}
              r={4}
              fill={activePoint.color}
              stroke="var(--chart-point-stroke)"
              strokeWidth={1.5}
              pointerEvents="none"
            />
          ) : null}
        </Group>
      </Group>
    </svg>
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

function PointTooltip({
  hover,
  valueLabel,
}: {
  hover: HoverPoint
  valueLabel: string
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
          {hover.point.kind === "rolling-median" ? TREND_LABEL : valueLabel}
        </span>
        <span className="-mx-1 rounded-sm bg-surface-2 px-1 py-0.5 font-mono font-bold text-foreground">
          {formatLatency(hover.point.ms)}
        </span>
        {hover.point.sampleCount ? (
          <>
            <span className="text-muted-foreground">Window</span>
            <span className="font-mono text-muted-foreground">
              {hover.point.sampleCount} samples
            </span>
          </>
        ) : null}
      </div>
    </>
  )
}

function DisplayModeToggle({
  value,
  onChange,
}: {
  value: LatencyDisplayMode
  onChange: (value: LatencyDisplayMode) => void
}) {
  const modes: Array<{ label: string; value: LatencyDisplayMode }> = [
    { label: "Raw", value: "raw" },
    { label: "Trend", value: "trend" },
    { label: "Trend + raw", value: "trend-raw" },
  ]

  return (
    <div className="flex h-8 overflow-hidden rounded-sm border border-border bg-surface-1 text-[11px]">
      {modes.map((mode) => (
        <button
          key={mode.value}
          type="button"
          onClick={() => onChange(mode.value)}
          className={`px-2.5 ${
            value === mode.value
              ? "bg-primary/15 text-foreground ring-1 ring-inset ring-primary/40"
              : "text-muted-foreground hover:bg-surface-2 hover:text-foreground"
          }`}
          aria-pressed={value === mode.value}
        >
          {mode.label}
        </button>
      ))}
    </div>
  )
}

function OutlierToggle({
  checked,
  onChange,
}: {
  checked: boolean
  onChange: (checked: boolean) => void
}) {
  return (
    <label className="flex h-8 cursor-pointer items-center gap-2 rounded-sm border border-border bg-surface-1 px-2 text-[11px] text-muted-foreground hover:bg-surface-2 hover:text-foreground">
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

function SeriesLegend({
  hideOutliers,
  series,
}: {
  hideOutliers: boolean
  series: Array<DisplaySeries>
}) {
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
          <VenueName
            className="max-w-[240px]"
            label={item.label}
            venue={item.venue}
          />
          <span className="font-mono text-[9px] text-muted-foreground/80">
            {item.rawPoints.length || item.linePoints.length} pts
          </span>
          {hideOutliers && item.outlierCount > 0 ? (
            <span className="font-mono text-[9px] text-muted-foreground/80">
              {item.outlierCount} hidden
            </span>
          ) : null}
        </div>
      ))}
    </div>
  )
}

function pointMarkerPath(
  points: Array<Point>,
  xScale: DateScale,
  yScale: NumberScale,
  radius: number
) {
  return points
    .map((point) => {
      const x = xScale(point.date)
      const y = yScale(point.ms)
      if (!Number.isFinite(x) || !Number.isFinite(y)) {
        return ""
      }
      const cx = formatSVGNumber(x)
      const cy = formatSVGNumber(y)
      const r = formatSVGNumber(radius)
      const diameter = formatSVGNumber(radius * 2)
      return `M${cx},${cy}m-${r},0a${r},${r} 0 1,0 ${diameter},0a${r},${r} 0 1,0 -${diameter},0`
    })
    .join("")
}

function bandAreaPath(
  points: Array<BandPoint>,
  xScale: DateScale,
  yScale: NumberScale
) {
  const top = points
    .map((point) => scaledBandCoordinate(point.date, point.upperMS, xScale, yScale))
    .filter((point): point is [number, number] => point !== null)
  const bottom = points
    .map((point) => scaledBandCoordinate(point.date, point.lowerMS, xScale, yScale))
    .filter((point): point is [number, number] => point !== null)
    .reverse()
  const coordinates = [...top, ...bottom]

  if (coordinates.length < 3) {
    return ""
  }

  return coordinates
    .map(([x, y], index) =>
      `${index === 0 ? "M" : "L"}${formatSVGNumber(x)},${formatSVGNumber(y)}`
    )
    .join("") + "Z"
}

function scaledBandCoordinate(
  date: Date,
  ms: number,
  xScale: DateScale,
  yScale: NumberScale
): [number, number] | null {
  const x = xScale(date)
  const y = yScale(ms)
  if (!Number.isFinite(x) || !Number.isFinite(y)) {
    return null
  }
  return [x, y]
}

function nearestHoverPoint(
  series: Array<DisplaySeries>,
  plotX: number,
  plotY: number,
  xScale: DateScale,
  yScale: NumberScale
): ScaledHoverPoint | null {
  let nearest: ScaledHoverPoint | null = null
  let nearestDistance = TOOLTIP_HIT_RADIUS_PX * TOOLTIP_HIT_RADIUS_PX

  const considerPoint = (item: DisplaySeries, point: Point) => {
    const x = xScale(point.date)
    const y = yScale(point.ms)
    if (!Number.isFinite(x) || !Number.isFinite(y)) {
      return
    }
    const dx = x - plotX
    const dy = y - plotY
    const distance = dx * dx + dy * dy
    if (distance <= nearestDistance) {
      nearestDistance = distance
      nearest = {
        color: item.color,
        point,
        series: item,
        x,
        y,
      }
    }
  }

  for (const item of series) {
    for (const point of item.rawPoints) {
      considerPoint(item, point)
    }
    for (const point of item.linePoints) {
      considerPoint(item, point)
    }
  }

  return nearest
}

function scaledHoverPoint(
  hover: HoverPoint,
  xScale: DateScale,
  yScale: NumberScale
): ScaledHoverPoint | null {
  const x = xScale(hover.point.date)
  const y = yScale(hover.point.ms)
  if (!Number.isFinite(x) || !Number.isFinite(y)) {
    return null
  }
  return { ...hover, x, y }
}

function formatSVGNumber(value: number) {
  return value.toFixed(2)
}

function buildSeries(
  samples: Array<Sample>,
  valueForSample: (sample: Sample) => number | undefined
): Array<Series> {
  const grouped = new Map<string, Array<Point>>()

  for (const sample of samples) {
    if (!sample.ok || sample.warmup || sample.network_ns <= 0) {
      continue
    }

    const date = samplePlotDate(sample)
    if (!date) {
      continue
    }

    const ms = valueForSample(sample)
    if (ms === undefined || !Number.isFinite(ms) || ms <= 0) {
      continue
    }

    const batchKind = batchSubmission(sample)
    const key = batchKind ? `${sample.venue}:${batchKind}` : sample.venue
    const points = grouped.get(key) ?? []
    points.push({
      date,
      kind: "raw",
      ms,
    })
    grouped.set(key, points)
  }

  return [...grouped.entries()]
    .map(([key, points]) => {
      const [venue = "unknown", batchKind = ""] = key.split(":")

      return {
        color: colorForVenue(venue),
        key,
        label: seriesLabel(venue, batchKind),
        points: points.sort((a, b) => a.date.getTime() - b.date.getTime()),
        strokeDasharray: batchKind === "manual" ? "5 4" : undefined,
        venue,
      }
    })
    .filter((item) => item.points.length > 0)
    .sort((left, right) => left.label.localeCompare(right.label))
}

function batchSubmission(sample: Sample) {
  if (sample.scenario !== "batch") {
    return ""
  }
  if (sample.metadata?.native_batch_endpoint === false) {
    return "manual"
  }
  if (sample.metadata?.native_batch_endpoint === true) {
    return "native"
  }
  if (typeof sample.metadata?.submission_model === "string") {
    return "manual"
  }
  return "native"
}

function seriesLabel(venue: string, batchKind: string) {
  const label = formatVenueLabel(venue)
  if (batchKind === "manual") {
    return `${label} (manual)`
  }
  if (batchKind === "native") {
    return `${label} (native)`
  }
  return label
}

function buildDisplaySeries(
  series: Array<Series>,
  {
    displayMode,
    hideOutliers,
  }: {
    displayMode: LatencyDisplayMode
    hideOutliers: boolean
  }
): Array<DisplaySeries> {
  return series
    .map((item) => {
      const { outlierCount, points: visibleRawPoints } = hideOutliers
        ? withoutUpperOutliers(item.points)
        : { outlierCount: 0, points: item.points }
      const trend = rollingTrend(visibleRawPoints)

      return {
        ...item,
        bandPoints: displayMode === "raw" ? [] : trend.bandPoints,
        linePoints: displayMode === "raw" ? visibleRawPoints : trend.linePoints,
        outlierCount,
        rawPoints: displayMode === "trend-raw" ? visibleRawPoints : [],
      }
    })
    .filter((item) => item.linePoints.length > 0 || item.rawPoints.length > 0)
}

function rollingTrend(points: Array<Point>): {
  bandPoints: Array<BandPoint>
  linePoints: Array<Point>
} {
  if (points.length === 0) {
    return { bandPoints: [], linePoints: [] }
  }

  const startTime = floorToStep(points[0].date.getTime(), TREND_STEP_MS)
  const endTime = points[points.length - 1].date.getTime()
  const halfWindow = TREND_WINDOW_MS / 2
  const bandPoints: Array<BandPoint> = []
  const linePoints: Array<Point> = []
  let startIndex = 0
  let endIndex = 0

  for (let time = startTime; time <= endTime; time += TREND_STEP_MS) {
    const minTime = time - halfWindow
    const maxTime = time + halfWindow

    while (
      startIndex < points.length &&
      points[startIndex].date.getTime() < minTime
    ) {
      startIndex += 1
    }
    while (
      endIndex < points.length &&
      points[endIndex].date.getTime() <= maxTime
    ) {
      endIndex += 1
    }

    const values = points
      .slice(startIndex, endIndex)
      .map((point) => point.ms)
      .sort((left, right) => left - right)

    if (values.length === 0) {
      continue
    }

    const date = new Date(time)
    linePoints.push({
      date,
      kind: "rolling-median",
      ms: quantile(values, 0.5),
      sampleCount: values.length,
    })
    bandPoints.push({
      date,
      lowerMS: quantile(values, 0.25),
      upperMS: quantile(values, 0.75),
    })
  }

  return { bandPoints, linePoints }
}

function floorToStep(value: number, step: number) {
  return Math.floor(value / step) * step
}

function withoutUpperOutliers(points: Array<Point>) {
  if (points.length < 8) {
    return { outlierCount: 0, points }
  }

  const values = points.map((point) => point.ms).sort((a, b) => a - b)
  const q1 = quantile(values, 0.25)
  const q3 = quantile(values, 0.75)
  const iqr = q3 - q1
  const upperFence = iqr > 0 ? q3 + 1.5 * iqr : q3 * 3
  const filtered = points.filter((point) => point.ms <= upperFence)

  return {
    outlierCount: points.length - filtered.length,
    points: filtered,
  }
}

function quantile(sortedValues: Array<number>, q: number) {
  if (sortedValues.length === 0) {
    return Number.NaN
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

function getDomains(series: Array<DisplaySeries>, scaleMode: LatencyScaleMode) {
  const points = series.flatMap((item) => [
    ...item.linePoints,
    ...item.rawPoints,
    ...item.bandPoints.flatMap((point) => [
      { date: point.date, kind: "rolling-median" as const, ms: point.lowerMS },
      { date: point.date, kind: "rolling-median" as const, ms: point.upperMS },
    ]),
  ])

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
    const hasRoom = selected.every(
      (existing) => Math.abs(position(existing) - y) >= minGap
    )
    if (hasRoom) {
      selected.push(value)
    }
  }

  return selected.sort((left, right) => left - right)
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
  nado: "oklch(0.55 0.18 305)",
  variational_omni: "oklch(0.62 0.15 75)",
}
