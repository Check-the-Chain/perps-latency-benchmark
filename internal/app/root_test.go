package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"perps-latency-benchmark/internal/lifecycle"
)

func TestWebSocketCapableLiveExamplesPreferWebSocketSubmission(t *testing.T) {
	for _, path := range []string{
		"examples/hyperliquid-builder.json",
		"examples/hyperliquid-batch5-builder.json",
		"examples/hyperliquid-taker-builder.json",
		"examples/lighter-builder.json",
		"examples/lighter-batch5-builder.json",
		"examples/lighter-market-builder.json",
		"examples/lighter-free-market-builder.json",
	} {
		data, err := os.ReadFile(filepath.Join("..", "..", path))
		if err != nil {
			t.Fatal(err)
		}
		var cfg struct {
			Request struct {
				Transport string `json:"transport"`
			} `json:"request"`
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if cfg.Request.Transport != "websocket" {
			t.Fatalf("%s transport = %q, want websocket", path, cfg.Request.Transport)
		}
	}
}

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
	err := runRoot("run", "--venue", "http", "--url", "https://example.com", "--body", "{}")
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
	resultPath := writeTestFile(t, filepath.Join(dir, "result.json"), `{
  "venue": "mock",
  "scenario": "single",
  "latency_mode": "total",
  "reconciliation": {"attempted": true, "ok": true},
  "samples": [
    {"venue": "mock", "scenario": "single", "transport": "https", "network_ns": 1000000, "ok": true, "cleanup": {"attempted": true, "ok": true}},
    {"venue": "mock", "scenario": "single", "transport": "https", "network_ns": 2000000, "ok": true, "cleanup": {"attempted": true, "ok": true}}
  ]
}`, 0o644)

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
	script := writeTestFile(t, filepath.Join(dir, "builder.sh"), `#!/bin/sh
printf '{"headers":{"X-Builder":"test"},"body":"dynamic-body"}\n'
`, 0o755)
	config := writeConfig(t, dir, `{
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
}`)

	if err := runRoot("run", "--config", config, "--confirm-live"); err != nil {
		t.Fatal(err)
	}
	if got := <-received; got != "dynamic-body" {
		t.Fatalf("body = %q", got)
	}
}

func TestRunLoadsEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFile := writeTestFile(t, filepath.Join(dir, ".env.hyperliquid.local"), "BENCH_TEST_ENV=from-file\n", 0o600)
	unsetEnv(t, "BENCH_TEST_ENV")

	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	script := writeTestFile(t, filepath.Join(dir, "builder.sh"), `#!/bin/sh
printf '{"body":"%s"}\n' "$BENCH_TEST_ENV"
`, 0o755)
	config := writeConfig(t, dir, `{
  "venue": "http",
  "benchmark": {"iterations": 1},
  "request": {
    "url": "`+server.URL+`",
    "builder": {
      "type": "command",
      "command": ["`+script+`"]
    }
  }
}`)

	if err := runRoot("run", "--config", config, "--env-file", envFile, "--confirm-live"); err != nil {
		t.Fatal(err)
	}
	if got := <-received; got != "from-file" {
		t.Fatalf("body = %q", got)
	}
}

