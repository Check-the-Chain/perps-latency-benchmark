package payload

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"perps-latency-benchmark/internal/bench"
)

func TestCommandBuilder(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "builder.sh")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
input="$(cat)"
case "$input" in
  *'"iteration":7'*) iteration=7 ;;
  *) iteration=unknown ;;
esac
printf '{"headers":{"X-Iteration":"%s"},"body":"dynamic","metadata":{"scenario":"single"}}\n' "$iteration"
`), 0o755); err != nil {
		t.Fatal(err)
	}

	builder, err := NewCommandBuilder(CommandConfig{
		Command: []string{script},
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	built, err := builder.Build(context.Background(), Request{
		Venue:     "test",
		Transport: "http",
		Scenario:  bench.ScenarioSingle,
		Iteration: 7,
		BatchSize: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if built.Headers["X-Iteration"] != "7" {
		t.Fatalf("headers = %#v", built.Headers)
	}
	if built.Body == nil || *built.Body != "dynamic" {
		t.Fatalf("body = %#v", built.Body)
	}
	if built.Metadata["scenario"] != "single" {
		t.Fatalf("metadata = %#v", built.Metadata)
	}
}

func TestCommandBuilderTimeoutKillsProcess(t *testing.T) {
	builder, err := NewCommandBuilder(CommandConfig{
		Command: []string{"/bin/sh", "-c", "sleep 1; printf '{}'"},
		Timeout: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	_, err = builder.Build(context.Background(), Request{
		Venue:     "test",
		Transport: "http",
		Scenario:  bench.ScenarioSingle,
		Iteration: 1,
		BatchSize: 1,
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("builder did not time out promptly: %s", elapsed)
	}
}

func TestPersistentCommandBuilderReusesProcess(t *testing.T) {
	dir := t.TempDir()
	countFile := filepath.Join(dir, "starts")
	script := filepath.Join(dir, "builder.sh")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
printf start >> "`+countFile+`"
while IFS= read -r input; do
  case "$input" in
    *'"iteration":7'*) iteration=7 ;;
    *'"iteration":8'*) iteration=8 ;;
    *) iteration=unknown ;;
  esac
  printf '{"headers":{"X-Iteration":"%s"},"body":"dynamic-%s"}\n' "$iteration" "$iteration"
done
`), 0o755); err != nil {
		t.Fatal(err)
	}

	builder, err := NewPersistentCommandBuilder(CommandConfig{
		Command: []string{script},
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer builder.Close(context.Background())

	for _, iteration := range []int{7, 8} {
		built, err := builder.Build(context.Background(), Request{
			Venue:     "test",
			Transport: "http",
			Scenario:  bench.ScenarioSingle,
			Iteration: iteration,
			BatchSize: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		want := "dynamic-" + built.Headers["X-Iteration"]
		if built.Body == nil || *built.Body != want {
			t.Fatalf("body = %#v, want %q", built.Body, want)
		}
	}

	data, err := os.ReadFile(countFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "start" {
		t.Fatalf("process starts = %q", data)
	}
}

func TestResolveCommandUsesDirectPythonOverride(t *testing.T) {
	got := resolveCommand([]string{
		"uv",
		"run",
		"--with",
		"lighter-sdk",
		"python",
		"internal/venues/lighter/build_payload.py",
	}, map[string]string{
		"PERPS_BENCH_PYTHON": "/opt/perps/python/bin/python -I",
	})

	want := []string{"/opt/perps/python/bin/python", "-I", "internal/venues/lighter/build_payload.py"}
	if len(got) != len(want) {
		t.Fatalf("command len = %d, want %d: %#v", len(got), len(want), got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("command[%d] = %q, want %q: %#v", index, got[index], want[index], got)
		}
	}
}

func TestResolveCommandKeepsUVWithoutDirectPythonOverride(t *testing.T) {
	command := []string{
		"uv",
		"run",
		"--with",
		"eth-account",
		"python",
		"internal/venues/hyperliquid/cancel_payload.py",
	}
	got := resolveCommand(command, nil)
	if len(got) != len(command) {
		t.Fatalf("command len = %d, want %d", len(got), len(command))
	}
	for index := range command {
		if got[index] != command[index] {
			t.Fatalf("command[%d] = %q, want %q", index, got[index], command[index])
		}
	}
}
