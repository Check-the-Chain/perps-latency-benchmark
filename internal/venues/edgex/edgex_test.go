package edgex

import (
	"slices"
	"strings"
	"testing"

	"perps-latency-benchmark/internal/lifecycle"
)

func TestDefinitionVerifiedDefaults(t *testing.T) {
	def := Definition()

	if def.Name != "edgex" {
		t.Fatalf("Name = %q", def.Name)
	}
	if def.DefaultBaseURL != "https://pro.edgex.exchange" {
		t.Fatalf("DefaultBaseURL = %q", def.DefaultBaseURL)
	}
	if def.DefaultHTTPPath != "/api/v1/private/order/createOrder" {
		t.Fatalf("DefaultHTTPPath = %q", def.DefaultHTTPPath)
	}
	if def.DefaultBatchPath != "" || def.DefaultBatchURL != "" {
		t.Fatalf("batch defaults should be empty when unverified: path=%q url=%q", def.DefaultBatchPath, def.DefaultBatchURL)
	}
	if def.DefaultWSURL != "" || def.DefaultWSBatchURL != "" {
		t.Fatalf("websocket order submission defaults should be empty when unverified: url=%q batch=%q", def.DefaultWSURL, def.DefaultWSBatchURL)
	}
	if !def.Capabilities.Cleanup {
		t.Fatal("expected cleanup capability")
	}
	if def.Confirmation == nil {
		t.Fatal("expected confirmation factory")
	}
}

func TestDefinitionAliasesAndDocs(t *testing.T) {
	def := Definition()

	for _, alias := range []string{"edgeX", "edge-x", "edge_x", "edgex_exchange"} {
		if !slices.Contains(def.Aliases, alias) {
			t.Fatalf("Aliases missing %q: %#v", alias, def.Aliases)
		}
	}

	for _, doc := range []string{
		"https://edgex-1.gitbook.io/edgeX-documentation/api/authentication",
		"https://edgex-1.gitbook.io/edgeX-documentation/api/sign",
		"https://edgexhelp.zendesk.com/hc/en-001/articles/14562599237391-Order-API",
		"https://edgex-1.gitbook.io/edgeX-documentation/api/websocket-api",
		"https://github.com/edgex-Tech/edgex-golang-sdk",
		"https://github.com/edgex-Tech/edgex-python-sdk",
	} {
		if !slices.Contains(def.Docs, doc) {
			t.Fatalf("Docs missing %q: %#v", doc, def.Docs)
		}
	}
}

func TestDefinitionNotesDocumentUnverifiedFieldsAndPayloadWrapping(t *testing.T) {
	def := Definition()
	notes := def.Notes

	for _, want := range []string{
		"JSON body containing order fields directly",
		"outside the timed send path",
		"official edgeX Python SDK",
		"caller-provided metadata",
		"private account stream",
		"Cleanup cancels by clientOrderId",
		"No official batch order creation endpoint",
		"not order submission over WebSocket",
	} {
		if !containsNote(notes, want) {
			t.Fatalf("Notes missing %q: %#v", want, notes)
		}
	}
}

func containsNote(notes []string, want string) bool {
	for _, note := range notes {
		if strings.Contains(note, want) {
			return true
		}
	}
	return false
}

func TestClassifyEdgeXCodeFailure(t *testing.T) {
	got := Classify(lifecycle.ResponseInput{
		StatusCode: 200,
		Body:       []byte(`{"code":"FAILED","msg":"bad order"}`),
	})
	if got.Status != lifecycle.StatusRejected {
		t.Fatalf("status = %s, want rejected", got.Status)
	}
}

func TestMatchEdgeXConfirmationByClientOrderID(t *testing.T) {
	ok, err := matchEdgeXConfirmation(map[string]any{
		"type":  "type-event",
		"event": "ORDER_UPDATE",
		"data": map[string]any{
			"order": map[string]any{"clientOrderId": "123", "status": "OPEN"},
		},
	}, map[string]struct{}{"123": {}}, "post_only")
	if err != nil {
		t.Fatalf("matchEdgeXConfirmation error = %v", err)
	}
	if !ok {
		t.Fatal("expected confirmation match")
	}
}

func TestMatchEdgeXConfirmationRejectsTerminalFailure(t *testing.T) {
	ok, err := matchEdgeXConfirmation(map[string]any{
		"data": map[string]any{
			"order": map[string]any{"clientOrderId": "123", "status": "REJECTED"},
		},
	}, map[string]struct{}{"123": {}}, "post_only")
	if err == nil {
		t.Fatal("expected terminal failure error")
	}
	if ok {
		t.Fatal("expected no successful match")
	}
}
