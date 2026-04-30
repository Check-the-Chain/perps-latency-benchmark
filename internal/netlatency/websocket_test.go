package netlatency

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestWebSocketClientDo(t *testing.T) {
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
		if err := conn.WriteMessage(msgType, data); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := NewWebSocketClient(wsURL, nil, false)
	defer client.Close()

	result, err := client.Do(context.Background(), []byte(`{"ping":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Trace.Transport != "websocket" {
		t.Fatalf("transport = %s", result.Trace.Transport)
	}
	if result.Trace.TotalNS <= 0 {
		t.Fatal("expected total duration")
	}
	if result.BytesRead == 0 {
		t.Fatal("expected response bytes")
	}
	if string(result.Body) != `{"ping":true}` {
		t.Fatalf("body = %s", result.Body)
	}
}
