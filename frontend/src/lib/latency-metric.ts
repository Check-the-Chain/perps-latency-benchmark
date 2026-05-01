import type { Sample, SummaryRow } from "@/api/bench"
import { nsToMs } from "@/lib/format"

export function confirmP50(row: SummaryRow) {
  return isHyperliquid(row.venue) ? row.submission_p50_ms : row.p50_ms
}

export function confirmP95(row: SummaryRow) {
  return isHyperliquid(row.venue) ? row.submission_p95_ms : row.p95_ms
}

export function confirmSampleMs(sample: Sample) {
  if (isHyperliquid(sample.venue) && sample.submission_ns && sample.submission_ns > 0) {
    return nsToMs(sample.submission_ns)
  }
  return nsToMs(sample.network_ns)
}

export function secondarySampleMs(sample: Sample) {
  if (isHyperliquid(sample.venue)) {
    return sample.network_ns > 0 ? nsToMs(sample.network_ns) : undefined
  }
  return sample.submission_ns && sample.submission_ns > 0
    ? nsToMs(sample.submission_ns)
    : undefined
}

export function secondaryLabel(venue: string) {
  return isHyperliquid(venue) ? "Private WS update" : "Ack latency"
}

function isHyperliquid(venue: string) {
  return venue.toLowerCase() === "hyperliquid"
}
