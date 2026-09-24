package nado

import (
	"testing"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/lifecycle"
)

func TestDefinitionSupportsWebSocketSingle(t *testing.T) {
	definition := Definition()

	if definition.Name != "nado" {
		t.Fatalf("name = %s", definition.Name)
	}
	if !definition.Supports("websocket", bench.ScenarioSingle) {
		t.Fatal("expected websocket single support")
	}
	if definition.Supports("websocket", bench.ScenarioBatch) {
		t.Fatal("did not expect websocket batch support without a verified native Nado batch endpoint")
	}
	if !definition.Supports("https", bench.ScenarioBatch) {
		t.Fatal("expected HTTPS manual fanout batch support")
	}
	if definition.DefaultWSURL != DefaultWSURL {
		t.Fatalf("default ws url = %s", definition.DefaultWSURL)
	}
}

func TestClassifyNadoResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
		want lifecycle.ClassificationStatus
	}{
		{
			name: "success",
			body: `{"status":"success","data":{"digest":"0xabc"},"request_type":"execute_place_order"}`,
			want: lifecycle.StatusAccepted,
		},
		{
			name: "failure",
			body: `{"status":"failure","error":"price invalid","request_type":"execute_place_order"}`,
			want: lifecycle.StatusRejected,
		},
		{
			name: "signature",
			body: `{"status":"failure","error":"invalid signature","request_type":"execute_place_order"}`,
			want: lifecycle.StatusAuthError,
		},
		{
			name: "nonce",
			body: `{"status":"failure","error":"nonce recv_time expired","request_type":"execute_place_order"}`,
			want: lifecycle.StatusNonceError,
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
