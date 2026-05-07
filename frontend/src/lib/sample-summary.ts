import type { Sample, SummaryRow } from "@/api/bench"
import {
  cancelSampleMs,
  confirmSampleMs,
  rawConfirmSampleMs,
  speedBumpSampleMs,
} from "@/lib/latency-metric"

export function buildSummaryRows(samples: Array<Sample>): Array<SummaryRow> {
  const groups = new Map<string, Array<Sample>>()

  for (const sample of samples) {
    const key = [
      sample.venue,
      sample.scenario,
      sample.order_type ?? "",
      sample.batch_size,
      sample.measurement_mode ?? "",
    ].join("\x00")
    groups.set(key, [...(groups.get(key) ?? []), sample])
  }

  return [...groups.values()]
    .map(summaryForSamples)
    .filter((row): row is SummaryRow => Boolean(row))
    .sort(compareSummaryRows)
}

function summaryForSamples(samples: Array<Sample>): SummaryRow | null {
  const included = samples.filter((sample) => !sample.warmup)
  if (included.length === 0) {
    return null
  }

  const first = included[0]
  const okSamples = included.filter((sample) => sample.ok)
  const adjustedValues = finiteValues(okSamples.map(confirmSampleMs))
  const rawValues = finiteValues(okSamples.map(rawConfirmSampleMs))
  const submissionValues = finiteValues(
    okSamples.map((sample) =>
      sample.submission_ns && sample.submission_ns > 0
        ? sample.submission_ns / 1_000_000
        : undefined
    )
  )
  const speedBumps = okSamples.map((sample) => speedBumpSampleMs(sample) ?? 0)
  const cancelValues = finiteValues(included.map(cancelSampleMs))
  const cleanupAttempted = included.filter((sample) => sample.cleanup?.attempted)
  const cleanCosts = included
    .map((sample) => sample.cost)
    .filter((cost) => cost?.clean && Number.isFinite(cost.trade_cost_usd))

  return {
    venue: first.venue,
    transport: groupTransport(included),
    scenario: first.scenario,
    order_type: first.order_type ?? "",
    measurement_mode: first.measurement_mode,
    batch_size: first.batch_size,
    count: included.length,
    ok: okSamples.length,
    failed: included.length - okSamples.length,
    mean_ms: mean(adjustedValues),
    p50_ms: percentile(adjustedValues, 50),
    p95_ms: percentile(adjustedValues, 95),
    p99_ms: percentile(adjustedValues, 99),
    raw_mean_ms: mean(rawValues),
    raw_p50_ms: percentile(rawValues, 50),
    raw_p95_ms: percentile(rawValues, 95),
    raw_p99_ms: percentile(rawValues, 99),
    speed_bump_ms: mean(speedBumps),
    speed_bump_source: first.speed_bump_source,
    submission_p50_ms: percentile(submissionValues, 50),
    submission_p95_ms: percentile(submissionValues, 95),
    submission_p99_ms: percentile(submissionValues, 99),
    cleanup_mean_ms: mean(cancelValues),
    cleanup_p50_ms: percentile(cancelValues, 50),
    cleanup_p95_ms: percentile(cancelValues, 95),
    cleanup_p99_ms: percentile(cancelValues, 99),
    cleanup_ok: cleanupAttempted.filter((sample) => sample.cleanup?.ok).length,
    cleanup_failed: cleanupAttempted.filter((sample) => !sample.cleanup?.ok).length,
    cost_count: cleanCosts.length,
    cost_mean_usd: mean(cleanCosts.map((cost) => cost?.trade_cost_usd)),
    cost_total_usd: sum(cleanCosts.map((cost) => cost?.trade_cost_usd)),
  }
}

function finiteValues(values: Array<number | undefined>) {
  return values.filter(
    (value): value is number =>
      value !== undefined && Number.isFinite(value) && value > 0
  )
}

function groupTransport(samples: Array<Sample>) {
  const transports = samples
    .map((sample) => sample.transport)
    .filter((transport): transport is string => Boolean(transport))
  const unique = new Set(transports)
  return unique.size === 1 ? transports[0] ?? "unknown" : "mixed"
}

function mean(values: Array<number | undefined>) {
  const finite = finiteValues(values)
  return finite.length > 0 ? sum(finite) / finite.length : 0
}

function sum(values: Array<number | undefined>) {
  return values.reduce<number>(
    (total, value) =>
      value !== undefined && Number.isFinite(value) ? total + value : total,
    0
  )
}

function percentile(values: Array<number>, percentileValue: number) {
  if (values.length === 0) {
    return 0
  }
  const sorted = [...values].sort((a, b) => a - b)
  const index = Math.min(
    sorted.length - 1,
    Math.max(0, Math.ceil((percentileValue / 100) * sorted.length) - 1)
  )
  return sorted[index]
}

function compareSummaryRows(left: SummaryRow, right: SummaryRow) {
  return `${left.venue}:${left.scenario}:${left.order_type}:${left.batch_size}`.localeCompare(
    `${right.venue}:${right.scenario}:${right.order_type}:${right.batch_size}`
  )
}
