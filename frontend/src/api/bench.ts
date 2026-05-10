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
  order_type: string
  measurement_mode?: string
  batch_size: number
  batch_submission?: string
  count: number
  ok: number
  failed: number
  mean_ms: number
  p50_ms: number
  p95_ms: number
  p99_ms: number
  raw_mean_ms?: number
  raw_p50_ms?: number
  raw_p95_ms?: number
  raw_p99_ms?: number
  network_floor_mean_ms?: number
  network_floor_p50_ms?: number
  network_floor_p95_ms?: number
  network_floor_p99_ms?: number
  network_adjusted_mean_ms?: number
  network_adjusted_p50_ms?: number
  network_adjusted_p95_ms?: number
  network_adjusted_p99_ms?: number
  speed_bump_ms?: number
  speed_bump_source?: string
  submission_p50_ms?: number
  submission_p95_ms?: number
  submission_p99_ms?: number
  cleanup_mean_ms?: number
  cleanup_p50_ms?: number
  cleanup_p95_ms?: number
  cleanup_p99_ms?: number
  network_adjusted_cleanup_mean_ms?: number
  network_adjusted_cleanup_p50_ms?: number
  network_adjusted_cleanup_p95_ms?: number
  network_adjusted_cleanup_p99_ms?: number
  cleanup_ok: number
  cleanup_failed: number
  cost_count?: number
  cost_mean_usd?: number
  cost_total_usd?: number
}

export interface SamplesResponse {
  samples: Array<Sample>
}

export interface ExchangeTPSResponse {
  updated_at: string
  bucket: ExchangeTPSBucket
  window: string
  series: Array<ExchangeTPSRow>
  sources: Array<ExchangeTPSSource>
}

export interface ExchangeTPSRow {
  venue: string
  bucket_start: string
  bucket_seconds: number
  complete: boolean
  tx_count: number
  block_count?: number
  order_count?: number
  place_count?: number
  cancel_count?: number
  error_count?: number
  tps: number
  orders_per_second?: number
  source_quality: string
}

export interface ExchangeTPSSource {
  venue: string
  quality: string
  bucket_seconds: number
  description: string
  updated_at: number
}

export interface CleanupResult {
  attempted?: boolean
  ok: boolean
  status_code?: number
  duration_ns?: number
  prepared_ns?: number
  scheduled_at?: string
  sent_at?: string
  start_delay_ns?: number
  write_delay_ns?: number
  bytes_read?: number
  error?: string
  description?: string
  cleanup_confirmation?: string
  metadata?: Record<string, unknown>
}

export interface SampleCost {
  venue: string
  run_id?: string
  completed_at?: string
  entry_order_id?: string
  exit_order_id?: string
  entry_qty?: number
  exit_qty?: number
  entry_value_usd?: number
  exit_value_usd?: number
  entry_fee_usd?: number
  exit_fee_usd?: number
  price_move_cost_usd?: number
  trade_cost_usd?: number
  balance_before_usd?: number
  balance_after_usd?: number
  balance_delta_usd?: number
  reconciliation_diff_usd?: number
  clean: boolean
  quality_reason?: string
}

export interface OrderRef {
  venue?: string
  symbol?: string
  market?: string
  market_index?: number
  side?: string
  size?: string
  asset?: number
  client_order_id?: string
  client_order_index?: string
  order_index?: string
  external_id?: string
  cloid?: string
}

export interface ExpectedFill {
  phase?: string
  side?: string
  size?: number
  expected_price?: number
  top_bid?: number
  top_ask?: number
  top_bid_size?: number
  top_ask_size?: number
  top_available?: number
  top_sufficient?: boolean
  book_available?: number
  book_sufficient?: boolean
  book_levels?: number
  depth_weighted?: boolean
  book_age_ns?: number
  book_received_at?: string
  book_exchange_at?: string
}

export interface Sample {
  venue: string
  run_id?: string
  scenario: string
  transport: string
  order_type?: string
  index?: number
  iteration?: number
  warmup?: boolean
  batch_size: number
  plot_at?: string
  scheduled_at?: string
  sent_at?: string
  prepared_ns?: number
  network_ns?: number
  confirm_ns?: number
  raw_network_ns?: number
  adjusted_network_ns?: number
  network_floor_ns?: number
  network_floor_source?: string
  speed_bump_ns?: number
  speed_bump_source?: string
  submission_ns?: number
  corrected_ns?: number
  start_delay_ns?: number
  status_code?: number
  bytes_read?: number
  ok: boolean
  error?: string
  cleanup?: CleanupResult
  cleanup_confirm_ns?: number
  cleanup_account_feed?: boolean
  cost?: SampleCost
  order_refs?: OrderRef[]
  closeout_order_refs?: OrderRef[]
  expected_entry_fill?: ExpectedFill
  expected_exit_fill?: ExpectedFill
  batch_submission?: string
  metadata?: Record<string, unknown>
  measurement_mode?: string
  completed_at?: string
}

export const WINDOW_OPTIONS = ["6h", "12h", "24h"] as const
export const DEFAULT_WINDOW = "12h" satisfies WindowOption
export const EXCHANGE_TPS_BUCKETS = ["1m", "1h"] as const
export const DEFAULT_EXCHANGE_TPS_BUCKET = "1m" satisfies ExchangeTPSBucket

export type WindowOption = (typeof WINDOW_OPTIONS)[number]
export type ExchangeTPSBucket = (typeof EXCHANGE_TPS_BUCKETS)[number]

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
        `/api/bench/samples?window=${window}&limit=10000`
      ),
    refetchInterval: 30_000,
    refetchOnWindowFocus: false,
    staleTime: 20_000,
  })
}

export function latencySeriesQueryOptions(window: WindowOption) {
  return queryOptions({
    queryKey: ["bench-latency-series", window],
    queryFn: () =>
      fetchJSON<SamplesResponse>(
        `/api/bench/latency-series?window=${window}&limit=10000`
      ),
    refetchInterval: 30_000,
    refetchOnWindowFocus: false,
    staleTime: 20_000,
  })
}

export function takerCostSeriesQueryOptions(window: WindowOption) {
  return queryOptions({
    queryKey: ["bench-taker-cost-series", window],
    queryFn: () =>
      fetchJSON<SamplesResponse>(
        `/api/bench/taker-cost-series?window=${window}&limit=10000`
      ),
    refetchInterval: 30_000,
    refetchOnWindowFocus: false,
    staleTime: 20_000,
  })
}

export function exchangeTPSQueryOptions(
  window: WindowOption,
  bucket: ExchangeTPSBucket = DEFAULT_EXCHANGE_TPS_BUCKET
) {
  return queryOptions({
    queryKey: ["bench-exchange-tps", window, bucket],
    queryFn: () =>
      fetchJSON<ExchangeTPSResponse>(
        `/api/bench/exchange-tps?window=${window}&bucket=${bucket}&limit=20000`
      ),
    refetchInterval: 30_000,
    refetchOnWindowFocus: false,
    staleTime: 20_000,
  })
}

async function fetchJSON<T>(path: string): Promise<T> {
  const response = await fetch(path)

  if (!response.ok) {
    throw new Error(`Request failed: ${response.status}`)
  }

  return response.json() as Promise<T>
}
