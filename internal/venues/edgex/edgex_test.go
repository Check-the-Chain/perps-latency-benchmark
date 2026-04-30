package edgex

import (
	"slices"
	"strings"
	"testing"
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
