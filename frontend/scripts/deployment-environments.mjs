export const deploymentEnvironments = {
  staging: {
    name: "perps-latency-dashboard-staging",
    route: "staging-latency.perps.trading",
    publicSiteURL: "https://staging-latency.perps.trading",
    apiUser: "bench",
    hiddenVenues: "edgex",
  },
  production: {
    name: "perps-latency-dashboard",
    route: "latency.perps.trading",
    publicSiteURL: "https://latency.perps.trading",
    apiUser: "bench",
    hiddenVenues: "edgex",
  },
}

export function workerEnvConfig(profile, baseConfig, nonInheritedBindings) {
  return {
    ...nonInheritedBindings,
    name: profile.name,
    workers_dev: false,
    routes: [
      {
        pattern: profile.route,
        custom_domain: true,
      },
    ],
    observability: baseConfig.observability,
    secrets: baseConfig.secrets,
    vars: {
      PERPS_BENCH_API_USER: profile.apiUser,
      PERPS_BENCH_HIDDEN_VENUES: profile.hiddenVenues,
      PERPS_BENCH_PUBLIC_SITE_URL: profile.publicSiteURL,
    },
  }
}
