package netlatency

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientDoReadsResponseAndRecordsTrace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(time.Millisecond)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{})
	template := RequestTemplate{
		Method: http.MethodPost,
		URL:    server.URL,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   []byte(`{"ping":true}`),
	}

	result, err := client.Do(context.Background(), template)
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", result.StatusCode)
	}
	if result.BytesRead == 0 {
		t.Fatal("expected response bytes to be read")
	}
	if string(result.Body) != `{"ok":true}` {
		t.Fatalf("body = %s", result.Body)
	}
	if result.Trace.TotalNS <= 0 {
		t.Fatal("expected total trace duration")
	}
	if result.Trace.TTFBNS <= 0 {
		t.Fatal("expected ttfb trace duration")
	}
}
