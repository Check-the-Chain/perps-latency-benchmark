package lifecycle

import (
	"errors"
	"net/http"
	"testing"
)

func TestClassifyResponse(t *testing.T) {
	tests := []struct {
		name string
		in   ResponseInput
		want ClassificationStatus
	}{
		{
			name: "accepted empty body",
			in:   ResponseInput{StatusCode: http.StatusOK},
			want: StatusAccepted,
		},
		{
			name: "rate limited status",
			in:   ResponseInput{StatusCode: http.StatusTooManyRequests},
			want: StatusRateLimited,
		},
		{
			name: "auth status",
			in:   ResponseInput{StatusCode: http.StatusForbidden},
			want: StatusAuthError,
		},
		{
			name: "nonce body",
			in:   ResponseInput{StatusCode: http.StatusOK, Body: []byte(`{"error":"bad nonce"}`)},
			want: StatusNonceError,
		},
		{
			name: "rejected body",
			in:   ResponseInput{StatusCode: http.StatusOK, Body: []byte(`{"status":"rejected","reason":"invalid order"}`)},
			want: StatusRejected,
		},
		{
			name: "ok false body",
			in:   ResponseInput{StatusCode: http.StatusOK, Body: []byte(`{"ok":false}`)},
			want: StatusRejected,
		},
		{
			name: "transport error",
			in:   ResponseInput{Err: errors.New("dial failed")},
			want: StatusTransportError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyResponse(tt.in)
			if got.Status != tt.want {
				t.Fatalf("status = %s, want %s", got.Status, tt.want)
			}
		})
	}
}
