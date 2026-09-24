package lifecycle

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type ClassificationStatus string

const (
	StatusAccepted       ClassificationStatus = "accepted"
	StatusRejected       ClassificationStatus = "rejected"
	StatusRateLimited    ClassificationStatus = "rate_limited"
	StatusAuthError      ClassificationStatus = "auth_error"
	StatusNonceError     ClassificationStatus = "nonce_error"
	StatusTransportError ClassificationStatus = "transport_error"
	StatusUnknown        ClassificationStatus = "unknown"
)

type Classification struct {
	Status ClassificationStatus `json:"status"`
	Reason string               `json:"reason,omitempty"`
}

type Classifier func(ResponseInput) Classification

func (c Classification) OK() bool {
	return c.Status == StatusAccepted || c.Status == StatusUnknown
}

type ResponseInput struct {
	StatusCode int
	Body       []byte
	Err        error
}

func ClassifyResponse(in ResponseInput) Classification {
	if in.Err != nil {
		return Classification{Status: StatusTransportError, Reason: in.Err.Error()}
	}
	if in.StatusCode == http.StatusTooManyRequests {
		return Classification{Status: StatusRateLimited, Reason: "http 429"}
	}
	if in.StatusCode == http.StatusUnauthorized || in.StatusCode == http.StatusForbidden {
		return Classification{Status: StatusAuthError, Reason: fmt.Sprintf("http %d", in.StatusCode)}
	}
	if in.StatusCode >= 400 {
		return classifyBody(in.Body, Classification{Status: StatusRejected, Reason: fmt.Sprintf("http %d", in.StatusCode)})
	}
	return classifyBody(in.Body, Classification{Status: StatusAccepted})
}

func classifyBody(body []byte, fallback Classification) Classification {
	if len(body) == 0 {
		return fallback
	}
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return fallback
	}
	finding := inspectValue(decoded)
	if finding == "" {
		return fallback
	}
	switch {
	case containsAny(finding, "rate limit", "too many request"):
		return Classification{Status: StatusRateLimited, Reason: finding}
	case containsAny(finding, "auth", "unauthor", "forbidden", "api key", "signature"):
		return Classification{Status: StatusAuthError, Reason: finding}
	case containsAny(finding, "nonce", "sequence"):
		return Classification{Status: StatusNonceError, Reason: finding}
	case containsAny(finding, "error", "reject", "failed", "invalid", "ok: false", "success: false"):
		return Classification{Status: StatusRejected, Reason: finding}
	default:
		return fallback
	}
}

func inspectValue(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"ok", "success"} {
			if child, ok := typed[key]; ok {
				if value, ok := child.(bool); ok && !value {
					return key + ": false"
				}
			}
		}
		for _, key := range []string{"error", "errors", "message", "reason", "status", "code"} {
			if child, ok := typed[key]; ok {
				if text := textValue(child); text != "" && looksNegative(text) {
					return text
				}
			}
		}
		for _, child := range typed {
			if text := inspectValue(child); text != "" {
				return text
			}
		}
	case []any:
		for _, child := range typed {
			if text := inspectValue(child); text != "" {
				return text
			}
		}
	case string:
		if looksNegative(typed) {
			return typed
		}
	}
	return ""
}

func textValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any, map[string]any:
		if text := inspectValue(typed); text != "" {
			return text
		}
	case float64:
		return fmt.Sprintf("%.0f", typed)
	case bool:
		if !typed {
			return "false"
		}
	}
	return ""
}

func looksNegative(text string) bool {
	if text == "" {
		return false
	}
	return containsAny(text, "error", "reject", "failed", "invalid", "ok: false", "success: false", "nonce", "rate limit", "too many request", "auth", "unauthor", "forbidden", "signature", "api key")
}

func containsAny(text string, needles ...string) bool {
	lower := strings.ToLower(text)
	for _, needle := range needles {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}
