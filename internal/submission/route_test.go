package submission

import (
	"net/http"
	"testing"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/payload"
)

func TestHTTPRequestUsesBatchOverrides(t *testing.T) {
	body := `{"single":true}`
	batchBody := `{"batch":true}`
	req, err := HTTPRequest(payload.Built{
		Body:      &body,
		BatchBody: &batchBody,
	}, bench.ScenarioBatch, Defaults{
		Method:   http.MethodPost,
		URL:      "https://example.test/single",
		BatchURL: "https://example.test/batch",
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.URL != "https://example.test/batch" || string(req.Body) != batchBody {
		t.Fatalf("request = %#v body=%s", req, req.Body)
	}
}

func TestWebSocketUsesBatchFallbackOrder(t *testing.T) {
	body := `{"body":true}`
	batchBody := `{"batch":true}`
	ws, err := WebSocket(payload.Built{
		Body:        &body,
		WSBatchBody: &batchBody,
	}, bench.ScenarioBatch, Defaults{
		WSURL:      "wss://example.test/single",
		WSBatchURL: "wss://example.test/batch",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ws.URL != "wss://example.test/batch" || string(ws.Body) != batchBody {
		t.Fatalf("ws = %#v", ws)
	}
}
