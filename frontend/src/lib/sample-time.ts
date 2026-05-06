import type { Sample } from "@/api/bench"

export function samplePlotDate(sample: Sample) {
  return firstValidDate(sample.scheduled_at, sample.sent_at, sample.completed_at)
}

function firstValidDate(...values: Array<string | undefined>) {
  for (const value of values) {
    if (!value || value.startsWith("0001-01-01")) {
      continue
    }
    const date = new Date(value)
    if (!Number.isNaN(date.getTime())) {
      return date
    }
  }
  return null
}
