package spec

import (
	"testing"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/venues/prebuilt"
)

func TestDefinitionBuildFillsHTTPAndWebSocketDefaults(t *testing.T) {
	definition := Definition{
		Name:             "test",
		DefaultBaseURL:   "https://api.example.com",
		DefaultHTTPPath:  "/orders",
		DefaultBatchPath: "/orders/batch",
		DefaultWSURL:     "wss://ws.example.com/stream",
	}

	venue, err := definition.Build(Config{
		Request: prebuilt.Config{
			Body: "{}",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if venue.Name() != "test" {
		t.Fatalf("name = %s", venue.Name())
	}
}

func TestDefinitionNamesNormalizeAliases(t *testing.T) {
	definition := Definition{Name: "variational_omni", Aliases: []string{"Variational Omni", "variational-omni"}}

	names := definition.Names()

	for _, want := range []string{"variational_omni", "variational_omni", "variational_omni"} {
		if !contains(names, want) {
			t.Fatalf("names = %#v, missing %s", names, want)
		}
	}
}

func TestDefinitionSupportsExplicitCapabilities(t *testing.T) {
	definition := Definition{
		Capabilities: Capabilities{
			HTTPSingle:      true,
			WebSocketSingle: true,
		},
	}

	if !definition.Supports("https", bench.ScenarioSingle) {
		t.Fatal("expected https single support")
	}
	if !definition.Supports("websocket", bench.ScenarioSingle) {
		t.Fatal("expected websocket single support")
	}
	if definition.Supports("http", bench.ScenarioBatch) {
		t.Fatal("unexpected http batch support")
	}
	if definition.Supports("websocket", bench.ScenarioBatch) {
		t.Fatal("unexpected websocket batch support")
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
