package prebuilt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/payload"
)

func TestPrebuiltVenueUsesBatchOverrides(t *testing.T) {
	venue, err := New(Config{
		Name:      "test",
		URL:       "https://example.com/single",
		BatchURL:  "https://example.com/batch",
		Body:      `{"single":true}`,
		BatchBody: `{"batch":true}`,
		Headers:   map[string]string{"X-Test": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := venue.Prepare(context.Background(), bench.ScenarioBatch, 0, 3)
	if err != nil {
		t.Fatal(err)
	}

	if prepared.Request.URL != "https://example.com/batch" {
		t.Fatalf("url = %s", prepared.Request.URL)
	}
	if string(prepared.Request.Body) != `{"batch":true}` {
		t.Fatalf("body = %s", prepared.Request.Body)
	}
	if prepared.Request.Header.Get("X-Test") != "1" {
		t.Fatalf("header = %s", prepared.Request.Header.Get("X-Test"))
	}
	if prepared.Request.Method != http.MethodPost {
		t.Fatalf("method = %s", prepared.Request.Method)
	}
}

func TestPrebuiltVenueUsesDynamicBuilder(t *testing.T) {
	dynamicBody := `{"dynamic":true}`
	venue, err := New(Config{
		Name: "test",
		URL:  "https://example.com/single",
		Body: `{"static":true}`,
		Builder: staticBuilder{built: payload.Built{
			Headers:  map[string]string{"X-Test": "dynamic"},
			Body:     &dynamicBody,
			Metadata: map[string]any{"builder": "static"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := venue.Prepare(context.Background(), bench.ScenarioSingle, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if string(prepared.Request.Body) != dynamicBody {
		t.Fatalf("body = %s", prepared.Request.Body)
	}
	if prepared.Request.Header.Get("X-Test") != "dynamic" {
		t.Fatalf("headers = %#v", prepared.Request.Header)
	}
	if prepared.Metadata["builder"] != "static" {
		t.Fatalf("metadata = %#v", prepared.Metadata)
	}
}

func TestPrebuiltVenueAllowsBuilderWithoutStaticBody(t *testing.T) {
	dynamicBody := `{"dynamic":true}`
	venue, err := New(Config{
		Name: "test",
		URL:  "https://example.com/single",
		Builder: staticBuilder{built: payload.Built{
			Body: &dynamicBody,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := venue.Prepare(context.Background(), bench.ScenarioSingle, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if string(prepared.Request.Body) != dynamicBody {
		t.Fatalf("body = %s", prepared.Request.Body)
	}
}

func TestPrebuiltVenueUsesBuilderBodyForBatchScenario(t *testing.T) {
	dynamicBody := `{"batch":true}`
	venue, err := New(Config{
		Name:     "test",
		URL:      "https://example.com/single",
		BatchURL: "https://example.com/batch",
		Builder: staticBuilder{built: payload.Built{
			Body: &dynamicBody,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := venue.Prepare(context.Background(), bench.ScenarioBatch, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	if string(prepared.Request.Body) != dynamicBody {
		t.Fatalf("body = %s", prepared.Request.Body)
	}
	if prepared.Request.URL != "https://example.com/batch" {
		t.Fatalf("url = %s", prepared.Request.URL)
	}
}

type staticBuilder struct {
	built payload.Built
}

func (b staticBuilder) Build(context.Context, payload.Request) (payload.Built, error) {
	return b.built, nil
}

func TestPrebuiltVenueUsesSeparateWebSocketBody(t *testing.T) {
	received := make(chan string, 1)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()

		msgType, data, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("read: %v", err)
			return
		}
		received <- string(data)
		_ = conn.WriteMessage(msgType, []byte(`{"ok":true}`))
	}))
	defer server.Close()

	venue, err := New(Config{
		Name:      "test",
		Transport: "websocket",
		WSURL:     "ws" + strings.TrimPrefix(server.URL, "http"),
		Body:      `{"http":true}`,
		WSBody:    `{"ws":true}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer venue.Close(context.Background())

	prepared, err := venue.Prepare(context.Background(), bench.ScenarioSingle, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := <-received; got != `{"ws":true}` {
		t.Fatalf("received = %s", got)
	}
}

func TestPrebuiltVenueCanDrainInitialWebSocketMessage(t *testing.T) {
	received := make(chan string, 1)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()

		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"connected"}`))
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("read: %v", err)
			return
		}
		received <- string(data)
		_ = conn.WriteMessage(msgType, []byte(`{"ok":true}`))
	}))
	defer server.Close()

	venue, err := New(Config{
		Name:          "test",
		Transport:     "websocket",
		WSURL:         "ws" + strings.TrimPrefix(server.URL, "http"),
		WSBody:        `{"ws":true}`,
		WSReadInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer venue.Close(context.Background())

	prepared, err := venue.Prepare(context.Background(), bench.ScenarioSingle, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := <-received; got != `{"ws":true}` {
		t.Fatalf("received = %s", got)
	}
}

func TestPrebuiltVenueUsesBuilderBodyForWebSocket(t *testing.T) {
	received := make(chan string, 1)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()

		msgType, data, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("read: %v", err)
			return
		}
		received <- string(data)
		_ = conn.WriteMessage(msgType, []byte(`{"ok":true}`))
	}))
	defer server.Close()

	dynamicBody := `{"dynamic":true}`
	venue, err := New(Config{
		Name:      "test",
		Transport: "websocket",
		WSURL:     "ws" + strings.TrimPrefix(server.URL, "http"),
		Builder: staticBuilder{built: payload.Built{
			Body: &dynamicBody,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer venue.Close(context.Background())

	prepared, err := venue.Prepare(context.Background(), bench.ScenarioSingle, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := <-received; got != dynamicBody {
		t.Fatalf("received = %s", got)
	}
}
