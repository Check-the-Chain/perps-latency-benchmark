package app

import (
	"maps"
	"time"

	costadapter "perps-latency-benchmark/internal/cost"
)

func buildCostAdapter(venueName string, cfg fileConfig) (*costadapter.CommandAdapter, error) {
	runtime, ok := resolveVenueRuntime(venueName, cfg)
	if !ok {
		return nil, nil
	}
	command := runtime.Definition.CostCommand
	if len(command.Command) == 0 {
		return nil, nil
	}
	params := maps.Clone(runtime.Params)
	params["base_url"] = runtime.BaseURL()
	timeout := command.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return costadapter.NewCommandAdapter(costadapter.CommandConfig{
		Type:         command.Type,
		Command:      command.Command,
		Timeout:      timeout,
		StaticParams: params,
	})
}
