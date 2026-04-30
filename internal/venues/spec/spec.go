package spec

import (
	"cmp"
	"fmt"
	"net/url"
	"strings"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/names"
	"perps-latency-benchmark/internal/venues/prebuilt"
)

type Capabilities struct {
	HTTPSingle      bool
	HTTPBatch       bool
	WebSocketSingle bool
	WebSocketBatch  bool
	Cleanup         bool
}

type BuilderParams struct {
	Required []string
}

type Definition struct {
	Name              string
	Aliases           []string
	DefaultBaseURL    string
	DefaultHTTPPath   string
	DefaultBatchPath  string
	DefaultHTTPURL    string
	DefaultBatchURL   string
	DefaultWSURL      string
	DefaultWSBatchURL string
	Capabilities      Capabilities
	BuilderParams     BuilderParams
	Classifier        lifecycle.Classifier
	Docs              []string
	Notes             []string
}

type Config struct {
	BaseURL string
	WSURL   string
	Request prebuilt.Config
}

func (d Definition) Build(cfg Config) (bench.Venue, error) {
	if d.Name == "" {
		return nil, fmt.Errorf("venue definition missing name")
	}

	req := cfg.Request
	req.Name = d.Name
	if req.URL == "" {
		req.URL = d.httpURL(cfg.BaseURL)
	}
	if req.BatchURL == "" {
		req.BatchURL = d.batchURL(cfg.BaseURL)
	}
	if req.WSURL == "" {
		req.WSURL = cmp.Or(cfg.WSURL, d.DefaultWSURL)
	}
	if req.WSBatchURL == "" {
		req.WSBatchURL = cmp.Or(d.DefaultWSBatchURL, req.WSURL)
	}
	req.Classifier = d.Classifier

	return prebuilt.New(req)
}

func (d Definition) Names() []string {
	normalized := []string{names.Normalize(d.Name)}
	for _, alias := range d.Aliases {
		normalized = append(normalized, names.Normalize(alias))
	}
	return normalized
}

func (d Definition) httpURL(baseURL string) string {
	if d.DefaultHTTPURL != "" {
		return d.DefaultHTTPURL
	}
	return joinURL(cmp.Or(baseURL, d.DefaultBaseURL), d.DefaultHTTPPath)
}

func (d Definition) batchURL(baseURL string) string {
	if d.DefaultBatchURL != "" {
		return d.DefaultBatchURL
	}
	if d.DefaultBatchPath == "" {
		return ""
	}
	return joinURL(cmp.Or(baseURL, d.DefaultBaseURL), d.DefaultBatchPath)
}

func (d Definition) Supports(transport string, scenario bench.Scenario) bool {
	capabilities := d.Capabilities
	if transport == "" || transport == "https" {
		transport = "http"
	}
	switch transport {
	case "http":
		if scenario == bench.ScenarioBatch {
			return capabilities.HTTPBatch
		}
		return capabilities.HTTPSingle
	case "websocket":
		if scenario == bench.ScenarioBatch {
			return capabilities.WebSocketBatch
		}
		return capabilities.WebSocketSingle
	default:
		return false
	}
}

func joinURL(base string, path string) string {
	if base == "" || path == "" {
		return ""
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + strings.TrimLeft(path, "/")
	return parsed.String()
}
