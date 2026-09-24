package cleanup

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/payload"
)

func TestCleanupMetadataPreservesCleanupOrders(t *testing.T) {
	metadata := map[string]any{
		"cleanup":        "neutralize_position",
		"cleanup_orders": []any{map[string]any{"client_order_id": "close-1"}},
		"reconciliation": map[string]any{"position_before": "long"},
	}

	got := cleanupMetadata(metadata)
	if _, ok := got["cleanup"]; ok {
		t.Fatalf("cleanup description leaked into metadata: %#v", got)
	}
	if got["cleanup_orders"] == nil {
		t.Fatalf("cleanup_orders missing: %#v", got)
	}
	if got["reconciliation"] == nil {
		t.Fatalf("reconciliation missing: %#v", got)
	}
}

func TestRetryableCleanupResult(t *testing.T) {
	tests := []struct {
		name    string
		cleanup bench.CleanupResult
		want    bool
	}{
		{
			name:    "ok",
			cleanup: bench.CleanupResult{OK: true},
			want:    false,
		},
		{
			name:    "device time",
			cleanup: bench.CleanupResult{Error: "rejected: Your device time must match the actual time"},
			want:    true,
		},
		{
			name:    "nonce",
			cleanup: bench.CleanupResult{Error: "nonce_error: timestamp outside recvWindow"},
			want:    true,
		},
		{
			name:    "hard rejection",
			cleanup: bench.CleanupResult{Error: "rejected: insufficient margin"},
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retryableCleanupResult(tt.cleanup); got != tt.want {
				t.Fatalf("retryableCleanupResult() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCleanupRoutesPreferWebSocketThenHTTP(t *testing.T) {
	body := "{}"
	wsBody := `{"method":"post"}`
	routes, ok, err := cleanupRoutes(payload.Request{}, payload.Built{
		Body:   &body,
		WSBody: &wsBody,
	}, CommandConfig{
		URL:   "https://example.test/cancel",
		WSURL: "wss://example.test/ws",
	}, http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || len(routes) != 2 {
		t.Fatalf("routes = %#v ok=%v", routes, ok)
	}
	if routes[0].kind != cleanupRouteWebSocket || routes[1].kind != cleanupRouteHTTP {
		t.Fatalf("route order = %#v", routes)
	}
}

func TestCleanupRoutesAllowExplicitHTTPWithoutBody(t *testing.T) {
	routes, ok, err := cleanupRoutes(payload.Request{}, payload.Built{
		Method: "DELETE",
		URL:    "https://example.test/cancel?id=1",
	}, CommandConfig{}, http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || len(routes) != 1 || routes[0].kind != cleanupRouteHTTP {
		t.Fatalf("routes = %#v ok=%v", routes, ok)
	}
	if routes[0].http.Method != "DELETE" || routes[0].http.URL == "" {
		t.Fatalf("http route = %#v", routes[0].http)
	}
}

func TestAfterRunRetriesNonceCleanup(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt := atomic.AddInt32(&requests, 1)
		if attempt == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid nonce"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	body := "{}"
	builder := &staticCleanupBuilder{built: payload.Built{
		Method: http.MethodPost,
		URL:    server.URL,
		Body:   &body,
	}}
	adapter := &CommandAdapter{
		cfg: CommandConfig{
			Client:  netlatency.NewClient(netlatency.ClientConfig{Timeout: time.Second}),
			Timeout: time.Second,
		},
		builder: builder,
		headers: make(http.Header),
	}

	cleanup := adapter.AfterRun(context.Background(), bench.Result{Venue: "lighter"})
	if !cleanup.OK {
		t.Fatalf("AfterRun cleanup OK = false, error = %q", cleanup.Error)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
	if builder.calls != 2 {
		t.Fatalf("builder calls = %d, want 2", builder.calls)
	}
	if got := cleanup.Metadata["cleanup_retry_count"]; got != 1 {
		t.Fatalf("cleanup_retry_count = %#v, want 1", got)
	}
}

type staticCleanupBuilder struct {
	built payload.Built
	calls int
}

func (b *staticCleanupBuilder) Build(context.Context, payload.Request) (payload.Built, error) {
	b.calls++
	return b.built, nil
}
