import { env } from "cloudflare:workers"

const DEFAULT_API_USER = "bench"
const BENCH_API_CACHE_NAME = "perps-bench-api"
const BENCH_API_CACHE_TTL_SECONDS = 60
const CACHE_STATUS_HEADER = "x-perps-bench-cache"

type BenchEnv = Cloudflare.Env & {
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
  const cached = await readCachedBenchResponse(path)
  if (cached) {
    return cached
  }

  try {
    const response = Response.json(await fetchBenchJSON(path), {
      headers: cacheHeaders("MISS"),
    })
    await writeCachedBenchResponse(path, response)
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

function cacheHeaders(status: "HIT" | "MISS") {
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
