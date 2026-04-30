package variational_omni

import (
	"slices"
	"strings"
	"testing"
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

func TestDefinitionLeavesUnverifiedSubmissionEndpointsEmpty(t *testing.T) {
	def := Definition()

	if def.DefaultBaseURL != "" {
		t.Fatalf("DefaultBaseURL = %q", def.DefaultBaseURL)
	}
	if def.DefaultHTTPPath != "" {
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
}

func TestDefinitionDocumentsEndpointUncertainty(t *testing.T) {
	def := Definition()

	if !slices.Contains(def.Docs, "https://docs.variational.io/technical-documentation/api") {
		t.Fatalf("Docs missing current API docs: %#v", def.Docs)
	}
	if !slices.Contains(def.Docs, "https://github.com/variational-research/variational-sdk-python") {
		t.Fatalf("Docs missing official SDK repo: %#v", def.Docs)
	}

	notes := strings.Join(def.Notes, "\n")
	for _, want := range []string{
		ReadOnlyBaseURL + ReadOnlyStatsPath,
		"trading API is still in development",
		"HTTP order/tx, batch, and WebSocket submission defaults are intentionally empty",
		"does not provide a verified Omni order payload builder",
	} {
		if !strings.Contains(notes, want) {
			t.Fatalf("Notes missing %q: %#v", want, def.Notes)
		}
	}
}
