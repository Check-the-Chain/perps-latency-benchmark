package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestServeAuthRequiredForNonLoopbackListen(t *testing.T) {
	t.Setenv("PERPS_BENCH_TEST_API_PASSWORD", "")

	err := serveResults(t.Context(), &serveOptions{
		storePath:       filepath.Join(t.TempDir(), "bench.db"),
		listen:          "0.0.0.0:0",
		authPasswordEnv: "PERPS_BENCH_TEST_API_PASSWORD",
	})
	if err == nil {
		t.Fatal("expected public bind without password to fail")
	}
}

func TestServeAuthListenClassification(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:8080":  false,
		"localhost:8080":  false,
		"[::1]:8080":      false,
		":8080":           true,
		"0.0.0.0:8080":    true,
		"[::]:8080":       true,
		"172.31.0.2:8080": true,
	}
	for listen, want := range cases {
		if got := requiresServeAuth(listen); got != want {
			t.Fatalf("requiresServeAuth(%q) = %v, want %v", listen, got, want)
		}
	}
}

func TestBasicAuthMiddleware(t *testing.T) {
	called := false
	handler := withBasicAuth("bench", "secret", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", w.Code)
	}
	if called {
		t.Fatal("handler was called for unauthenticated request")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.SetBasicAuth("bench", "secret")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("authenticated status = %d", w.Code)
	}
}
