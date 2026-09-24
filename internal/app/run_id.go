package app

import "perps-latency-benchmark/internal/venues/registry"

func injectRunID(cfg *fileConfig, venueName string, runID string) {
	if runID == "" {
		return
	}
	setBuilderParam(&cfg.Request.Builder, "run_id", runID)
	if definition, ok := registry.Lookup(venueName); ok && cfg.Venues != nil {
		venueCfg := cfg.Venues[definition.Name]
		setBuilderParam(&venueCfg.Request.Builder, "run_id", runID)
		cfg.Venues[definition.Name] = venueCfg
	}
}

func setBuilderParam(builder *builderConfig, key string, value any) {
	if builder.Params == nil {
		builder.Params = make(map[string]any)
	}
	builder.Params[key] = value
}
