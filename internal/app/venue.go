package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/payload"
	"perps-latency-benchmark/internal/venues/mock"
	"perps-latency-benchmark/internal/venues/prebuilt"
	"perps-latency-benchmark/internal/venues/registry"
	"perps-latency-benchmark/internal/venues/spec"
)

func buildVenue(name string, cfg fileConfig) (bench.Venue, error) {
	switch name {
	case "mock":
		return mock.New(mock.Config{
			Latency:   durationMS(cfg.Mock.LatencyMS),
			Status:    cfg.Mock.Status,
			Body:      cfg.Mock.Body,
			Transport: cfg.Request.Transport,
		}), nil
	case "http":
		return prebuilt.New(toPrebuiltConfig("http", cfg.Request))
	default:
		definition, err := registry.MustLookup(name)
		if err != nil {
			return nil, err
		}
		venueCfg := cfgForVenue(definition.Name, cfg)
		request := mergeRequest(cfg.Request, venueCfg.Request)
		return definition.Build(spec.Config{
			BaseURL: venueCfg.BaseURL,
			WSURL:   venueCfg.WSURL,
			Request: toPrebuiltConfig(definition.Name, request),
		})
	}
}

func toPrebuiltConfig(name string, req requestConfig) prebuilt.Config {
	method := req.Method
	if method == "" {
		method = http.MethodPost
	}
	builder, err := payloadBuilder(req.Builder)
	if err != nil {
		builder = errorBuilder{err: err}
	}
	return prebuilt.Config{
		Name:            name,
		Transport:       req.Transport,
		Method:          method,
		URL:             req.URL,
		BatchURL:        req.BatchURL,
		WSURL:           req.WSURL,
		WSBatchURL:      req.WSBatchURL,
		Headers:         req.Headers,
		Body:            req.Body,
		BodyFile:        req.BodyFile,
		BatchBody:       req.BatchBody,
		BatchBodyFile:   req.BatchBodyFile,
		WSBody:          req.WSBody,
		WSBodyFile:      req.WSBodyFile,
		WSBatchBody:     req.WSBatchBody,
		WSBatchBodyFile: req.WSBatchBodyFile,
		Builder:         builder,
		BuilderParams:   req.Builder.Params,
	}
}

func payloadBuilder(cfg builderConfig) (payload.Builder, error) {
	if cfg.Type == "" {
		return nil, nil
	}
	switch strings.ToLower(cfg.Type) {
	case "command":
		return payload.NewCommandBuilder(payload.CommandConfig{
			Command:   cfg.Command,
			Env:       cfg.Env,
			Timeout:   durationMS(cfg.TimeoutMS),
			Directory: cfg.Directory,
		})
	case "persistent_command":
		return payload.NewPersistentCommandBuilder(payload.CommandConfig{
			Command:   cfg.Command,
			Env:       cfg.Env,
			Timeout:   durationMS(cfg.TimeoutMS),
			Directory: cfg.Directory,
		})
	default:
		return nil, fmt.Errorf("unknown builder type %q", cfg.Type)
	}
}

type errorBuilder struct {
	err error
}

func (b errorBuilder) Build(context.Context, payload.Request) (payload.Built, error) {
	return payload.Built{}, b.err
}
