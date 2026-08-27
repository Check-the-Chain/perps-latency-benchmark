package bullet

import (
	"testing"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/lifecycle"
)

func TestDefinitionSupportsBothTransports(t *testing.T) {
	definition := Definition()
	if definition.Name != "bullet" {
		t.Fatalf("name = %q, want bullet", definition.Name)
	}
	if !definition.Supports("websocket", bench.ScenarioSingle) {
		t.Fatal("websocket single must be supported")
	}
	if !definition.Supports("websocket", bench.ScenarioBatch) {
		t.Fatal("websocket batch must be supported")
	}
	// HTTP POST /tx/submit carries the identical signed transaction, so both
	// transports are benchmarkable and comparable.
	if !definition.Supports("http", bench.ScenarioSingle) {
		t.Fatal("http single must be supported")
	}
	if !definition.Supports("http", bench.ScenarioBatch) {
		t.Fatal("http batch must be supported")
	}
	if !definition.WSReadInitial {
		t.Fatal("Bullet sends a status frame on connect; WSReadInitial must be true")
	}
}

func TestClassifyAcceptsProcessedStatus(t *testing.T) {
	body := []byte(`{"e":"order.place","id":10,"E":1706745600000000,"results":{"tx_id":"0xabc","status":"processed","order_ids":[1],"client_order_ids":[12345]}}`)
	got := Classify(lifecycle.ResponseInput{Body: body})
	if got.Status != lifecycle.StatusAccepted {
		t.Fatalf("status = %q, want accepted (reason %q)", got.Status, got.Reason)
	}
}

func TestClassifyRejectsDroppedStatus(t *testing.T) {
	body := []byte(`{"e":"order.place","id":10,"results":{"tx_id":"0xabc","status":"dropped","order_ids":[],"client_order_ids":[]}}`)
	got := Classify(lifecycle.ResponseInput{Body: body})
	if got.Status != lifecycle.StatusRejected {
		t.Fatalf("status = %q, want rejected", got.Status)
	}
}

// Bullet returns "submitted" on the happy path for every accepted order — the
// 2026-08-15 testnet run classified 100% of successful samples this way. Treating
// it as unknown left every sample in the unknown bucket, so it is accepted here.
// Book entry is still verified independently by the ws_confirmation match.
func TestClassifySubmittedStatusIsAccepted(t *testing.T) {
	body := []byte(`{"e":"order.place","id":10,"results":{"tx_id":"0xabc","status":"submitted","order_ids":[1],"client_order_ids":[12345]}}`)
	got := Classify(lifecycle.ResponseInput{Body: body})
	if got.Status != lifecycle.StatusAccepted {
		t.Fatalf("status = %q, want accepted", got.Status)
	}
	if got.Reason == "" {
		t.Fatal("reason must record that this is a pre-publication ack")
	}
}

func TestClassifyUnrecognizedStatusIsUnknown(t *testing.T) {
	body := []byte(`{"e":"order.place","id":10,"results":{"tx_id":"0xabc","status":"teleported","order_ids":[1],"client_order_ids":[12345]}}`)
	got := Classify(lifecycle.ResponseInput{Body: body})
	if got.Status != lifecycle.StatusUnknown {
		t.Fatalf("status = %q, want unknown", got.Status)
	}
}

func TestClassifyErrorTakesPrecedenceOverResults(t *testing.T) {
	body := []byte(`{"id":12,"error":{"code":-2010,"msg":"new order rejected"},"results":{"tx_id":"0xabc","status":"processed","order_ids":[1],"client_order_ids":[12345]}}`)
	got := Classify(lifecycle.ResponseInput{Body: body})
	if got.Status != lifecycle.StatusRejected {
		t.Fatalf("status = %q, want rejected (error must win over results)", got.Status)
	}
}

func TestClassifyNeitherErrorNorResultsDoesNotReportAccepted(t *testing.T) {
	body := []byte(`{"e":"pong","id":4}`)
	got := Classify(lifecycle.ResponseInput{Body: body})
	if got.Status == lifecycle.StatusAccepted {
		t.Fatalf("status = %q, must not be accepted for a bare pong frame", got.Status)
	}
}

