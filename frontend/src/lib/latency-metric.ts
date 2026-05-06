import type { Sample, SummaryRow } from "@/api/bench"
import { nsToMs } from "@/lib/format"

export function confirmP50(row: SummaryRow) {
  return row.p50_ms
}

export function confirmP95(row: SummaryRow) {
  return row.p95_ms
}

export function confirmSampleMs(sample: Sample) {
  return nsToMs(adjustedNetworkNS(sample))
}

export function rawConfirmSampleMs(sample: Sample) {
  return nsToMs(rawNetworkNS(sample))
}

export function speedBumpSampleMs(sample: Sample) {
  return sample.speed_bump_ns && sample.speed_bump_ns > 0
    ? nsToMs(sample.speed_bump_ns)
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

function adjustedNetworkNS(sample: Sample) {
  if (sample.adjusted_network_ns && sample.adjusted_network_ns > 0) {
    return sample.adjusted_network_ns
  }
  const adjusted = rawNetworkNS(sample) - (sample.speed_bump_ns ?? 0)
  return Math.max(adjusted, 0)
}

function rawNetworkNS(sample: Sample) {
  return sample.raw_network_ns && sample.raw_network_ns > 0
    ? sample.raw_network_ns
    : sample.network_ns
}
