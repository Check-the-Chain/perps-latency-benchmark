package lighter

import (
	"context"
	"slices"
	"strings"
	"testing"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/venues/prebuilt"
	"perps-latency-benchmark/internal/venues/spec"
)

func TestDefinitionMatchesOfficialLighterEndpoints(t *testing.T) {
	definition := Definition()

	if definition.Name != "lighter" {
		t.Fatalf("Name = %q", definition.Name)
	}
	for _, alias := range []string{"zklighter", "zk_lighter"} {
		if !slices.Contains(definition.Names(), alias) {
			t.Fatalf("Names() = %#v, missing alias %q", definition.Names(), alias)
		}
	}
	if definition.DefaultBaseURL != "https://mainnet.zklighter.elliot.ai" {
		t.Fatalf("DefaultBaseURL = %q", definition.DefaultBaseURL)
	}
	if definition.DefaultHTTPPath != "/api/v1/sendTx" {
		t.Fatalf("DefaultHTTPPath = %q", definition.DefaultHTTPPath)
	}
	if definition.DefaultBatchPath != "/api/v1/sendTxBatch" {
		t.Fatalf("DefaultBatchPath = %q", definition.DefaultBatchPath)
	}
	if definition.DefaultWSURL != "wss://mainnet.zklighter.elliot.ai/stream" {
		t.Fatalf("DefaultWSURL = %q", definition.DefaultWSURL)
	}
	for _, doc := range []string{
		"https://apidocs.lighter.xyz/reference/sendtx",
		"https://apidocs.lighter.xyz/reference/sendtxbatch",
		"https://apidocs.lighter.xyz/docs/websocket-reference",
		"https://github.com/elliottech/lighter-python",
	} {
		if !slices.Contains(definition.Docs, doc) {
			t.Fatalf("Docs = %#v, missing %q", definition.Docs, doc)
		}
	}
}

func TestDefinitionBuildUsesPrebuiltPayloadsOutsideTimedPath(t *testing.T) {
	venue, err := Definition().Build(spec.Config{
		Request: prebuilt.Config{
			Body:      `{"tx_type":14,"tx_info":"signed-single"}`,
			BatchBody: `{"tx_types":"[14,15]","tx_infos":"[\"signed-a\",\"signed-b\"]"}`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	single, err := venue.Prepare(context.Background(), bench.ScenarioSingle, 7, 1)
	if err != nil {
		t.Fatal(err)
	}
	if single.Request.URL != "https://mainnet.zklighter.elliot.ai/api/v1/sendTx" {
		t.Fatalf("single URL = %q", single.Request.URL)
	}
	if string(single.Request.Body) != `{"tx_type":14,"tx_info":"signed-single"}` {
		t.Fatalf("single body = %s", single.Request.Body)
	}

	batch, err := venue.Prepare(context.Background(), bench.ScenarioBatch, 8, 2)
	if err != nil {
		t.Fatal(err)
	}
	if batch.Request.URL != "https://mainnet.zklighter.elliot.ai/api/v1/sendTxBatch" {
		t.Fatalf("batch URL = %q", batch.Request.URL)
	}
	if string(batch.Request.Body) != `{"tx_types":"[14,15]","tx_infos":"[\"signed-a\",\"signed-b\"]"}` {
		t.Fatalf("batch body = %s", batch.Request.Body)
	}
}

func TestWebSocketWrapperTypesMatchLighterDocs(t *testing.T) {
	if WebSocketSendTxType != "jsonapi/sendtx" {
		t.Fatalf("WebSocketSendTxType = %q", WebSocketSendTxType)
	}
	if WebSocketSendTxBatchType != "jsonapi/sendtxbatch" {
		t.Fatalf("WebSocketSendTxBatchType = %q", WebSocketSendTxBatchType)
	}
}

func TestClassifyPreservesLighterErrorMessage(t *testing.T) {
	classification := Classify(lifecycle.ResponseInput{
		StatusCode: 400,
		Body:       []byte(`{"code":20558,"message":"restricted jurisdiction"}`),
	})

	if classification.Status != lifecycle.StatusRejected {
		t.Fatalf("classification = %+v", classification)
	}
	if !strings.Contains(classification.Reason, "restricted jurisdiction") {
		t.Fatalf("reason = %q", classification.Reason)
	}
}
