package netlatency

import (
	"bytes"
	"context"
	"net/http"
)

type RequestTemplate struct {
	Method string
	URL    string
	Header http.Header
	Body   []byte
}

func (t RequestTemplate) NewRequest(ctx context.Context) (*http.Request, error) {
	method := t.Method
	if method == "" {
		method = http.MethodPost
	}

	req, err := http.NewRequestWithContext(ctx, method, t.URL, bytes.NewReader(t.Body))
	if err != nil {
		return nil, err
	}
	req.Header = t.Header.Clone()
	if len(t.Body) > 0 && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}
