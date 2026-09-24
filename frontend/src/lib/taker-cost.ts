import type { ExpectedFill, Sample, SampleCost } from "@/api/bench"
import { samplePlotDate } from "@/lib/sample-time"

export interface TakerCostRecord {
  actualCostGapUSD?: number
  balanceDiffUSD?: number
  clean: boolean
  cost: SampleCost
  date: Date
  entryActualPrice?: number
  entryExpectedPrice?: number
  entryFeeUSD: number
  entrySlippageBps?: number
  entrySlippageUSD?: number
  exitActualPrice?: number
  exitExpectedPrice?: number
  exitFeeUSD: number
  exitSlippageBps?: number
  exitSlippageUSD?: number
  expectedRoundTripCostUSD?: number
  priceMoveCostUSD: number
  qualityReason?: string
  sample: Sample
  totalSlippageBps?: number
  totalSlippageUSD?: number
  tradeCostUSD: number
  venue: string
}

export interface StableCostWindow {
  dirtyCount: number
  end?: Date
  requiredVenues: Array<string>
  records: Array<TakerCostRecord>
  start?: Date
}

export interface VenueSlippageSummary {
  cleanCount: number
  costGapMeanUSD?: number
  entryMeanBps?: number
  exitMeanBps?: number
  feeMeanPerFillUSD: number
  meanCostUSD: number
  meanFillCostUSD: number
  priceMoveMeanUSD: number
  sampleCount: number
  totalCostUSD: number
  totalSlippageMeanBps?: number
  totalSlippageP95Bps?: number
  venue: string
}

export function buildTakerCostRecords(samples: Array<Sample>) {
  return samples
    .map((sample) => toTakerCostRecord(sample))
    .filter((record): record is TakerCostRecord => record !== null)
    .sort((left, right) => left.date.getTime() - right.date.getTime())
}

export function stableCostWindow(records: Array<TakerCostRecord>): StableCostWindow {
  const requiredVenues = uniqueSorted(records.map((record) => record.venue))
  const stableStart = firstCompleteCleanCycle(records, requiredVenues)
  const stableRecords = records.filter(
    (record) => record.clean && (!stableStart || record.date >= stableStart)
  )

  return {
    dirtyCount: records.filter((record) => !record.clean).length,
    end: stableRecords.at(-1)?.date,
    requiredVenues,
    records: stableRecords,
    start: stableRecords[0]?.date,
  }
}

export function summarizeSlippage(
  records: Array<TakerCostRecord>
): Array<VenueSlippageSummary> {
  const groups = new Map<string, Array<TakerCostRecord>>()
  for (const record of records) {
    groups.set(record.venue, [...(groups.get(record.venue) ?? []), record])
  }

  return [...groups.entries()]
    .map(([venue, venueRecords]) => {
      const cleanRecords = venueRecords.filter((record) => record.clean)
      const totalCostUSD = sum(cleanRecords.map((record) => record.tradeCostUSD))
      const totalFees = sum(
        cleanRecords.map((record) => record.entryFeeUSD + record.exitFeeUSD)
      )
      return {
        cleanCount: cleanRecords.length,
        costGapMeanUSD: mean(values(cleanRecords, (record) => record.actualCostGapUSD)),
        entryMeanBps: mean(values(cleanRecords, (record) => record.entrySlippageBps)),
        exitMeanBps: mean(values(cleanRecords, (record) => record.exitSlippageBps)),
        feeMeanPerFillUSD:
          cleanRecords.length > 0 ? totalFees / (cleanRecords.length * 2) : 0,
        meanCostUSD: cleanRecords.length > 0 ? totalCostUSD / cleanRecords.length : 0,
        meanFillCostUSD:
          cleanRecords.length > 0 ? totalCostUSD / (cleanRecords.length * 2) : 0,
        priceMoveMeanUSD: mean(values(cleanRecords, (record) => record.priceMoveCostUSD)) ?? 0,
        sampleCount: venueRecords.length,
        totalCostUSD,
        totalSlippageMeanBps: mean(values(cleanRecords, (record) => record.totalSlippageBps)),
        totalSlippageP95Bps: percentile(
          values(cleanRecords, (record) => record.totalSlippageBps).map(Math.abs),
          0.95
        ),
        venue,
      }
    })
    .sort((left, right) => {
      const leftScore = left.totalSlippageP95Bps ?? -Infinity
      const rightScore = right.totalSlippageP95Bps ?? -Infinity
      return rightScore - leftScore
    })
}

