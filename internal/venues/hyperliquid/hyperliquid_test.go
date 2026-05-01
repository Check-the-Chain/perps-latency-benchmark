package hyperliquid

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
	"perps-latency-benchmark/internal/venues/prebuilt"
	"perps-latency-benchmark/internal/venues/spec"
)

func TestDefinitionDocumentsOfficialEndpointsAndAliases(t *testing.T) {
	definition := Definition()

	if definition.Name != "hyperliquid" {
		t.Fatalf("Name = %q", definition.Name)
	}
	if definition.DefaultBaseURL != DefaultBaseURL {
		t.Fatalf("DefaultBaseURL = %q", definition.DefaultBaseURL)
	}
	if definition.DefaultHTTPPath != DefaultHTTPPath {
		t.Fatalf("DefaultHTTPPath = %q", definition.DefaultHTTPPath)
	}
	if definition.DefaultWSURL != DefaultWSURL {
		t.Fatalf("DefaultWSURL = %q", definition.DefaultWSURL)
	}
	if definition.WSHeartbeat.Message != WebSocketHeartbeatMessage {
		t.Fatalf("WSHeartbeat.Message = %q", definition.WSHeartbeat.Message)
	}
	if definition.WSHeartbeat.IdleAfter >= time.Minute {
		t.Fatalf("WSHeartbeat.IdleAfter = %s", definition.WSHeartbeat.IdleAfter)
	}

	wantAliases := []string{"hl", "hyper-liquid", "hyper_liquid"}
	for _, alias := range wantAliases {
		if !slices.Contains(definition.Aliases, alias) {
			t.Fatalf("Aliases = %v, missing %q", definition.Aliases, alias)
		}
	}

	wantNames := []string{"hyperliquid", "hl", "hyper_liquid"}
	names := definition.Names()
	for _, name := range wantNames {
		if !slices.Contains(names, name) {
			t.Fatalf("Names = %v, missing %q", names, name)
		}
	}

	wantDocs := []string{
		ExchangeEndpointDocsURL,
		WebSocketDocsURL,
		WebSocketPostRequestsDocsURL,
	}
	for _, doc := range wantDocs {
		if !slices.Contains(definition.Docs, doc) {
			t.Fatalf("Docs = %v, missing %q", definition.Docs, doc)
		}
	}

	notes := strings.Join(definition.Notes, "\n")
	for _, phrase := range []string{
		"POST /exchange",
		"method=post",
		"untimed ping",
		"request.type=action",
		"request.payload",
		"prebuilt body",
	} {
		if !strings.Contains(notes, phrase) {
			t.Fatalf("Notes = %q, missing %q", notes, phrase)
		}
	}
}

func TestBuildUsesDefaultExchangeEndpoint(t *testing.T) {
	venue, err := Definition().Build(spec.Config{
		Request: prebuilt.Config{
			Body: `{"action":{"type":"order"},"nonce":1,"signature":{"r":"0x1","s":"0x2","v":27}}`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := venue.Prepare(context.Background(), bench.ScenarioSingle, 7, 1)
	if err != nil {
		t.Fatal(err)
	}

	if prepared.Transport != "https" {
		t.Fatalf("Transport = %q", prepared.Transport)
	}
	if prepared.Request.Method != http.MethodPost {
		t.Fatalf("Method = %q", prepared.Request.Method)
	}
	if prepared.Request.URL != "https://api.hyperliquid.xyz/exchange" {
		t.Fatalf("URL = %q", prepared.Request.URL)
	}
	if string(prepared.Request.Body) != `{"action":{"type":"order"},"nonce":1,"signature":{"r":"0x1","s":"0x2","v":27}}` {
		t.Fatalf("Body = %s", prepared.Request.Body)
	}
}

func TestBuildKeepsWebSocketPostBodyPrebuilt(t *testing.T) {
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
		_ = conn.WriteMessage(messageType, []byte(`{"channel":"post","data":{"id":1,"response":{"type":"action","payload":{"status":"ok"}}}}`))
	}))
	defer server.Close()

	httpBody := `{"action":{"type":"order"},"nonce":1,"signature":{"r":"0x1","s":"0x2","v":27}}`
	wsBody := `{"method":"post","id":1,"request":{"type":"action","payload":{"action":{"type":"order"},"nonce":1,"signature":{"r":"0x1","s":"0x2","v":27}}}}`
	venue, err := Definition().Build(spec.Config{
		WSURL: "ws" + strings.TrimPrefix(server.URL, "http"),
		Request: prebuilt.Config{
			Transport: "websocket",
			Body:      httpBody,
			WSBody:    wsBody,
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
		if got != wsBody {
			t.Fatalf("received = %s", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for websocket request body")
	}
}
