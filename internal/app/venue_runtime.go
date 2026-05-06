package app

import (
	"strings"

	"perps-latency-benchmark/internal/venues/registry"
	"perps-latency-benchmark/internal/venues/spec"
)

type resolvedVenueRuntime struct {
	Name       string
	Definition spec.Definition
	Config     venueConfig
	Request    requestConfig
	Params     map[string]any
}

func resolveVenueRuntime(venueName string, cfg fileConfig) (resolvedVenueRuntime, bool) {
	definition, ok := registry.Lookup(venueName)
	if !ok {
		return resolvedVenueRuntime{}, false
	}
	venueCfg := cfgForVenue(definition.Name, cfg)
	request := mergeRequest(cfg.Request, venueCfg.Request)
	return resolvedVenueRuntime{
		Name:       definition.Name,
		Definition: definition,
		Config:     venueCfg,
		Request:    request,
		Params:     definition.BuilderParams.Merge(request.Builder.Params),
	}, true
}

func (r resolvedVenueRuntime) RuntimeConfig() spec.RuntimeConfig {
	return spec.RuntimeConfig{
		BaseURL: r.Config.BaseURL,
		WSURL:   r.Request.WSURL,
		Params:  r.Params,
	}
}

func (r resolvedVenueRuntime) BaseURL() string {
	baseURL := strings.TrimRight(r.Config.BaseURL, "/")
	if baseURL != "" {
		return baseURL
	}
	return strings.TrimSuffix(r.Definition.HTTPURL(""), r.Definition.DefaultHTTPPath)
}
