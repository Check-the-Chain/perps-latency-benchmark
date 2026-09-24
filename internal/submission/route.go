package submission

import (
	"net/http"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/payload"
)

type Defaults struct {
	Method      string
	URL         string
	BatchURL    string
	Header      http.Header
	Body        []byte
	BatchBody   []byte
	WSURL       string
	WSBatchURL  string
	WSBody      []byte
	WSBatchBody []byte
}

type WebSocketPayload struct {
	URL  string
	Body []byte
}

func HTTPRequest(built payload.Built, scenario bench.Scenario, defaults Defaults) (netlatency.RequestTemplate, error) {
	targetURL := payload.FirstNonEmpty(built.URL, defaults.URL)
	bodyText := built.Body
	bodyBase64 := built.BodyBase64
	fallbackBody := defaults.Body
	if scenario == bench.ScenarioBatch {
		bodyText = firstPointer(built.BatchBody, built.Body)
		bodyBase64 = payload.FirstNonEmpty(built.BatchBodyBase64, built.BodyBase64)
		fallbackBody = firstBytes(defaults.BatchBody, defaults.Body)
		if batchURL := payload.FirstNonEmpty(built.BatchURL, defaults.BatchURL); batchURL != "" {
			targetURL = batchURL
		}
	}
	body, err := payload.Bytes(bodyText, bodyBase64, fallbackBody)
	if err != nil {
		return netlatency.RequestTemplate{}, err
	}
	return netlatency.RequestTemplate{
		Method: payload.FirstNonEmpty(built.Method, defaults.Method, http.MethodPost),
		URL:    targetURL,
		Header: payload.MergeHeaders(defaults.Header, built.Headers),
		Body:   body,
	}, nil
}

func ParallelHTTPRequests(built payload.Built, defaults Defaults) ([]netlatency.RequestTemplate, error) {
	requests := make([]netlatency.RequestTemplate, 0, len(built.ParallelRequests))
	for _, builtRequest := range built.ParallelRequests {
		body, err := payload.Bytes(builtRequest.Body, builtRequest.BodyBase64, nil)
		if err != nil {
			return nil, err
		}
		targetURL := payload.FirstNonEmpty(builtRequest.URL, built.URL, defaults.URL)
		requests = append(requests, netlatency.RequestTemplate{
			Method: payload.FirstNonEmpty(builtRequest.Method, built.Method, defaults.Method, http.MethodPost),
			URL:    targetURL,
			Header: payload.MergeHeaders(payload.MergeHeaders(defaults.Header, built.Headers), builtRequest.Headers),
			Body:   body,
		})
	}
	return requests, nil
}

func WebSocket(built payload.Built, scenario bench.Scenario, defaults Defaults) (WebSocketPayload, error) {
	bodyText := firstPointer(built.WSBody, built.Body)
	bodyBase64 := payload.FirstNonEmpty(built.WSBodyBase64, built.BodyBase64)
	fallbackBody := firstBytes(defaults.WSBody, defaults.Body)
	targetURL := payload.FirstNonEmpty(built.WSURL, defaults.WSURL)
	if scenario == bench.ScenarioBatch {
		bodyText = firstPointer(built.WSBatchBody, built.BatchBody, built.WSBody, built.Body)
		bodyBase64 = payload.FirstNonEmpty(built.WSBatchBodyBase64, built.BatchBodyBase64, built.WSBodyBase64, built.BodyBase64)
		fallbackBody = firstBytes(defaults.WSBatchBody, defaults.BatchBody, defaults.WSBody, defaults.Body)
		targetURL = payload.FirstNonEmpty(built.WSBatchURL, built.BatchURL, built.WSURL, built.URL, defaults.WSBatchURL, defaults.BatchURL, defaults.WSURL, defaults.URL)
	}
	body, err := payload.Bytes(bodyText, bodyBase64, fallbackBody)
	if err != nil {
		return WebSocketPayload{}, err
	}
	return WebSocketPayload{URL: targetURL, Body: body}, nil
}

func firstPointer(values ...*string) *string {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstBytes(values ...[]byte) []byte {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}
