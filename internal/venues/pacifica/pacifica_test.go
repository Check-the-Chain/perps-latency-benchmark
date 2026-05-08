package pacifica

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/venues/prebuilt"
	"perps-latency-benchmark/internal/venues/spec"
)

func TestDefinitionDocumentsPacificaDefaults(t *testing.T) {
	definition := Definition()

	if definition.Name != "pacifica" {
		t.Fatalf("Name = %q", definition.Name)
	}
	if definition.DefaultBaseURL != DefaultBaseURL {
		t.Fatalf("DefaultBaseURL = %q", definition.DefaultBaseURL)
	}
	if definition.DefaultWSURL != DefaultWSURL {
		t.Fatalf("DefaultWSURL = %q", definition.DefaultWSURL)
	}
	if !definition.Capabilities.WebSocketSingle || !definition.Capabilities.WebSocketBatch {
		t.Fatal("expected websocket single and batch capabilities")
	}
	if !definition.Capabilities.Cleanup {
		t.Fatal("expected cleanup capability")
	}
	if definition.Confirmation == nil || definition.CancelConfirmation == nil {
		t.Fatal("expected confirmation factories")
	}
	for _, doc := range []string{WebSocketLimitOrderDocsURL, WebSocketBatchOrderDocsURL, SigningDocsURL, RateLimitsDocsURL} {
		if !slices.Contains(definition.Docs, doc) {
			t.Fatalf("Docs = %v, missing %q", definition.Docs, doc)
		}
	}
	notes := strings.Join(definition.Notes, "\n")
	for _, phrase := range []string{"WebSocket order submission", "TIF=ALO", "200 ms", "50-100 ms", "API Agent Keys"} {
		if !strings.Contains(notes, phrase) {
			t.Fatalf("Notes = %q, missing %q", notes, phrase)
		}
	}
}

func TestBuildKeepsPacificaWebSocketBodyPrebuilt(t *testing.T) {
	received := make(chan string, 1)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("read: %v", err)
			return
		}
		received <- string(data)
		_ = conn.WriteMessage(messageType, []byte(`{"code":200,"data":{"I":"pb_123","i":1,"s":"BTC"},"type":"create_order"}`))
	}))
	defer server.Close()

	venue, err := Definition().Build(spec.Config{
		WSURL: "ws" + strings.TrimPrefix(server.URL, "http"),
		Request: prebuilt.Config{
			Transport: "websocket",
			WSBody:    `{"id":"1","params":{"create_order":{"symbol":"BTC"}}}`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer venue.Close(context.Background())

	prepared, err := venue.Prepare(context.Background(), bench.ScenarioSingle, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Transport != "websocket" {
		t.Fatalf("Transport = %q", prepared.Transport)
	}
	if _, err := prepared.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-received:
		if got != `{"id":"1","params":{"create_order":{"symbol":"BTC"}}}` {
			t.Fatalf("received = %s", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for websocket request body")
	}
}

func TestClassifyPacificaError(t *testing.T) {
	got := Classify(lifecycle.ResponseInput{
		StatusCode: 200,
		Body:       []byte(`{"code":400,"error":"bad request"}`),
	})
	if got.Status != lifecycle.StatusRejected {
		t.Fatalf("status = %s, want rejected", got.Status)
	}
}

func TestMatchPacificaConfirmation(t *testing.T) {
	ok, err := matchPacificaConfirmation(map[string]any{
		"channel": "account_order_updates",
		"data": []any{
			map[string]any{"I": "pb_123", "oe": "make", "os": "open"},
		},
	}, map[string]struct{}{"pb_123": {}}, "post_only")
	if err != nil {
		t.Fatalf("matchPacificaConfirmation error = %v", err)
	}
	if !ok {
		t.Fatal("expected confirmation match")
	}
}

func TestMatchPacificaConfirmationRejectsTerminalFailure(t *testing.T) {
	ok, err := matchPacificaConfirmation(map[string]any{
		"channel": "account_order_updates",
		"data": []any{
			map[string]any{"I": "pb_123", "oe": "post_only_rejected", "os": "rejected"},
		},
	}, map[string]struct{}{"pb_123": {}}, "post_only")
	if err == nil {
		t.Fatal("expected terminal failure error")
	}
	if ok {
		t.Fatal("expected no successful match")
	}
}

func TestMatchPacificaCancelConfirmationWaitsForAllOrders(t *testing.T) {
	remaining := map[string]struct{}{"pb_123": {}, "pb_124": {}}
	first := matchPacificaCancelConfirmation(map[string]any{
		"channel": "account_order_updates",
		"data":    []any{map[string]any{"I": "pb_123", "oe": "cancel", "os": "cancelled"}},
	}, remaining)
	if first {
		t.Fatal("expected first cancel update to leave one order outstanding")
	}
	second := matchPacificaCancelConfirmation(map[string]any{
		"channel": "account_order_updates",
		"data":    []any{map[string]any{"I": "pb_124", "oe": "cancel", "os": "cancelled"}},
	}, remaining)
	if !second {
		t.Fatalf("expected all cancels confirmed, remaining = %#v", remaining)
	}
}
