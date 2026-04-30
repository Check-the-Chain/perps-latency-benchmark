import { queryOptions } from "@tanstack/react-query"

export interface HealthResponse {
  ok: boolean
  updated_at: string
}

export interface LatestResponse {
  updated_at: string
  window: string
  summaries: Array<SummaryRow>
}

export interface SummaryRow {
  venue: string
  transport: string
  scenario: string
  count: number
  ok: number
  failed: number
  mean_ms: number
  p50_ms: number
  p95_ms: number
  p99_ms: number
  cleanup_ok: number
  cleanup_failed: number
}

export interface SamplesResponse {
  samples: Array<Sample>
}

export interface CleanupResult {
  ok: boolean
  error?: string
}

export interface Sample {
  venue: string
  run_id?: string
  scenario: string
  transport: string
  index: number
  iteration: number
  warmup: boolean
  batch_size: number
  prepared_ns: number
  network_ns: number
  corrected_ns?: number
  start_delay_ns?: number
  status_code?: number
  bytes_read?: number
  ok: boolean
  error?: string
  cleanup?: CleanupResult
  completed_at: string
}

export const WINDOW_OPTIONS = ["5m", "15m", "1h", "6h", "24h"] as const

export type WindowOption = (typeof WINDOW_OPTIONS)[number]

export function isWindowOption(value: string): value is WindowOption {
  return WINDOW_OPTIONS.includes(value as WindowOption)
}

export function healthQueryOptions() {
  return queryOptions({
    queryKey: ["bench-health"],
    queryFn: () => fetchJSON<HealthResponse>("/api/bench/health"),
    refetchInterval: 10_000,
  })
}

export function latestQueryOptions(window: WindowOption) {
  return queryOptions({
    queryKey: ["bench-latest", window],
    queryFn: () =>
      fetchJSON<LatestResponse>(`/api/bench/latest?window=${window}`),
    refetchInterval: 5_000,
  })
}

export function samplesQueryOptions(window: WindowOption) {
  return queryOptions({
    queryKey: ["bench-samples", window],
    queryFn: () =>
      fetchJSON<SamplesResponse>(
        `/api/bench/samples?window=${window}&limit=2000`
      ),
    refetchInterval: 5_000,
  })
}

async function fetchJSON<T>(path: string): Promise<T> {
  const response = await fetch(path)

  if (!response.ok) {
    throw new Error(`Request failed: ${response.status}`)
  }

  return response.json() as Promise<T>
}