export function cumulativeVenueSeries(records: Array<TakerCostRecord>) {
  const running = new Map<string, number>()
  return records.map((record) => {
    const next = (running.get(record.venue) ?? 0) + record.tradeCostUSD
    running.set(record.venue, next)
    return {
      date: record.date,
      record,
      value: next,
      venue: record.venue,
    }
  })
}

export function isTakerOrder(value: string | undefined) {
  const normalized = (value && value.length > 0 ? value : "unknown").toLowerCase()
  return ["market", "ioc", "immediate_or_cancel", "fok", "fill_or_kill"].includes(normalized)
}

function toTakerCostRecord(sample: Sample): TakerCostRecord | null {
  if (!isTakerOrder(sample.order_type) || sample.warmup || !sample.cost) {
    return null
  }

  const date = samplePlotDate(sample)
  if (!date) {
    return null
  }

  const cost = sample.cost
  const entryActualPrice = averagePrice(cost.entry_value_usd, cost.entry_qty)
  const exitActualPrice = averagePrice(cost.exit_value_usd, cost.exit_qty)
  const entryExpectedPrice = usableExpectedPrice(sample.expected_entry_fill)
  const exitExpectedPrice = usableExpectedPrice(sample.expected_exit_fill)
  const entrySlippageUSD = fillSlippageUSD(
    sample.expected_entry_fill,
    entryActualPrice,
    cost.entry_qty
  )
  const exitSlippageUSD = fillSlippageUSD(
    sample.expected_exit_fill,
    exitActualPrice,
    cost.exit_qty
  )
  const totalSlippageUSD =
    entrySlippageUSD !== undefined && exitSlippageUSD !== undefined
      ? entrySlippageUSD + exitSlippageUSD
      : undefined
  const expectedRoundTripCostUSD =
    entryExpectedPrice !== undefined &&
    exitExpectedPrice !== undefined &&
    cost.entry_qty !== undefined &&
    cost.exit_qty !== undefined
      ? expectedLegCost(sample.expected_entry_fill, entryExpectedPrice, cost.entry_qty) +
        expectedLegCost(sample.expected_exit_fill, exitExpectedPrice, cost.exit_qty) +
        (cost.entry_fee_usd ?? 0) +
        (cost.exit_fee_usd ?? 0)
      : undefined

  return {
    actualCostGapUSD:
      expectedRoundTripCostUSD !== undefined && cost.trade_cost_usd !== undefined
        ? cost.trade_cost_usd - expectedRoundTripCostUSD
        : undefined,
    balanceDiffUSD: cost.reconciliation_diff_usd,
    clean: cost.clean,
    cost,
    date,
    entryActualPrice,
    entryExpectedPrice,
    entryFeeUSD: cost.entry_fee_usd ?? 0,
    entrySlippageBps: fillSlippageBps(sample.expected_entry_fill, entryActualPrice),
    entrySlippageUSD,
    exitActualPrice,
    exitExpectedPrice,
    exitFeeUSD: cost.exit_fee_usd ?? 0,
    exitSlippageBps: fillSlippageBps(sample.expected_exit_fill, exitActualPrice),
    exitSlippageUSD,
    expectedRoundTripCostUSD,
    priceMoveCostUSD: cost.price_move_cost_usd ?? 0,
    qualityReason: cost.quality_reason,
    sample,
    totalSlippageBps:
      entrySlippageUSD !== undefined &&
      exitSlippageUSD !== undefined &&
      cost.entry_value_usd !== undefined &&
      cost.exit_value_usd !== undefined
        ? ((entrySlippageUSD + exitSlippageUSD) /
            Math.max((cost.entry_value_usd + cost.exit_value_usd) / 2, 1)) *
          10_000
        : undefined,
    totalSlippageUSD,
    tradeCostUSD: cost.trade_cost_usd ?? 0,
    venue: sample.venue,
  }
}

