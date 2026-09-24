package variational_omni

import (
	"slices"
	"strings"
	"testing"

	"perps-latency-benchmark/internal/lifecycle"
)

func TestDefinitionNames(t *testing.T) {
	def := Definition()

	if def.Name != "variational_omni" {
		t.Fatalf("Name = %q", def.Name)
	}

	for _, want := range []string{"variational_omni", "variational", "omni"} {
		if !slices.Contains(def.Names(), want) {
			t.Fatalf("Names() missing %q: %#v", want, def.Names())
		}
	}
}

func TestDefinitionSetsGuardedAPIEndpoint(t *testing.T) {
	def := Definition()

	if def.DefaultBaseURL != DefaultBaseURL {
		t.Fatalf("DefaultBaseURL = %q", def.DefaultBaseURL)
	}
	if def.DefaultHTTPPath != DefaultHTTPPath {
		t.Fatalf("DefaultHTTPPath = %q", def.DefaultHTTPPath)
	}
	if def.DefaultHTTPURL != "" {
		t.Fatalf("DefaultHTTPURL = %q", def.DefaultHTTPURL)
	}
	if def.DefaultBatchPath != "" {
		t.Fatalf("DefaultBatchPath = %q", def.DefaultBatchPath)
	}
	if def.DefaultBatchURL != "" {
		t.Fatalf("DefaultBatchURL = %q", def.DefaultBatchURL)
	}
	if def.DefaultWSURL != "" {
		t.Fatalf("DefaultWSURL = %q", def.DefaultWSURL)
	}
	if def.DefaultWSBatchURL != "" {
		t.Fatalf("DefaultWSBatchURL = %q", def.DefaultWSBatchURL)
	}
	if !def.Capabilities.HTTPSingle {
		t.Fatal("expected HTTPS single capability")
	}
	if def.Capabilities.HTTPBatch || def.Capabilities.WebSocketSingle || def.Capabilities.WebSocketBatch || def.Capabilities.Cleanup {
		t.Fatalf("unexpected capabilities: %#v", def.Capabilities)
	}
}

func TestDefinitionDocumentsEndpointUncertainty(t *testing.T) {
	def := Definition()

	if !slices.Contains(def.Docs, "https://docs.variational.io/technical-documentation/api") {
		t.Fatalf("Docs missing current API docs: %#v", def.Docs)
	}
	if !slices.Contains(def.Docs, "https://docs.variational.io/for-developers/api/endpoints") {
		t.Fatalf("Docs missing endpoint docs: %#v", def.Docs)
	}
	if !slices.Contains(def.Docs, "https://github.com/variational-research/variational-sdk-python") {
		t.Fatalf("Docs missing official SDK repo: %#v", def.Docs)
	}

	notes := strings.Join(def.Notes, "\n")
	for _, want := range []string{
		ReadOnlyBaseURL + ReadOnlyStatsPath,
		"trading API is still in development",
		"live trading access must be confirmed",
		"HMAC-signed RFQ/portfolio endpoints",
		"Default action is an authenticated /status smoke check",
	} {
		if !strings.Contains(notes, want) {
			t.Fatalf("Notes missing %q: %#v", want, def.Notes)
		}
	}
}

func TestClassifyVariationalErrorEnvelope(t *testing.T) {
	got := Classify(lifecycle.ResponseInput{
		StatusCode: 200,
		Body:       []byte(`{"error":{"code":"bad_request","message":"missing rfq_id"}}`),
	})
	if got.Status != lifecycle.StatusRejected {
		t.Fatalf("status = %s, want rejected", got.Status)
	}
	if !strings.Contains(got.Reason, "missing rfq_id") {
		t.Fatalf("reason = %q", got.Reason)
	}
}
