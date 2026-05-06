package confirmutil

import (
	"testing"
	"time"

	"perps-latency-benchmark/internal/netlatency"
)

func TestStartUsesCompletedWriteTime(t *testing.T) {
	start := time.Unix(10, 0).UTC()
	got := Start(netlatency.Trace{StartedAt: start, WroteRequestAtNS: int64(2 * time.Millisecond)})
	if want := start.Add(2 * time.Millisecond); !got.Equal(want) {
		t.Fatalf("Start = %s, want %s", got, want)
	}
}

func TestIDSetCoercesNumericIDs(t *testing.T) {
	ids := IDSet([]any{"abc", float64(123), int64(456)})
	for _, want := range []string{"abc", "123", "456"} {
		if _, ok := ids[want]; !ok {
			t.Fatalf("IDSet missing %q: %#v", want, ids)
		}
	}
}
