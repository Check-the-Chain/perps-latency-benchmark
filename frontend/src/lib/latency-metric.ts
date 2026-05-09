import type { Sample, SummaryRow } from "@/api/bench"
import { nsToMs } from "@/lib/format"

const EXTENDED_SPEED_BUMP_NS = 150_000_000
const EXTENDED_SPEED_BUMP_MS = 150

export function confirmP50(row: SummaryRow, subtractNetworkFloor = false) {
  return summaryLatency(
    row,
    row.p50_ms,
    row.raw_p50_ms,
    row.network_adjusted_p50_ms,
    subtractNetworkFloor
  )
}

export function confirmP95(row: SummaryRow, subtractNetworkFloor = false) {
  return summaryLatency(
    row,
    row.p95_ms,
    row.raw_p95_ms,
    row.network_adjusted_p95_ms,
    subtractNetworkFloor
  )
}

export function confirmSampleMs(sample: Sample, subtractNetworkFloor = false) {
  if (sample.confirm_ns && sample.confirm_ns > 0) {
    return nsToMs(
      subtractNetworkFloor
        ? subtractNetworkFloorNS(sample.confirm_ns, sample)
        : sample.confirm_ns
    )
  }
  return nsToMs(
    subtractNetworkFloor
      ? subtractNetworkFloorNS(adjustedNetworkNS(sample), sample)
      : adjustedNetworkNS(sample)
  )
}

export function cancelSampleMs(sample: Sample, subtractNetworkFloor = false) {
  if (
    sample.cleanup_account_feed &&
    sample.cleanup_confirm_ns &&
    sample.cleanup_confirm_ns > 0
  ) {
    return nsToMs(
      subtractNetworkFloor
        ? subtractNetworkFloorNS(sample.cleanup_confirm_ns, sample)
        : sample.cleanup_confirm_ns
    )
  }
  const durationNS = sample.cleanup?.duration_ns
  return isAccountFeedCancelCleanup(sample) && durationNS && durationNS > 0
    ? nsToMs(
        subtractNetworkFloor
          ? subtractNetworkFloorNS(durationNS, sample)
          : durationNS
      )
    : undefined
}

export function cancelP50(row: SummaryRow, subtractNetworkFloor = false) {
  if (subtractNetworkFloor) {
    return positiveMetric(row.network_adjusted_cleanup_p50_ms) ?? positiveMetric(row.cleanup_p50_ms)
  }
  return positiveMetric(row.cleanup_p50_ms)
}

export function cancelP95(row: SummaryRow, subtractNetworkFloor = false) {
  if (subtractNetworkFloor) {
    return positiveMetric(row.network_adjusted_cleanup_p95_ms) ?? positiveMetric(row.cleanup_p95_ms)
  }
  return positiveMetric(row.cleanup_p95_ms)
}

export function isCancelCleanup(sample: Sample) {
  return Boolean(
    sample.cleanup?.ok &&
      sample.cleanup.description?.toLowerCase().includes("cancel")
  )
}

export function isAccountFeedCancelCleanup(sample: Sample) {
  if (sample.cleanup_account_feed) {
    return true
  }
  const confirmation =
    sample.cleanup?.cleanup_confirmation ??
    sample.cleanup?.metadata?.cleanup_confirmation
  return Boolean(
    isCancelCleanup(sample) &&
      confirmation === "account_feed"
  )
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

export function primaryLabel(_venue: string) {
  return "Account feed"
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

function summaryLatency(
  row: SummaryRow,
  value: number,
  rawValue: number | undefined,
  networkAdjustedValue: number | undefined,
  subtractNetworkFloor: boolean
) {
  if (subtractNetworkFloor && networkAdjustedValue && networkAdjustedValue > 0) {
    return networkAdjustedValue
  }
  return adjustedSummaryLatency(value, row, rawValue)
}

function subtractNetworkFloorNS(value: number, sample: Sample) {
  const floor = sample.network_floor_ns
  if (!floor || floor <= 0) {
    return value
  }
  return Math.max(value - floor, 0)
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
    : sample.network_ns ?? 0
}
