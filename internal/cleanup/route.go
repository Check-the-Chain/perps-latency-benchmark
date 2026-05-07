package cleanup

import (
	"cmp"
	"fmt"
	"net/http"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/payload"
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
	bodyText := built.WSBody
	bodyBase64 := built.WSBodyBase64
	wsURL := cmp.Or(built.WSURL, cfg.WSURL)
	if req.Scenario == bench.ScenarioBatch {
		bodyText = firstCleanupPointer(built.WSBatchBody, built.WSBody)
		bodyBase64 = payload.FirstNonEmpty(built.WSBatchBodyBase64, built.WSBodyBase64)
		wsURL = cmp.Or(built.WSBatchURL, built.WSURL, cfg.WSBatchURL, cfg.WSURL)
	}
	if bodyText == nil && bodyBase64 == "" {
		return cleanupRoute{}, false, nil
	}
	body, err := payload.Bytes(bodyText, bodyBase64, nil)
	if err != nil {
		return cleanupRoute{}, false, err
	}
	if len(body) == 0 {
		return cleanupRoute{}, false, nil
	}
	if wsURL == "" {
		return cleanupRoute{}, false, fmt.Errorf("cleanup websocket payload requires ws_url")
	}
	return cleanupRoute{
		kind:   cleanupRouteWebSocket,
		wsURL:  wsURL,
		wsBody: body,
	}, true, nil
}

func httpCleanupRoute(built payload.Built, cfg CommandConfig, headers http.Header) (cleanupRoute, bool, error) {
	body, err := payload.Bytes(built.Body, built.BodyBase64, nil)
	if err != nil {
		return cleanupRoute{}, false, err
	}
	explicit := built.Method != "" || built.URL != "" || len(built.Headers) > 0
	if len(body) == 0 && !explicit {
		return cleanupRoute{}, false, nil
	}
	return cleanupRoute{
		kind: cleanupRouteHTTP,
		http: netlatency.RequestTemplate{
			Method: cmp.Or(built.Method, cfg.Method, http.MethodPost),
			URL:    cmp.Or(built.URL, cfg.URL),
			Header: payload.MergeHeaders(headers, built.Headers),
			Body:   body,
		},
	}, true, nil
}

func firstCleanupPointer(values ...*string) *string {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
