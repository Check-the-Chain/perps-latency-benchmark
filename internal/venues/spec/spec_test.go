package spec

import (
	"context"
	"testing"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/payload"
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

func TestDefinitionBuildMergesBuilderDefaults(t *testing.T) {
	builder := &captureBuilder{}
	definition := Definition{
		Name:           "test",
		DefaultHTTPURL: "https://api.example.com/orders",
		BuilderParams: BuilderParams{
			Defaults: map[string]any{
				"symbol": "BTC",
				"size":   "0.001",
				"price":  "75000",
			},
		},
	}

	venue, err := definition.Build(Config{
		Request: prebuilt.Config{
			Builder: builder,
			BuilderParams: map[string]any{
				"price": "76000",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := venue.Prepare(context.Background(), bench.ScenarioSingle, 0, 1); err != nil {
		t.Fatal(err)
	}
	if builder.params["symbol"] != "BTC" {
		t.Fatalf("symbol = %v", builder.params["symbol"])
	}
	if builder.params["size"] != "0.001" {
		t.Fatalf("size = %v", builder.params["size"])
	}
	if builder.params["price"] != "76000" {
		t.Fatalf("price override = %v", builder.params["price"])
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

type captureBuilder struct {
	params map[string]any
}

func (b *captureBuilder) Build(_ context.Context, req payload.Request) (payload.Built, error) {
	b.params = req.Params
	body := "{}"
	return payload.Built{Body: &body}, nil
}
