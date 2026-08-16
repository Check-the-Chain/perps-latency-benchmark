package risex

import (
	"testing"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/lifecycle"
)

func TestDefinitionCapabilities(t *testing.T) {
	definition := Definition()

	if definition.Name != "risex" {
		t.Fatalf("name = %s", definition.Name)
	}
	if !definition.Supports("http", bench.ScenarioSingle) {
		t.Fatal("expected http single support")
	}
	if definition.Supports("http", bench.ScenarioBatch) {
		t.Fatal("did not expect batch support; concurrent single-order fanout is confirmed unreliable for RISEx's strict sequential nonce_anchor")
	}
	if definition.Supports("websocket", bench.ScenarioSingle) {
		t.Fatal("did not expect websocket order submission; RISEx has no documented WS order-entry endpoint")
	}
	if definition.DefaultBaseURL != DefaultBaseURL {
		t.Fatalf("default base url = %s", definition.DefaultBaseURL)
	}
}

func TestClassifyRisexResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
		want lifecycle.ClassificationStatus
	}{
		{
			name: "success",
			body: `{"order_id":"12345-100-0","tx_hash":"0xabc","block_number":"1","sc_order_id":"12345","filled_quantity":"0","filled_percent":"0.00"}`,
			want: lifecycle.StatusAccepted,
		},
		{
			name: "success_wrapped",
			body: `{"data":{"order_id":"12345-100-0","tx_hash":"0xabc"},"request_id":"r1"}`,
			want: lifecycle.StatusAccepted,
		},
		{
			name: "generic_rejection",
			body: `{"code":5,"message":"price out of bounds","details":[]}`,
			want: lifecycle.StatusRejected,
		},
		{
			name: "not_registered",
			body: `{"code":3,"message":"account not registered (userId 0)","details":[]}`,
			want: lifecycle.StatusAuthError,
		},
		{
			name: "invalid_signature",
			body: `{"code":7,"message":"invalid permit signature","details":[]}`,
			want: lifecycle.StatusAuthError,
		},
		{
			name: "nonce_reused",
			body: `{"code":9,"message":"nonce bitmap index already consumed","details":[]}`,
			want: lifecycle.StatusNonceError,
		},
		{
			name: "rate_limited",
			body: `{"code":11,"message":"rate limit exceeded","details":[]}`,
			want: lifecycle.StatusRateLimited,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(lifecycle.ResponseInput{Body: []byte(tt.body)})
			if got.Status != tt.want {
				t.Fatalf("status = %s, want %s (%s)", got.Status, tt.want, got.Reason)
			}
		})
	}
}
