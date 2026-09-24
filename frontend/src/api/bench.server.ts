import { env } from "cloudflare:workers"

const DEFAULT_API_USER = "bench"
const BENCH_API_CACHE_NAME = "perps-bench-api"
const BENCH_API_CACHE_TTL_SECONDS = 60
const CACHE_STATUS_HEADER = "x-perps-bench-cache"

type CacheStatus = "HIT" | "MISS" | "BYPASS"

type BenchEnv = Cloudflare.Env & {
  PERPS_BENCH_HIDDEN_VENUES?: string
  PERPS_BENCH_API_PASSWORD?: string
}

export async function fetchBenchJSON<T>(path: string): Promise<T> {
  const response = await fetch(`${benchAPIURL()}${path}`, {
    headers: benchAPIHeaders(),
  })

  if (!response.ok) {
    throw new BenchAPIError(path, response.status)
  }

  return response.json() as Promise<T>
}

export async function proxyBenchJSON(path: string) {
  return proxyBenchJSONWithFallback(path)
}

export async function proxyBenchJSONWithFallback(
  path: string,
  fallbackPath?: string | Array<string>
) {
  const cached = await readCachedBenchResponse(path)
  if (cached) {
    return cached
  }

  try {
    const fetched = await fetchBenchJSONWithFallback(path, fallbackPath)
    const data = shapeBenchJSON(fetched.path, fetched.data)
    const shouldCache = shouldCacheBenchData(path, data)
    const response = Response.json(data, {
      headers: cacheHeaders(shouldCache ? "MISS" : "BYPASS"),
    })
    if (shouldCache) {
      await writeCachedBenchResponse(path, response)
    }
    return response
  } catch (error) {
    const body = benchAPIErrorBody(path, error)
    console.error("benchmark API proxy failed", body)
    return Response.json(body, {
      headers: {
        [CACHE_STATUS_HEADER]: "ERROR",
      },
      status: 502,
    })
  }
}

async function fetchBenchJSONWithFallback<T>(
  path: string,
  fallbackPath?: string | Array<string>
): Promise<{ data: T; path: string }> {
  const paths = [path, ...(Array.isArray(fallbackPath) ? fallbackPath : fallbackPath ? [fallbackPath] : [])]
  let lastError: unknown

  for (const candidate of paths) {
    try {
      return { data: await fetchBenchJSON<T>(candidate), path: candidate }
    } catch (error) {
      lastError = error
      if (!(error instanceof BenchAPIError) || error.upstreamStatus !== 404) {
        throw error
      }
    }
  }
  throw lastError
}

class BenchAPIError extends Error {
  constructor(
    public readonly path: string,
    public readonly upstreamStatus: number
  ) {
    super(`Benchmark API request failed with status ${upstreamStatus}`)
  }
}

function benchAPIErrorBody(path: string, error: unknown) {
  if (error instanceof BenchAPIError) {
    return {
      ok: false,
      path,
      upstream_status: error.upstreamStatus,
    }
  }

  return {
    ok: false,
    path,
    error: error instanceof Error ? error.message : "Unknown benchmark API error",
  }
}

function benchAPIURL() {
  const url = readEnv("PERPS_BENCH_API_URL")
  if (!url) {
    throw new Error("PERPS_BENCH_API_URL is not configured")
  }
  return trimTrailingSlash(url)
}

function benchAPIHeaders() {
  const password = readEnv("PERPS_BENCH_API_PASSWORD")
  if (!password) {
    return undefined
  }

  const user = readEnv("PERPS_BENCH_API_USER") ?? DEFAULT_API_USER
  return {
    Authorization: `Basic ${base64(`${user}:${password}`)}`,
  }
}

async function readCachedBenchResponse(path: string) {
  const cache = await benchResponseCache()
  if (!cache) {
    return null
  }

  const response = await cache.match(cacheRequest(path))
  if (!response) {
    return null
  }

  const headers = new Headers(response.headers)
  headers.set("Cache-Control", cacheControlHeader())
  headers.set(CACHE_STATUS_HEADER, "HIT")
  return new Response(response.body, {
    headers,
    status: response.status,
    statusText: response.statusText,
  })
}

async function writeCachedBenchResponse(path: string, response: Response) {
  const cache = await benchResponseCache()
  if (!cache) {
    return
  }

  await cache.put(cacheRequest(path), response.clone())
}

async function benchResponseCache() {
  if (typeof caches === "undefined") {
    return null
  }
  return caches.open(BENCH_API_CACHE_NAME)
}

function cacheRequest(path: string) {
  return new Request(`${publicSiteURL()}/__bench-api-cache${path}`, {
    method: "GET",
  })
}

function cacheHeaders(status: CacheStatus) {
  return {
    "Cache-Control": cacheControlHeader(),
    [CACHE_STATUS_HEADER]: status,
  }
}

function cacheControlHeader() {
  return `public, max-age=0, s-maxage=${BENCH_API_CACHE_TTL_SECONDS}`
}

function publicSiteURL() {
  return trimTrailingSlash(
    readEnv("PERPS_BENCH_PUBLIC_SITE_URL") ?? "https://perps-bench.local"
  )
}

function readEnv(name: keyof BenchEnv) {
  const value = ((env as BenchEnv)[name] ?? process.env[name])?.trim()
  return value ? value : undefined
}

