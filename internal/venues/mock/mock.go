package mock

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/netlatency"
)

type Config struct {
	Latency   time.Duration
	Status    int
	Body      string
	Transport string
}

type Venue struct {
	server    *httptest.Server
	transport string
	wsClient  *netlatency.WebSocketClient
}

func New(cfg Config) *Venue {
	status := cfg.Status
	if status == 0 {
		status = http.StatusOK
	}
	body := cfg.Body
	if body == "" {
		body = `{"ok":true}`
	}

	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			for {
				msgType, data, err := conn.ReadMessage()
				if err != nil {
					return
				}
				if cfg.Latency > 0 {
					time.Sleep(cfg.Latency)
				}
				if len(data) == 0 {
					data = []byte(body)
				}
				if err := conn.WriteMessage(msgType, data); err != nil {
					return
				}
			}
		}
		if cfg.Latency > 0 {
			time.Sleep(cfg.Latency)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))

	transport := strings.ToLower(cfg.Transport)
	if transport == "" || transport == "https" {
		transport = "http"
	}
	return &Venue{
		server:    server,
		transport: transport,
		wsClient:  netlatency.NewWebSocketClient("ws"+strings.TrimPrefix(server.URL, "http"), nil),
	}
}

func (v *Venue) Name() string {
	return "mock"
}

func (v *Venue) Prepare(_ context.Context, scenario bench.Scenario, iteration int, batchSize int) (bench.PreparedRequest, error) {
	body := fmt.Sprintf(`{"scenario":%q,"iteration":%d,"batch_size":%d}`, scenario, iteration, batchSize)
	if v.transport == "websocket" {
		return bench.PreparedRequest{
			Transport: "websocket",
			Execute: func(ctx context.Context) (netlatency.Result, error) {
				return v.wsClient.Do(ctx, []byte(body))
			},
			Metadata: map[string]any{"local_mock": true},
		}, nil
	}
	return bench.PreparedRequest{
		Transport: "http",
		Request: netlatency.RequestTemplate{
			Method: http.MethodPost,
			URL:    v.server.URL,
			Header: http.Header{"Content-Type": []string{"application/json"}},
			Body:   []byte(body),
		},
		Metadata: map[string]any{"local_mock": true},
	}, nil
}

func (v *Venue) Close(context.Context) error {
	if v.wsClient != nil {
		_ = v.wsClient.Close()
	}
	v.server.Close()
	return nil
}
