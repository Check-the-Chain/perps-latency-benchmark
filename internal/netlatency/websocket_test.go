package netlatency

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	if result.Trace.WroteRequestAtNS <= 0 {
		t.Fatal("expected websocket write timestamp")
	}
	if result.BytesRead == 0 {
		t.Fatal("expected response bytes")
	}
	if string(result.Body) != `{"ping":true}` {
		t.Fatalf("body = %s", result.Body)
	}
}

func TestWebSocketClientHeartbeatRunsBeforeIdleRequest(t *testing.T) {
	messages := make(chan string, 4)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()

		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			messages <- string(data)
			switch string(data) {
			case `{"method":"ping"}`:
				if err := conn.WriteMessage(msgType, []byte(`{"channel":"pong"}`)); err != nil {
					return
				}
			default:
				if err := conn.WriteMessage(msgType, data); err != nil {
					return
				}
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := NewWebSocketClientWithHeartbeat(wsURL, nil, false, WebSocketHeartbeat{
		Message:   []byte(`{"method":"ping"}`),
		IdleAfter: 50 * time.Millisecond,
		Timeout:   time.Second,
	})
	defer client.Close()

	if _, err := client.Do(context.Background(), []byte(`{"order":1}`)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(75 * time.Millisecond)
	result, err := client.Do(context.Background(), []byte(`{"order":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Body) != `{"order":2}` {
		t.Fatalf("body = %s", result.Body)
	}

	want := []string{`{"order":1}`, `{"method":"ping"}`, `{"order":2}`}
	for _, expected := range want {
		select {
		case got := <-messages:
			if got != expected {
				t.Fatalf("message = %s, want %s", got, expected)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %s", expected)
		}
	}
}
