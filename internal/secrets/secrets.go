package secrets

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var sensitiveFragments = []string{
	"apikey",
	"api_key",
	"authorization",
	"cookie",
	"mnemonic",
	"passphrase",
	"password",
	"privatekey",
	"private_key",
	"seed",
	"secret",
	"signature",
	"token",
}

var assignmentPattern = regexp.MustCompile(`(?i)(["']?)([A-Za-z0-9_.\-/]*(?:secret|private[_-]?key|api[_-]?key|authorization|token|cookie|signature|passphrase|password|mnemonic|seed)[A-Za-z0-9_.\-/]*)(["']?)(\s*[:=]\s*)(["']?)[^"',\s}]+(["']?)`)

type Finding struct {
	Path string
}

func (f Finding) Error() string {
	return f.Path
}

func ContainsSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
	compact := strings.ReplaceAll(normalized, "_", "")
	for _, fragment := range sensitiveFragments {
		if strings.Contains(normalized, fragment) || strings.Contains(compact, strings.ReplaceAll(fragment, "_", "")) {
			return true
		}
	}
	return false
}

func FindInlineSecrets(value any) ([]Finding, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, err
	}
	var findings []Finding
	findInlineSecrets(decoded, "$", &findings)
	sort.Slice(findings, func(i, j int) bool {
		return findings[i].Path < findings[j].Path
	})
	return findings, nil
}

func findInlineSecrets(value any, path string, findings *[]Finding) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childPath := path + "." + key
			if ContainsSensitiveKey(key) && hasValue(child) {
				*findings = append(*findings, Finding{Path: childPath})
				continue
			}
			findInlineSecrets(child, childPath, findings)
		}
	case []any:
		for index, child := range typed {
			findInlineSecrets(child, fmt.Sprintf("%s[%d]", path, index), findings)
		}
	}
}

func hasValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return typed != ""
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return true
	}
}

func RedactString(input string) string {
	return assignmentPattern.ReplaceAllString(input, `$1$2$3$4$5[REDACTED]$6`)
}

func RedactValue(value any) any {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return value
	}
	return redactDecoded(decoded, "")
}

func redactDecoded(value any, key string) any {
	if ContainsSensitiveKey(key) && hasValue(value) {
		return "[REDACTED]"
	}
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for childKey, child := range typed {
			out[childKey] = redactDecoded(child, childKey)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, child := range typed {
			out[index] = redactDecoded(child, key)
		}
		return out
	case string:
		return RedactString(typed)
	default:
		return typed
	}
}