function averagePrice(valueUSD?: number, qty?: number) {
  if (
    valueUSD === undefined ||
    qty === undefined ||
    !Number.isFinite(valueUSD) ||
    !Number.isFinite(qty) ||
    qty <= 0
  ) {
    return undefined
  }

  return valueUSD / qty
}

function fillSlippageBps(fill: ExpectedFill | undefined, actualPrice?: number) {
  const expected = usableExpectedPrice(fill)
  if (
    actualPrice === undefined ||
    expected === undefined ||
    expected <= 0
  ) {
    return undefined
  }

  const side = (fill?.side ?? "buy").toLowerCase()
  const adverseMove = side === "sell" ? expected - actualPrice : actualPrice - expected
  return (adverseMove / expected) * 10_000
}

function fillSlippageUSD(
  fill: ExpectedFill | undefined,
  actualPrice?: number,
  qty?: number
) {
  const expected = usableExpectedPrice(fill)
  if (
    actualPrice === undefined ||
    qty === undefined ||
    expected === undefined ||
    !Number.isFinite(qty)
  ) {
    return undefined
  }

  const side = (fill?.side ?? "buy").toLowerCase()
  return side === "sell"
    ? (expected - actualPrice) * qty
    : (actualPrice - expected) * qty
}

function usableExpectedPrice(fill: ExpectedFill | undefined) {
  if (!fill || fill.expected_price === undefined || fill.expected_price <= 0) {
    return undefined
  }
  if (fill.book_sufficient === false) {
    return undefined
  }
  if (fill.book_sufficient === undefined && fill.top_sufficient === false) {
    return undefined
  }
  return fill.expected_price
}

function expectedLegCost(
  fill: ExpectedFill | undefined,
  expectedPrice: number,
  qty: number
) {
  const side = (fill?.side ?? "buy").toLowerCase()
  return side === "sell" ? -expectedPrice * qty : expectedPrice * qty
}

function values<T>(records: Array<T>, getValue: (record: T) => number | undefined) {
  return records
    .map(getValue)
    .filter((value): value is number => value !== undefined && Number.isFinite(value))
}

function sum(valuesToSum: Array<number>) {
  return valuesToSum.reduce((total, value) => total + value, 0)
}

function mean(valuesToAverage: Array<number>) {
  if (valuesToAverage.length === 0) {
    return undefined
  }
  return sum(valuesToAverage) / valuesToAverage.length
}

function percentile(valuesToRank: Array<number>, percentileValue: number) {
  if (valuesToRank.length === 0) {
    return undefined
  }

  const sorted = [...valuesToRank].sort((left, right) => left - right)
  const index = Math.min(
    sorted.length - 1,
    Math.max(0, Math.ceil(sorted.length * percentileValue) - 1)
  )
  return sorted[index]
}

function firstCompleteCleanCycle(
  records: Array<TakerCostRecord>,
  requiredVenues: Array<string>
) {
  if (requiredVenues.length === 0) {
    return undefined
  }

  const grouped = new Map<string, Array<TakerCostRecord>>()
  for (const record of records) {
    const key = cycleKey(record.date)
    grouped.set(key, [...(grouped.get(key) ?? []), record])
  }

  for (const [key, cycleRecords] of [...grouped.entries()].sort(([left], [right]) =>
    left.localeCompare(right)
  )) {
    const cleanVenues = new Set(
      cycleRecords
        .filter((record) => record.clean)
        .map((record) => record.venue)
    )
    if (requiredVenues.every((venue) => cleanVenues.has(venue))) {
      return new Date(key)
    }
  }

  return undefined
}

function cycleKey(date: Date) {
  const rounded = new Date(date)
  rounded.setUTCSeconds(0, 0)
  return rounded.toISOString()
}

function uniqueSorted(valuesToSort: Array<string>) {
  return [...new Set(valuesToSort)].sort((left, right) => left.localeCompare(right))
}
