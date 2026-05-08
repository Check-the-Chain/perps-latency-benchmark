import fs from "node:fs"

import {
  deploymentEnvironments,
  workerEnvConfig,
} from "./deployment-environments.mjs"

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

config.env = Object.fromEntries(
  Object.entries(deploymentEnvironments).map(([name, profile]) => [
    name,
    workerEnvConfig(profile, config, emptyBindings),
  ])
)

fs.writeFileSync(configPath, `${JSON.stringify(config, null, 2)}\n`)

for (const [name, profile] of Object.entries(deploymentEnvironments)) {
  const deployConfig = {
    ...config,
    ...workerEnvConfig(profile, config, emptyBindings),
  }

  delete deployConfig.definedEnvironments
  delete deployConfig.env
  delete deployConfig.legacy_env
  delete deployConfig.topLevelName

  const deployConfigPath = new URL(
    `../dist/server/wrangler.${name}.json`,
    import.meta.url
  )
  fs.writeFileSync(deployConfigPath, `${JSON.stringify(deployConfig, null, 2)}\n`)
}
