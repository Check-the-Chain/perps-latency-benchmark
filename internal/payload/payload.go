package payload

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"perps-latency-benchmark/internal/bench"
)

type Request struct {
	Venue       string         `json:"venue"`
	Transport   string         `json:"transport"`
	Scenario    bench.Scenario `json:"scenario"`
	Iteration   int            `json:"iteration"`
	BatchSize   int            `json:"batch_size"`
	RequestedAt time.Time      `json:"requested_at"`
	Params      map[string]any `json:"params,omitempty"`
}

type Built struct {
	Method            string               `json:"method,omitempty"`
	URL               string               `json:"url,omitempty"`
	BatchURL          string               `json:"batch_url,omitempty"`
	WSURL             string               `json:"ws_url,omitempty"`
	WSBatchURL        string               `json:"ws_batch_url,omitempty"`
	Headers           map[string]string    `json:"headers,omitempty"`
	Body              *string              `json:"body,omitempty"`
	BodyBase64        string               `json:"body_base64,omitempty"`
	BatchBody         *string              `json:"batch_body,omitempty"`
	BatchBodyBase64   string               `json:"batch_body_base64,omitempty"`
	WSBody            *string              `json:"ws_body,omitempty"`
	WSBodyBase64      string               `json:"ws_body_base64,omitempty"`
	WSBatchBody       *string              `json:"ws_batch_body,omitempty"`
	WSBatchBodyBase64 string               `json:"ws_batch_body_base64,omitempty"`
	Metadata          map[string]any       `json:"metadata,omitempty"`
	Cleanup           *bench.CleanupResult `json:"cleanup,omitempty"`
}

type Builder interface {
	Build(context.Context, Request) (Built, error)
}

type Closer interface {
	Close(context.Context) error
}

func MergeHeaders(base http.Header, overlay map[string]string) http.Header {
	merged := base.Clone()
	for key, value := range overlay {
		merged.Set(key, value)
	}
	return merged
}

func Bytes(text *string, encoded string, fallback []byte) ([]byte, error) {
	if encoded != "" {
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("decode base64 payload: %w", err)
		}
		return decoded, nil
	}
	if text != nil {
		return []byte(*text), nil
	}
	return append([]byte(nil), fallback...), nil
}

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
