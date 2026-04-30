package app

import (
	"maps"
	"path/filepath"
	"strings"

	"perps-latency-benchmark/internal/bench"
	cleanupadapter "perps-latency-benchmark/internal/cleanup"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/venues/hyperliquid"
	"perps-latency-benchmark/internal/venues/lighter"
	"perps-latency-benchmark/internal/venues/registry"
)

func buildCleanupAdapter(venueName string, cfg fileConfig, client *netlatency.Client) (bench.CleanupAdapter, error) {
	cleanupCfg := cfg.Cleanup.toBenchCleanupConfig()
	if !cleanupCfg.Enabled {
		return nil, nil
	}
	definition, ok := registry.Lookup(venueName)
	if !ok {
		return nil, nil
	}
	venueCfg := cfgForVenue(definition.Name, cfg)
	request := mergeRequest(cfg.Request, venueCfg.Request)
	params := maps.Clone(definition.BuilderParams.Merge(request.Builder.Params))
	if venueCfg.BaseURL != "" {
		params["base_url"] = venueCfg.BaseURL
	}
	timeout := durationMS(cleanupCfg.TimeoutMS)

	switch definition.Name {
	case "hyperliquid":
		url := request.URL
		if url == "" {
			url = venueDefaultURL(venueCfg.BaseURL, hyperliquid.DefaultBaseURL, hyperliquid.DefaultHTTPPath)
		}
		params["base_url"] = strings.TrimSuffix(url, hyperliquid.DefaultHTTPPath)
		return cleanupadapter.NewCommandAdapter(cleanupadapter.CommandConfig{
			Type: "persistent_command",
			Command: []string{
				"uv",
				"run",
				"--with",
				"hyperliquid-python-sdk",
				"--with",
				"eth-account",
				"python",
				filepath.FromSlash("internal/venues/hyperliquid/cancel_payload.py"),
			},
			Timeout:        timeout,
			URL:            url,
			StaticParams:   params,
			Client:         client,
			Classifier:     hyperliquid.Classify,
			Description:    "cancel hyperliquid benchmark orders by cloid",
			SkipNoRefs:     true,
			OrderRefsField: "cleanup_orders",
		})
	case "lighter":
		url := request.URL
		if url == "" {
			url = venueDefaultURL(venueCfg.BaseURL, lighter.DefaultBaseURL, lighter.DefaultHTTPPath)
		}
		params["base_url"] = strings.TrimSuffix(url, lighter.DefaultHTTPPath)
		params["cancel_batch_url"] = venueDefaultURL(venueCfg.BaseURL, lighter.DefaultBaseURL, lighter.DefaultBatchPath)
		return cleanupadapter.NewCommandAdapter(cleanupadapter.CommandConfig{
			Type: "persistent_command",
			Command: []string{
				"uv",
				"run",
				"--with",
				"lighter-sdk",
				"python",
				filepath.FromSlash("internal/venues/lighter/cancel_payload.py"),
			},
			Timeout:        timeout,
			URL:            url,
			StaticParams:   params,
			Client:         client,
			Classifier:     lighter.Classify,
			Description:    "cancel lighter benchmark orders by order index",
			SkipNoRefs:     true,
			OrderRefsField: "cleanup_orders",
		})
	default:
		return nil, nil
	}
}

func venueDefaultURL(baseURL string, defaultBaseURL string, path string) string {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return strings.TrimRight(baseURL, "/") + path
}
