package cleanup

import (
	"cmp"
	"fmt"
	"net/http"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/payload"
	"perps-latency-benchmark/internal/submission"
)

type cleanupRouteKind string

const (
	cleanupRouteHTTP      cleanupRouteKind = "http"
	cleanupRouteWebSocket cleanupRouteKind = "websocket"
)

type cleanupRoute struct {
	kind   cleanupRouteKind
	http   netlatency.RequestTemplate
	wsURL  string
	wsBody []byte
}

func cleanupRoutes(req payload.Request, built payload.Built, cfg CommandConfig, headers http.Header) ([]cleanupRoute, bool, error) {
	routes := make([]cleanupRoute, 0, 2)

	ws, ok, err := websocketCleanupRoute(req, built, cfg)
	if err != nil {
		return nil, false, err
	}
	if ok {
		routes = append(routes, ws)
	}

	httpRoute, ok, err := httpCleanupRoute(built, cfg, headers)
	if err != nil {
		return nil, false, err
	}
	if ok {
		routes = append(routes, httpRoute)
	}

	return routes, len(routes) > 0, nil
}

func websocketCleanupRoute(req payload.Request, built payload.Built, cfg CommandConfig) (cleanupRoute, bool, error) {
	if built.WSBody == nil && built.WSBodyBase64 == "" && (req.Scenario != bench.ScenarioBatch || (built.WSBatchBody == nil && built.WSBatchBodyBase64 == "")) {
		return cleanupRoute{}, false, nil
	}
	ws, err := submission.WebSocket(built, req.Scenario, submission.Defaults{
		WSURL:      cfg.WSURL,
		WSBatchURL: cfg.WSBatchURL,
	})
	if err != nil {
		return cleanupRoute{}, false, err
	}
	wsURL := payload.FirstNonEmpty(built.WSURL, cfg.WSURL)
	if req.Scenario == bench.ScenarioBatch {
		wsURL = payload.FirstNonEmpty(built.WSBatchURL, built.WSURL, cfg.WSBatchURL, cfg.WSURL)
	}
	if len(ws.Body) == 0 {
		return cleanupRoute{}, false, nil
	}
	if wsURL == "" {
		return cleanupRoute{}, false, fmt.Errorf("cleanup websocket payload requires ws_url")
	}
	return cleanupRoute{
		kind:   cleanupRouteWebSocket,
		wsURL:  wsURL,
		wsBody: ws.Body,
	}, true, nil
}

func httpCleanupRoute(built payload.Built, cfg CommandConfig, headers http.Header) (cleanupRoute, bool, error) {
	request, err := submission.HTTPRequest(built, bench.ScenarioSingle, submission.Defaults{
		Method: cmp.Or(cfg.Method, http.MethodPost),
		URL:    cfg.URL,
		Header: headers,
	})
	if err != nil {
		return cleanupRoute{}, false, err
	}
	explicit := built.Method != "" || built.URL != "" || len(built.Headers) > 0
	if len(request.Body) == 0 && !explicit {
		return cleanupRoute{}, false, nil
	}
	return cleanupRoute{
		kind: cleanupRouteHTTP,
		http: request,
	}, true, nil
}
