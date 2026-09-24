package confirmutil

import (
	"fmt"
	"strconv"
	"time"

	"perps-latency-benchmark/internal/netlatency"
)

func Start(trace netlatency.Trace) time.Time {
	if trace.WroteRequestAtNS > 0 {
		return trace.StartedAt.Add(time.Duration(trace.WroteRequestAtNS))
	}
	if trace.RequestWriteNS > 0 {
		return trace.StartedAt.Add(time.Duration(trace.RequestWriteNS))
	}
	return trace.StartedAt
}

func Text(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		if value == nil {
			return ""
		}
		return fmt.Sprint(value)
	}
}

func IDSet(value any) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, item := range AnySlice(value) {
		id := Text(item)
		if id != "" {
			ids[id] = struct{}{}
		}
	}
	return ids
}

func HasID(ids map[string]struct{}, values ...any) bool {
	for _, value := range values {
		if _, ok := ids[Text(value)]; ok {
			return true
		}
	}
	return false
}

func CopyIDSet(ids map[string]struct{}) map[string]struct{} {
	copied := make(map[string]struct{}, len(ids))
	for id := range ids {
		copied[id] = struct{}{}
	}
	return copied
}

func FirstMatchingID(ids map[string]struct{}, values ...any) string {
	for _, value := range values {
		id := Text(value)
		if _, ok := ids[id]; ok {
			return id
		}
	}
	return ""
}

func ObjectList(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if obj, ok := item.(map[string]any); ok {
			out = append(out, obj)
		}
	}
	return out
}

func Object(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}

func AnySlice(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []int:
		out := make([]any, len(typed))
		for i, value := range typed {
			out[i] = value
		}
		return out
	default:
		return nil
	}
}
