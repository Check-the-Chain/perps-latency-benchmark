import fs from "node:fs"

const configPath = new URL("../dist/server/wrangler.json", import.meta.url)
const config = JSON.parse(fs.readFileSync(configPath, "utf8"))

const nonInheritedKeys = [
  "durable_objects",
  "workflows",
  "kv_namespaces",
  "cloudchamber",
  "send_email",
  "queues",
  "r2_buckets",
  "d1_databases",
  "vectorize",
  "ai_search_namespaces",
  "ai_search",
  "hyperdrive",
  "services",
  "analytics_engine_datasets",
  "dispatch_namespaces",
  "mtls_certificates",
  "pipelines",
  "secrets_store_secrets",
  "artifacts",
  "unsafe_hello_world",
  "flagship",
  "worker_loaders",
  "ratelimits",
  "vpc_services",
  "vpc_networks",
]

function nonInheritedBindings() {
  return Object.fromEntries(
    nonInheritedKeys
      .filter((key) => Object.hasOwn(config, key))
      .map((key) => [key, config[key]])
  )
}

const emptyBindings = nonInheritedBindings()

config.env = {
  staging: {
    ...emptyBindings,
    name: "perps-latency-dashboard-staging",
    workers_dev: true,
    routes: [],
    observability: config.observability,
    secrets: config.secrets,
    vars: {
      PERPS_BENCH_API_USER: "bench",
      PERPS_BENCH_PUBLIC_SITE_URL:
        "https://perps-latency-dashboard-staging.workers.dev",
    },
  },
  production: {
    ...emptyBindings,
    name: "perps-latency-dashboard",
    workers_dev: false,
    routes: [
      {
        pattern: "latency.perps.trading",
        custom_domain: true,
      },
    ],
    observability: config.observability,
    secrets: config.secrets,
    vars: {
      PERPS_BENCH_API_USER: "bench",
      PERPS_BENCH_PUBLIC_SITE_URL: "https://latency.perps.trading",
    },
  },
}

fs.writeFileSync(configPath, `${JSON.stringify(config, null, 2)}\n`)
