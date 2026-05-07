import type { Sample, SummaryRow } from "@/api/bench"
import { nsToMs } from "@/lib/format"

const EXTENDED_SPEED_BUMP_NS = 150_000_000
const EXTENDED_SPEED_BUMP_MS = 150

export function confirmP50(row: SummaryRow) {
  return adjustedSummaryLatency(row.p50_ms, row, row.raw_p50_ms)
}

export function confirmP95(row: SummaryRow) {
  return adjustedSummaryLatency(row.p95_ms, row, row.raw_p95_ms)
}

export function confirmSampleMs(sample: Sample) {
  return nsToMs(adjustedNetworkNS(sample))
}

export function cancelSampleMs(sample: Sample) {
  const durationNS = sample.cleanup?.duration_ns
  return isCancelCleanup(sample) && durationNS && durationNS > 0
    ? nsToMs(durationNS)
    : undefined
}

export function cancelP50(row: SummaryRow) {
  return positiveMetric(row.cleanup_p50_ms)
}

export function cancelP95(row: SummaryRow) {
  return positiveMetric(row.cleanup_p95_ms)
}

export function isCancelCleanup(sample: Sample) {
  return Boolean(
    sample.cleanup?.ok &&
      sample.cleanup.description?.toLowerCase().includes("cancel")
  )
}

export function rawConfirmSampleMs(sample: Sample) {
  return nsToMs(rawNetworkNS(sample))
}

export function summarySpeedBumpMS(row: SummaryRow) {
  if (
    row.speed_bump_ms &&
    row.speed_bump_ms > 0 &&
    !isExtendedNonTaker(row.venue, row.order_type)
  ) {
    return row.speed_bump_ms
  }
  return isExtendedTaker(row.venue, row.order_type) ? EXTENDED_SPEED_BUMP_MS : undefined
}

export function speedBumpSampleMs(sample: Sample) {
  const speedBumpNS = effectiveSpeedBumpNS(sample)
  return speedBumpNS > 0
    ? nsToMs(speedBumpNS)
    : undefined
}

export function secondarySampleMs(sample: Sample) {
  return sample.measurement_mode === "ws_confirmation" && sample.submission_ns && sample.submission_ns > 0
    ? nsToMs(sample.submission_ns)
    : undefined
}

export function primaryLabel(_venue: string) {
  return "Account feed"
}

export function secondaryLabel(venue: string) {
  return isHyperliquid(venue) ? "Exchange response" : "Submit ack"
}

function isHyperliquid(venue: string) {
  return venue.toLowerCase() === "hyperliquid"
}

function adjustedSummaryLatency(
  value: number,
  row: SummaryRow,
  rawValue?: number
) {
  if (
    isExtendedTaker(row.venue, row.order_type) &&
    !(row.speed_bump_ms && row.speed_bump_ms > 0)
  ) {
    const sourceValue = Number.isFinite(rawValue) ? Number(rawValue) : value
    return Math.max(sourceValue - EXTENDED_SPEED_BUMP_MS, 0)
  }
  return value
}

function positiveMetric(value?: number) {
  return value && Number.isFinite(value) && value > 0 ? value : undefined
}

function adjustedNetworkNS(sample: Sample) {
  const speedBumpNS = effectiveSpeedBumpNS(sample)
  if (
    sample.adjusted_network_ns &&
    sample.adjusted_network_ns > 0 &&
    speedBumpNS === (sample.speed_bump_ns ?? 0)
  ) {
    return sample.adjusted_network_ns
  }
  const adjusted = rawNetworkNS(sample) - speedBumpNS
  return Math.max(adjusted, 0)
}

function effectiveSpeedBumpNS(sample: Sample) {
  if (
    sample.speed_bump_ns &&
    sample.speed_bump_ns > 0 &&
    !isExtendedNonTaker(sample.venue, sample.order_type)
  ) {
    return sample.speed_bump_ns
  }
  return isExtendedTaker(sample.venue, sample.order_type) ? EXTENDED_SPEED_BUMP_NS : 0
}

function isExtendedTaker(venue: string, orderType?: string) {
  return venue.toLowerCase() === "extended" && isTakerOrderType(orderType)
}

function isExtendedNonTaker(venue: string, orderType?: string) {
  return venue.toLowerCase() === "extended" && !isTakerOrderType(orderType)
}

function isTakerOrderType(orderType?: string) {
  const normalized = orderType?.toLowerCase().trim()
  return normalized === "market" || normalized === "ioc"
}

function rawNetworkNS(sample: Sample) {
  return sample.raw_network_ns && sample.raw_network_ns > 0
    ? sample.raw_network_ns
    : sample.network_ns
}
