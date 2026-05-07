package app

import (
	"cmp"
	"maps"
	"strings"

	"perps-latency-benchmark/internal/bench"
	cleanupadapter "perps-latency-benchmark/internal/cleanup"
	"perps-latency-benchmark/internal/netlatency"
)

func buildCleanupAdapter(venueName string, cfg fileConfig, client *netlatency.Client) (bench.CleanupAdapter, error) {
	cleanupCfg := cfg.Cleanup.toBenchCleanupConfig()
	if !cleanupCfg.Enabled {
		return nil, nil
	}
	runtime, ok := resolveVenueRuntime(venueName, cfg)
	if !ok {
		return nil, nil
	}
	command := runtime.Definition.CleanupCommand
	if len(command.Command) == 0 {
		return nil, nil
	}
	params := maps.Clone(runtime.Params)
	params["neutralize_on_fill"] = cfg.Risk.NeutralizeOnFill
	timeout := durationMS(cleanupCfg.TimeoutMS)

	url := runtime.Request.URL
	if url == "" {
		url = runtime.Definition.HTTPURL(runtime.Config.BaseURL)
	}
	if runtime.Definition.DefaultHTTPPath != "" {
		params["base_url"] = strings.TrimSuffix(url, runtime.Definition.DefaultHTTPPath)
	} else if runtime.Config.BaseURL != "" {
		params["base_url"] = runtime.Config.BaseURL
	}
	if batchURL := runtime.Definition.BatchURL(runtime.Config.BaseURL); batchURL != "" {
		params["cancel_batch_url"] = batchURL
	}
	return cleanupadapter.NewCommandAdapter(cleanupadapter.CommandConfig{
		Type:               command.Type,
		Command:            command.Command,
		Timeout:            timeout,
		URL:                url,
		WSURL:              cmp.Or(runtime.Request.WSURL, runtime.Config.WSURL, runtime.Definition.DefaultWSURL),
		WSBatchURL:         cmp.Or(runtime.Request.WSBatchURL, runtime.Definition.DefaultWSBatchURL, runtime.Request.WSURL, runtime.Config.WSURL, runtime.Definition.DefaultWSURL),
		WSReadInitial:      runtime.Definition.WSReadInitial,
		WSHeartbeat:        netlatency.WebSocketHeartbeat{Message: []byte(runtime.Definition.WSHeartbeat.Message), IdleAfter: runtime.Definition.WSHeartbeat.IdleAfter, Timeout: runtime.Definition.WSHeartbeat.Timeout},
		StaticParams:       params,
		Client:             client,
		Classifier:         runtime.Definition.Classifier,
		CancelConfirmation: runtime.Definition.CancelConfirmation,
		Description:        command.Description,
		SkipNoRefs:         command.SkipNoRefs,
		OrderRefsField:     command.OrderRefsField,
	})
}