func TestRunRejectsInlineSecrets(t *testing.T) {
	dir := t.TempDir()
	config := writeConfig(t, dir, `{
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
}`)

	err := runRoot("run", "--config", config, "--confirm-live")
	if err == nil {
		t.Fatal("expected inline secret rejection")
	}
	if !strings.Contains(err.Error(), "inline secrets") || !strings.Contains(err.Error(), "private_key") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunRejectsFillLikelyProfileWithoutLifecycleAdapter(t *testing.T) {
	dir := t.TempDir()
	config := writeConfig(t, dir, `{
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
}`)

	err := runRoot("run", "--config", config, "--confirm-live")
	if err == nil {
		t.Fatal("expected fill-likely lifecycle validation error")
	}
	if !strings.Contains(err.Error(), "cleanup/neutralization adapter") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunAllowsFillLikelyProfilesWithStrictNeutralization(t *testing.T) {
	tests := []struct {
		venue  string
		params map[string]any
	}{
		{"lighter", map[string]any{"market_index": 1, "base_amount": 100, "price": 750000, "order_type": 1, "time_in_force": 0}},
		{"extended", map[string]any{"market": "BTC-USD", "size": "0.001", "price": "75000", "order_type": "market"}},
		{"edgex", map[string]any{"contract_id": "10000001", "size": "0.001", "price": "75000", "type": "MARKET", "metadata": map[string]any{"contractList": []any{}, "coinList": []any{}}}},
		{"aster", map[string]any{"symbol": "BTCUSDT", "quantity": "0.001", "type": "MARKET"}},
	}

	for _, tt := range tests {
		t.Run(tt.venue, func(t *testing.T) {
			cfg := fillLikelyLifecycleConfig(tt.venue, tt.params)
			if err := validateLifecycleForRun(tt.venue, cfg); err != nil {
				t.Fatalf("validateLifecycleForRun = %v", err)
			}
		})
	}

	cfg := fillLikelyLifecycleConfig("lighter", tests[0].params)
	cfg.Cleanup.Mode = "best_effort"
	if err := validateLifecycleForRun("lighter", cfg); err == nil {
		t.Fatal("expected strict cleanup requirement")
	}
}

func TestRunRejectsUnsupportedVenueTransport(t *testing.T) {
	dir := t.TempDir()
	config := writeConfig(t, dir, `{
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
}`)

	err := runRoot("run", "--config", config, "--confirm-live")
	if err == nil {
		t.Fatal("expected unsupported transport error")
	}
	if !strings.Contains(err.Error(), "does not support websocket single") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunRejectsMissingVenueBuilderParam(t *testing.T) {
	dir := t.TempDir()
	config := writeConfig(t, dir, `{
  "venue": "edgex",
  "benchmark": {"iterations": 1},
  "request": {
    "builder": {
      "type": "command",
      "command": ["echo", "{}"],
      "params": {"price": "75000", "size": "0.001"}
    }
  }
}`)

	err := runRoot("run", "--config", config, "--confirm-live")
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

	err := validateCleanupForRun("grvt", cfg)
	if err == nil {
		t.Fatal("expected unsupported cleanup error")
	}
	if !strings.Contains(err.Error(), "cleanup adapter") {
		t.Fatalf("error = %v", err)
	}
}

func TestEnvFilePrecedence(t *testing.T) {
	dir := t.TempDir()
	shared := writeTestFile(t, filepath.Join(dir, ".env.local"), "BENCH_LAYERED_ENV=shared\nBENCH_SHELL_ENV=from-file\n", 0o600)
	venue := writeTestFile(t, filepath.Join(dir, ".env.hyperliquid.local"), "BENCH_LAYERED_ENV=venue\n", 0o600)

	t.Setenv("BENCH_SHELL_ENV", "from-shell")
	unsetEnv(t, "BENCH_LAYERED_ENV")

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
	config := writeConfig(t, dir, `{
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
}`)

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
	config := writeConfig(t, dir, `{
  "venue": "hyperliquid",
  "benchmark": {"iterations": 1},
  "request": {
    "builder": {
      "type": "command",
      "command": ["echo", "{}"],
      "params": {"asset": 0, "size": "0.001", "price": "75000"}
    }
  }
}`)
	unsetEnv(t, "HYPERLIQUID_SECRET_KEY")

	err := runRoot("run", "--config", config, "--confirm-live")
	if err == nil {
		t.Fatal("expected account setup failure")
	}
	if !strings.Contains(err.Error(), "HYPERLIQUID_SECRET_KEY") {
		t.Fatalf("error = %v", err)
	}
}

func runRoot(args ...string) error {
	cmd := NewRootCommand()
	cmd.SetArgs(args)
	return cmd.Execute()
}

func writeConfig(t *testing.T, dir string, content string) string {
	t.Helper()
	return writeTestFile(t, filepath.Join(dir, "config.json"), content, 0o644)
}

func writeTestFile(t *testing.T, path string, content string, perm os.FileMode) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
	return path
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	old, had := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func fillLikelyLifecycleConfig(venue string, params map[string]any) fileConfig {
	return fileConfig{
		Venue:   venue,
		Risk:    lifecycle.RiskConfig{AllowFill: true, NeutralizeOnFill: true},
		Cleanup: cleanupConfig{Enabled: true, Mode: "strict", Scope: "after_sample"},
		Request: requestConfig{
			Builder: builderConfig{
				Type:    "command",
				Command: []string{"echo", "{}"},
				Params:  params,
			},
		},
	}
}
