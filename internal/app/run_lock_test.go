package app

import (
	"strings"
	"testing"
)

func TestLighterRunLockRejectsSharedAPIKey(t *testing.T) {
	t.Setenv("PERPS_BENCH_LOCK_DIR", t.TempDir())
	t.Setenv("LIGHTER_ACCOUNT_INDEX", "1")
	t.Setenv("LIGHTER_API_KEY_INDEX", "4")

	cfg := fileConfig{
		Venue: "lighter",
		Request: requestConfig{
			Builder: builderConfig{
				Type:    "command",
				Command: []string{"echo", "{}"},
			},
		},
	}
	lock, err := acquireRunLock("lighter", cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()

	if _, err := acquireRunLock("lighter", cfg); err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("expected lock contention error, got %v", err)
	}
}

func TestLighterRunLockAllowsDistinctAPIKeys(t *testing.T) {
	t.Setenv("PERPS_BENCH_LOCK_DIR", t.TempDir())
	t.Setenv("LIGHTER_ACCOUNT_INDEX", "1")
	t.Setenv("LIGHTER_API_KEY_INDEX", "4")

	cfg := fileConfig{
		Venue: "lighter",
		Request: requestConfig{
			Builder: builderConfig{
				Type:    "command",
				Command: []string{"echo", "{}"},
			},
		},
	}
	lock, err := acquireRunLock("lighter", cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()

	cfg.Request.Builder.Params = map[string]any{"api_key_index": 5}
	other, err := acquireRunLock("lighter", cfg)
	if err != nil {
		t.Fatal(err)
	}
	other.Release()
}
