package grvt

import (
	"reflect"
	"strings"
	"testing"
)

func TestDefinition(t *testing.T) {
	def := Definition()

	if def.Name != "grvt" {
		t.Fatalf("Name = %q, want grvt", def.Name)
	}
	if def.DefaultBaseURL != "https://trades.grvt.io" {
		t.Fatalf("DefaultBaseURL = %q", def.DefaultBaseURL)
	}
	if def.DefaultHTTPPath != "/full/v1/create_order" {
		t.Fatalf("DefaultHTTPPath = %q", def.DefaultHTTPPath)
	}
	if def.DefaultBatchPath != "/full/v2/bulk_orders" {
		t.Fatalf("DefaultBatchPath = %q", def.DefaultBatchPath)
	}
	if def.DefaultWSURL != "wss://trades.grvt.io/ws/full" {
		t.Fatalf("DefaultWSURL = %q", def.DefaultWSURL)
	}
	if def.DefaultHTTPURL != "" {
		t.Fatalf("DefaultHTTPURL = %q, want empty so base overrides work", def.DefaultHTTPURL)
	}
	if def.DefaultBatchURL != "" {
		t.Fatalf("DefaultBatchURL = %q, want empty so base overrides work", def.DefaultBatchURL)
	}
	if def.DefaultWSBatchURL != "" {
		t.Fatalf("DefaultWSBatchURL = %q, want empty to share DefaultWSURL", def.DefaultWSBatchURL)
	}

	wantAliases := []string{"gravity", "gravitymarkets", "gravity-markets"}
	if !reflect.DeepEqual(def.Aliases, wantAliases) {
		t.Fatalf("Aliases = %#v, want %#v", def.Aliases, wantAliases)
	}
}

func TestDefinitionNames(t *testing.T) {
	got := Definition().Names()
	want := []string{"grvt", "gravity", "gravitymarkets", "gravity_markets"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Names = %#v, want %#v", got, want)
	}
}

func TestDefinitionDocsAndNotes(t *testing.T) {
	def := Definition()

	for _, want := range []string{
		"https://api-docs.grvt.io/",
		"https://api-docs.grvt.io/trading_api/",
		"https://api-docs.grvt.io/trading_streams/",
		"https://github.com/gravity-technologies/grvt-pysdk",
	} {
		if !contains(def.Docs, want) {
			t.Fatalf("Docs missing %q in %#v", want, def.Docs)
		}
	}

	for _, want := range []string{"signed order", "orders array", "JSON-RPC", "before benchmark timing", "X-Grvt-Account-Id"} {
		if !containsSubstring(def.Notes, want) {
			t.Fatalf("Notes missing substring %q in %#v", want, def.Notes)
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsSubstring(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}
