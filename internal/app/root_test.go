package app

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunMockCommand(t *testing.T) {
	var stdout bytes.Buffer

	cmd := NewRootCommand()
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"run", "--venue", "mock", "--iterations", "1", "--mock-latency-ms", "0"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "venue=mock") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestRunLiveRequiresConfirmation(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"run", "--venue", "http", "--url", "https://example.com", "--body", "{}"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected confirmation error")
	}
	if !strings.Contains(err.Error(), "--confirm-live") {
		t.Fatalf("error = %v", err)
	}
}

func TestCompareTransportsMockCommand(t *testing.T) {
	var stdout bytes.Buffer
	dir := t.TempDir()
	out := filepath.Join(dir, "comparison.json")

	cmd := NewRootCommand()
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{
		"compare-transports",
		"--venue",
		"mock",
		"--iterations",
		"1",
		"--transports",
		"https,websocket",
		"--run-id",
		"compare-test",
		"--output",
		out,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "[https]") || !strings.Contains(stdout.String(), "[websocket]") {
		t.Fatalf("stdout = %s", stdout.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"run_id": "compare-test"`, `"run_id": "compare-test-https"`, `"run_id": "compare-test-websocket"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("comparison output missing %s:\n%s", want, string(data))
		}
	}
}

func TestCompareResultsCommand(t *testing.T) {
	dir := t.TempDir()
	resultPath := filepath.Join(dir, "result.json")
	if err := os.WriteFile(resultPath, []byte(`{
  "venue": "mock",
  "scenario": "single",
  "latency_mode": "total",
  "reconciliation": {"attempted": true, "ok": true},
  "samples": [
    {"venue": "mock", "scenario": "single", "transport": "https", "network_ns": 1000000, "ok": true, "cleanup": {"attempted": true, "ok": true}},
    {"venue": "mock", "scenario": "single", "transport": "https", "network_ns": 2000000, "ok": true, "cleanup": {"attempted": true, "ok": true}}
  ]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"compare-results", resultPath})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	for _, want := range []string{"venue", "mock", "https", "2", "1.500", "2/2", "ok"} {
		if !strings.Contains(got, want) {
			t.Fatalf("compare output missing %q:\n%s", want, got)
		}
	}
}

func TestRunCommandBuilderConfig(t *testing.T) {
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	script := filepath.Join(dir, "builder.sh")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
printf '{"headers":{"X-Builder":"test"},"body":"dynamic-body"}\n'
`), 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "config.json")
	if err := os.WriteFile(config, []byte(`{
  "venue": "http",
  "benchmark": {"iterations": 1},
  "request": {
    "url": "`+server.URL+`",
    "body": "static-body",
    "builder": {
      "type": "command",
      "command": ["`+script+`"],
      "timeout_ms": 5000
    }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"run", "--config", config, "--confirm-live"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := <-received; got != "dynamic-body" {
		t.Fatalf("body = %q", got)
	}
}

func TestRunLoadsEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env.hyperliquid.local")
	if err := os.WriteFile(envFile, []byte("BENCH_TEST_ENV=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldEnv, hadEnv := os.LookupEnv("BENCH_TEST_ENV")
	if err := os.Unsetenv("BENCH_TEST_ENV"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if hadEnv {
			_ = os.Setenv("BENCH_TEST_ENV", oldEnv)
		} else {
			_ = os.Unsetenv("BENCH_TEST_ENV")
		}
	}()

	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	script := filepath.Join(dir, "builder.sh")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
printf '{"body":"%s"}\n' "$BENCH_TEST_ENV"
`), 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "config.json")
	if err := os.WriteFile(config, []byte(`{
  "venue": "http",
  "benchmark": {"iterations": 1},
  "request": {
    "url": "`+server.URL+`",
    "builder": {
      "type": "command",
      "command": ["`+script+`"]
    }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"run", "--config", config, "--env-file", envFile, "--confirm-live"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := <-received; got != "from-file" {
		t.Fatalf("body = %q", got)
	}
}

func TestRunRejectsInlineSecrets(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	if err := os.WriteFile(config, []byte(`{
  "venue": "http",
  "benchmark": {"iterations": 1},
  "request": {
    "url": "https://example.com",
    "builder": {
      "type": "command",
      "command": ["echo", "{}"],
      "params": {"private_key": "do-not-commit"}
    }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"run", "--config", config, "--confirm-live"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected inline secret rejection")
	}
	if !strings.Contains(err.Error(), "inline secrets") || !strings.Contains(err.Error(), "private_key") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunRejectsFillLikelyProfileWithoutLifecycleAdapter(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	if err := os.WriteFile(config, []byte(`{
  "venue": "http",
  "risk": {"allow_fill": true, "neutralize_on_fill": true},
  "benchmark": {"iterations": 1},
  "request": {
    "url": "https://example.com",
    "builder": {
      "type": "command",
      "command": ["echo", "{}"],
      "params": {"type": "MARKET"}
    }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"run", "--config", config, "--confirm-live"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected fill-likely lifecycle validation error")
	}
	if !strings.Contains(err.Error(), "cleanup/neutralization adapter") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunRejectsUnsupportedVenueTransport(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	if err := os.WriteFile(config, []byte(`{
  "venue": "edgex",
  "benchmark": {"iterations": 1},
  "request": {
    "transport": "websocket",
    "builder": {
      "type": "command",
      "command": ["echo", "{}"],
      "params": {
        "contract_id": "1",
        "price": "1000",
        "size": "0.1",
        "metadata": {"contractList": [], "coinList": []}
      }
    }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"run", "--config", config, "--confirm-live"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unsupported transport error")
	}
	if !strings.Contains(err.Error(), "does not support websocket single") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunRejectsMissingVenueBuilderParam(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	if err := os.WriteFile(config, []byte(`{
  "venue": "edgex",
  "benchmark": {"iterations": 1},
  "request": {
    "builder": {
      "type": "command",
      "command": ["echo", "{}"],
      "params": {"price": "75000", "size": "0.001"}
    }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"run", "--config", config, "--confirm-live"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing builder param error")
	}
	if !strings.Contains(err.Error(), "params.metadata") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRunConfigUsesVenueBuilderDefaults(t *testing.T) {
	cfg := fileConfig{
		Venue:     "lighter",
		Benchmark: benchmarkConfig{Iterations: 1},
		Request: requestConfig{
			Builder: builderConfig{
				Type:    "command",
				Command: []string{"echo", "{}"},
			},
		},
	}

	if err := validateRunConfig("lighter", cfg); err != nil {
		t.Fatalf("validateRunConfig = %v", err)
	}
}

func TestInjectRunIDAddsBuilderParam(t *testing.T) {
	cfg := fileConfig{
		Request: requestConfig{
			Builder: builderConfig{
				Params: map[string]any{"price": "75000", "run_id": "old"},
			},
		},
	}

	injectRunID(&cfg, "hyperliquid", "run-test")

	if got := cfg.Request.Builder.Params["run_id"]; got != "run-test" {
		t.Fatalf("run_id = %v", got)
	}
}

func TestCloneFileConfigSeparatesBuilderParams(t *testing.T) {
	cfg := fileConfig{
		Request: requestConfig{
			Builder: builderConfig{
				Params: map[string]any{"run_id": "base"},
			},
		},
	}
	cloned := cloneFileConfig(cfg)

	injectRunID(&cloned, "hyperliquid", "child")

	if got := cloned.Request.Builder.Params["run_id"]; got != "child" {
		t.Fatalf("cloned run_id = %v", got)
	}
	if got := cfg.Request.Builder.Params["run_id"]; got != "base" {
		t.Fatalf("base run_id = %v", got)
	}
}

func TestValidateCleanupRejectsUnsupportedVenue(t *testing.T) {
	cfg := fileConfig{Cleanup: cleanupConfig{Enabled: true, Mode: "best_effort", Scope: "after_sample"}}

	err := validateCleanupForRun("edgex", cfg)
	if err == nil {
		t.Fatal("expected unsupported cleanup error")
	}
	if !strings.Contains(err.Error(), "cleanup adapter") {
		t.Fatalf("error = %v", err)
	}
}

func TestEnvFilePrecedence(t *testing.T) {
	dir := t.TempDir()
	shared := filepath.Join(dir, ".env.local")
	venue := filepath.Join(dir, ".env.hyperliquid.local")
	if err := os.WriteFile(shared, []byte("BENCH_LAYERED_ENV=shared\nBENCH_SHELL_ENV=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(venue, []byte("BENCH_LAYERED_ENV=venue\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BENCH_SHELL_ENV", "from-shell")
	oldLayered, hadLayered := os.LookupEnv("BENCH_LAYERED_ENV")
	if err := os.Unsetenv("BENCH_LAYERED_ENV"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if hadLayered {
			_ = os.Setenv("BENCH_LAYERED_ENV", oldLayered)
		} else {
			_ = os.Unsetenv("BENCH_LAYERED_ENV")
		}
	}()

	if err := prepareRuntimeEnvironment(fileConfig{EnvFiles: []string{shared, venue}}, &runOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("BENCH_LAYERED_ENV"); got != "venue" {
		t.Fatalf("BENCH_LAYERED_ENV = %q", got)
	}
	if got := os.Getenv("BENCH_SHELL_ENV"); got != "from-shell" {
		t.Fatalf("BENCH_SHELL_ENV = %q", got)
	}
}

func TestLoadConfigNormalizesLegacyVenueBlocks(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	if err := os.WriteFile(config, []byte(`{
  "venue": "lighter",
  "venues": {
    "lighter": {
      "request": {
        "url": "https://generic.example/sendTx"
      }
    }
  },
  "lighter": {
    "base_url": "https://legacy.example",
    "send_tx_url": "https://legacy.example/sendTx",
    "send_tx_batch_url": "https://legacy.example/sendTxBatch",
    "request": {
      "batch_body": "legacy-batch"
    }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadFileConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	venueCfg := cfg.Venues["lighter"]
	if venueCfg.BaseURL != "https://legacy.example" {
		t.Fatalf("base url = %q", venueCfg.BaseURL)
	}
	if venueCfg.Request.URL != "https://generic.example/sendTx" {
		t.Fatalf("url = %q", venueCfg.Request.URL)
	}
	if venueCfg.Request.BatchURL != "https://legacy.example/sendTxBatch" {
		t.Fatalf("batch url = %q", venueCfg.Request.BatchURL)
	}
	if venueCfg.Request.BatchBody != "legacy-batch" {
		t.Fatalf("batch body = %q", venueCfg.Request.BatchBody)
	}
	if cfg.Lighter.BaseURL != "" {
		t.Fatalf("legacy lighter config was not cleared: %#v", cfg.Lighter)
	}
}

func TestAccountsGenerateAndPrintCommands(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, ".env.wallets.local")

	generate := NewRootCommand()
	generate.SetArgs([]string{"accounts", "generate", "--venues", "hyperliquid,grvt,edgex,extended,lighter", "--out", out})
	if err := generate.Execute(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"HYPERLIQUID_SECRET_KEY=",
		"GRVT_PRIVATE_KEY=",
		"EDGEX_STARK_PRIVATE_KEY=",
		"EXTENDED_PRIVATE_KEY=",
		"EXTENDED_PUBLIC_KEY=",
		"LIGHTER_PRIVATE_KEY=",
		"LIGHTER_ACCOUNT_INDEX=",
		"LIGHTER_API_KEY_INDEX=",
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("generated env missing %s:\n%s", want, string(data))
		}
	}
	if strings.Contains(string(data), "LIGHTER_PRIVATE_KEY=0x") {
		t.Fatalf("generated env should not generate Lighter API key material:\n%s", string(data))
	}

	printCmd := NewRootCommand()
	printCmd.SetArgs([]string{"accounts", "print", "--venues", "hyperliquid,extended,lighter", "--env-file", out})
	if err := printCmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountsChecklistCommand(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, ".env.wallets.local")

	generate := NewRootCommand()
	generate.SetArgs([]string{"accounts", "generate", "--venues", "hyperliquid,lighter", "--out", out})
	if err := generate.Execute(); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	checklist := NewRootCommand()
	checklist.SetOut(&stdout)
	checklist.SetArgs([]string{"accounts", "checklist", "--venues", "hyperliquid,lighter", "--env-file", out})
	if err := checklist.Execute(); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	for _, want := range []string{
		"benchmark setup checklist",
		"[hyperliquid]",
		"trading/agent wallet: 0x",
		"examples/hyperliquid-builder.json",
		"[lighter]",
		"ethereum setup/funding wallet: 0x",
		"LIGHTER_PRIVATE_KEY",
		"LIGHTER_ACCOUNT_INDEX",
		"examples/lighter-builder.json",
		"accounts check --venues hyperliquid,lighter --env-file .env.wallets.local",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("checklist missing %q:\n%s", want, got)
		}
	}
}

func TestRunBuilderVenueChecksAccountsBeforeBenchmark(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	if err := os.WriteFile(config, []byte(`{
  "venue": "hyperliquid",
  "benchmark": {"iterations": 1},
  "request": {
    "builder": {
      "type": "command",
      "command": ["echo", "{}"],
      "params": {"asset": 0, "size": "0.001", "price": "75000"}
    }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	old, had := os.LookupEnv("HYPERLIQUID_SECRET_KEY")
	_ = os.Unsetenv("HYPERLIQUID_SECRET_KEY")
	defer func() {
		if had {
			_ = os.Setenv("HYPERLIQUID_SECRET_KEY", old)
		}
	}()

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"run", "--config", config, "--confirm-live"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected account setup failure")
	}
	if !strings.Contains(err.Error(), "HYPERLIQUID_SECRET_KEY") {
		t.Fatalf("error = %v", err)
	}
}
