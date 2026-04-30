import { env } from "cloudflare:workers"

const DEFAULT_API_URL = "http://18.183.225.52:8080"
const DEFAULT_API_USER = "bench"

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
  try {
    return Response.json(await fetchBenchJSON(path))
  } catch (error) {
    const body = benchAPIErrorBody(path, error)
    console.error("benchmark API proxy failed", body)
    return Response.json(body, { status: 502 })
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
  return trimTrailingSlash(readEnv("PERPS_BENCH_API_URL") ?? DEFAULT_API_URL)
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
