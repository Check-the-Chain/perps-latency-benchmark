import { env } from "cloudflare:workers"
import { clearSession, useSession } from "@tanstack/react-start/server"

const ADVANCED_AUTH_COOKIE = "perps-bench-advanced"
const ADVANCED_AUTH_TTL_SECONDS = 30 * 24 * 60 * 60

type AuthEnv = Cloudflare.Env & {
  PERPS_BENCH_ADVANCED_PASSWORD?: string
  PERPS_BENCH_AUTH_SECRET?: string
}

interface AdvancedSession {
  advanced?: true
}

export async function advancedAuthStatus() {
  const enabled = advancedAuthConfigured()
  return {
    authenticated: enabled ? await isAdvancedAuthenticated() : false,
    enabled,
  }
}

export async function isAdvancedAuthenticated() {
  const session = await advancedSession()
  return session.data.advanced === true
}

export async function requireAdvancedAuth() {
  if (await isAdvancedAuthenticated()) {
    return null
  }
  return Response.json(
    { ok: false, error: "advanced authentication required" },
    {
      headers: privateNoStoreHeaders(),
      status: 401,
    }
  )
}

export async function createAdvancedLoginResponse(request: Request) {
  const configuredPassword = readAuthEnv("PERPS_BENCH_ADVANCED_PASSWORD")
  if (!configuredPassword || !readAuthEnv("PERPS_BENCH_AUTH_SECRET")) {
    return Response.json(
      { authenticated: false, enabled: false, error: "advanced auth is not configured" },
      { headers: privateNoStoreHeaders(), status: 503 }
    )
  }

  const body = await request.json().catch(() => null)
  const password = isRecord(body) && typeof body.password === "string" ? body.password : ""
  if (!constantTimeEqual(password, configuredPassword)) {
    return Response.json(
      { authenticated: false, enabled: true, error: "invalid password" },
      { headers: privateNoStoreHeaders(), status: 401 }
    )
  }

  const session = await advancedSession()
  await session.update({ advanced: true })
  return Response.json(
    { authenticated: true, enabled: true },
    { headers: privateNoStoreHeaders() }
  )
}

export async function createAdvancedLogoutResponse() {
  await clearSession(advancedSessionConfig())
  return Response.json(
    { authenticated: false, enabled: advancedAuthConfigured() },
    { headers: privateNoStoreHeaders() }
  )
}

export function privateNoStoreHeaders() {
  return new Headers({
    "Cache-Control": "private, no-store",
    Vary: "Cookie, Authorization",
  })
}

function advancedSession() {
  return useSession<AdvancedSession>(advancedSessionConfig())
}

function advancedSessionConfig() {
  return {
    cookie: {
      httpOnly: true,
      sameSite: "lax" as const,
      secure: process.env.NODE_ENV === "production",
    },
    maxAge: ADVANCED_AUTH_TTL_SECONDS,
    name: ADVANCED_AUTH_COOKIE,
    password: readAuthEnv("PERPS_BENCH_AUTH_SECRET") ?? fallbackDevSecret(),
  }
}

function advancedAuthConfigured() {
  return Boolean(
    readAuthEnv("PERPS_BENCH_ADVANCED_PASSWORD") &&
      readAuthEnv("PERPS_BENCH_AUTH_SECRET")
  )
}

function readAuthEnv(name: keyof AuthEnv) {
  const value = ((env as AuthEnv)[name] ?? process.env[name])?.trim()
  return value ? value : undefined
}

function fallbackDevSecret() {
  if (process.env.NODE_ENV === "production") {
    throw new Error("PERPS_BENCH_AUTH_SECRET is not configured")
  }
  return "development-only-perps-bench-auth-secret-32-chars"
}

function constantTimeEqual(left: string, right: string) {
  const maxLength = Math.max(left.length, right.length)
  let diff = left.length ^ right.length
  for (let index = 0; index < maxLength; index += 1) {
    diff |= (left.charCodeAt(index) || 0) ^ (right.charCodeAt(index) || 0)
  }
  return diff === 0
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}