func TestClassifyMapsErrorCodes(t *testing.T) {
	cases := []struct {
		name string
		body string
		want lifecycle.ClassificationStatus
	}{
		{"new order rejected", `{"id":12,"error":{"code":-2010,"msg":"new order rejected: insufficient margin"}}`, lifecycle.StatusRejected},
		{"rate limited", `{"id":12,"error":{"code":-1003,"msg":"rate limit exceeded"}}`, lifecycle.StatusRateLimited},
		{"too many orders", `{"id":12,"error":{"code":-1015,"msg":"order rate limit"}}`, lifecycle.StatusRateLimited},
		{"invalid signature", `{"id":12,"error":{"code":-1022,"msg":"signature invalid"}}`, lifecycle.StatusAuthError},
		{"invalid timestamp", `{"id":12,"error":{"code":-1021,"msg":"bad timestamp"}}`, lifecycle.StatusNonceError},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := Classify(lifecycle.ResponseInput{Body: []byte(testCase.body)})
			if got.Status != testCase.want {
				t.Fatalf("status = %q, want %q", got.Status, testCase.want)
			}
			if got.Reason == "" {
				t.Fatal("reason must carry the error message")
			}
		})
	}
}

// HTTP POST /tx/submit returns SubmitTxResponse with id and status at the top
// level rather than the WebSocket error/results envelope. Before this was
// handled, every successful HTTP sample classified as unknown and an HTTP
// "dropped" was never rejected.
func TestClassifyHTTPSubmitEnvelope(t *testing.T) {
	cases := []struct {
		name string
		body string
		want lifecycle.ClassificationStatus
	}{
		{"processed", `{"id":"0xabc","status":"processed","tx_number":7}`, lifecycle.StatusAccepted},
		{"submitted", `{"id":"0xabc","status":"submitted"}`, lifecycle.StatusAccepted},
		{"dropped", `{"id":"0xabc","status":"dropped"}`, lifecycle.StatusRejected},
		{"reverted receipt", `{"id":"0xabc","status":"processed","receipt":{"result":"reverted"}}`, lifecycle.StatusRejected},
		{"skipped receipt", `{"id":"0xabc","status":"processed","receipt":{"result":"skipped"}}`, lifecycle.StatusRejected},
		{"successful receipt", `{"id":"0xabc","status":"processed","receipt":{"result":"successful"}}`, lifecycle.StatusAccepted},
		{"flat http error", `{"status":400,"message":"Bad request: invalid tx"}`, lifecycle.StatusRejected},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := Classify(lifecycle.ResponseInput{Body: []byte(testCase.body)})
			if got.Status != testCase.want {
				t.Fatalf("status = %q, want %q (reason %q)", got.Status, testCase.want, got.Reason)
			}
		})
	}
}

// HTTP status codes must survive into the classification. Decoding past a
// non-OK generic verdict would collapse 429 into rejected and, for the
// IP-allowlisted MM ingress, 403 into rejected — hiding exactly the signals an
// operator needs. Matches how every other venue guards Classify.
func TestClassifyPreservesHTTPStatusCategory(t *testing.T) {
	cases := []struct {
		name string
		code int
		body string
		want lifecycle.ClassificationStatus
	}{
		{"rate limited", 429, `{"status":429,"message":"rate limited"}`, lifecycle.StatusRateLimited},
		{"mm ingress not allowlisted", 403, `{"status":403,"message":"forbidden"}`, lifecycle.StatusAuthError},
		{"bad request", 400, `{"status":400,"message":"Bad request: invalid tx"}`, lifecycle.StatusRejected},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := Classify(lifecycle.ResponseInput{StatusCode: testCase.code, Body: []byte(testCase.body)})
			if got.Status != testCase.want {
				t.Fatalf("status = %q, want %q (reason %q)", got.Status, testCase.want, got.Reason)
			}
		})
	}
}