function trimTrailingSlash(value: string) {
  return value.endsWith("/") ? value.slice(0, -1) : value
}

function base64(value: string) {
  if (typeof btoa === "function") {
    return btoa(value)
  }

  return Buffer.from(value).toString("base64")
}

function shouldCacheBenchData(path: string, data: unknown) {
  if (path.startsWith("/api/latest?")) {
    return hasNonEmptyArray(data, "summaries")
  }

  if (isSamplesPath(path)) {
    return hasNonEmptyArray(data, "samples")
  }

  if (path === "/api/health") {
    return isRecord(data) && data.ok === true
  }

  return true
}

function filterBenchJSON(path: string, data: unknown) {
  const hiddenVenues = hiddenVenueSet()
  if (hiddenVenues.size === 0 || !isRecord(data)) {
    return data
  }

  if (path.startsWith("/api/latest?") && Array.isArray(data.summaries)) {
    return {
      ...data,
      summaries: data.summaries.filter((row) => isVisibleVenueRow(row, hiddenVenues)),
    }
  }

  if (isSamplesPath(path) && Array.isArray(data.samples)) {
    return {
      ...data,
      samples: data.samples.filter((sample) =>
        isVisibleVenueRow(sample, hiddenVenues)
      ),
    }
  }

  if (path.startsWith("/api/exchange-tps?")) {
    return {
      ...data,
      series: Array.isArray(data.series)
        ? data.series.filter((row) => isVisibleVenueRow(row, hiddenVenues))
        : data.series,
      sources: Array.isArray(data.sources)
        ? data.sources.filter((row) => isVisibleVenueRow(row, hiddenVenues))
        : data.sources,
    }
  }

  return data
}

function shapeBenchJSON(path: string, data: unknown) {
  const filtered = filterBenchJSON(path, data)
  if (!path.startsWith("/api/samples?") || !isRecord(filtered) || !Array.isArray(filtered.samples)) {
    return filtered
  }

  return {
    ...filtered,
    samples: filtered.samples.map(compactSample),
  }
}

function isSamplesPath(path: string) {
  return (
    path.startsWith("/api/samples?") ||
    path.startsWith("/api/dashboard/samples?") ||
    path.startsWith("/api/dashboard/latency-series?") ||
    path.startsWith("/api/dashboard/taker-cost-series?")
  )
}

function compactSample(sample: unknown) {
  if (!isRecord(sample)) {
    return sample
  }

  return omitUndefined({
    adjusted_network_ns: sample.adjusted_network_ns,
    batch_submission: sample.batch_submission,
    batch_size: sample.batch_size,
    cleanup: compactCleanup(sample.cleanup),
    completed_at: sample.completed_at,
    cost: sample.cost,
    expected_entry_fill: compactExpectedFill(sample.expected_entry_fill),
    expected_exit_fill: compactExpectedFill(sample.expected_exit_fill),
    measurement_mode: sample.measurement_mode,
    metadata: compactMetadata(sample.metadata),
    network_floor_ns: sample.network_floor_ns,
    network_ns: sample.network_ns,
    ok: sample.ok,
    order_type: sample.order_type,
    raw_network_ns: sample.raw_network_ns,
    scenario: sample.scenario,
    scheduled_at: sample.scheduled_at,
    sent_at: sample.sent_at,
    speed_bump_ns: sample.speed_bump_ns,
    submission_ns: sample.submission_ns,
    venue: sample.venue,
    warmup: sample.warmup,
  })
}

function compactCleanup(cleanup: unknown) {
  if (!isRecord(cleanup)) {
    return undefined
  }

  return omitUndefined({
    cleanup_confirmation: cleanup.cleanup_confirmation,
    description: cleanup.description,
    duration_ns: cleanup.duration_ns,
    metadata: compactCleanupMetadata(cleanup.metadata),
    ok: cleanup.ok,
  })
}

function compactCleanupMetadata(metadata: unknown) {
  if (!isRecord(metadata)) {
    return undefined
  }

  return omitUndefined({
    cleanup_confirmation: metadata.cleanup_confirmation,
  })
}

function compactExpectedFill(fill: unknown) {
  if (!isRecord(fill)) {
    return undefined
  }

  return omitUndefined({
    book_sufficient: fill.book_sufficient,
    expected_price: fill.expected_price,
    side: fill.side,
    top_sufficient: fill.top_sufficient,
  })
}

function compactMetadata(metadata: unknown) {
  if (!isRecord(metadata)) {
    return undefined
  }

  return omitUndefined({
    native_batch_endpoint: metadata.native_batch_endpoint,
    submission_model: metadata.submission_model,
  })
}

function omitUndefined(record: Record<string, unknown>) {
  return Object.fromEntries(
    Object.entries(record).filter(([, value]) => value !== undefined)
  )
}

function hiddenVenueSet() {
  return new Set(
    (readEnv("PERPS_BENCH_HIDDEN_VENUES") ?? "")
      .split(",")
      .map((venue) => venue.trim().toLowerCase())
      .filter(Boolean)
  )
}

function isVisibleVenueRow(value: unknown, hiddenVenues: Set<string>) {
  if (!isRecord(value) || typeof value.venue !== "string") {
    return true
  }
  return !hiddenVenues.has(value.venue.toLowerCase())
}

function hasNonEmptyArray(data: unknown, key: string) {
  return isRecord(data) && Array.isArray(data[key]) && data[key].length > 0
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null
}
