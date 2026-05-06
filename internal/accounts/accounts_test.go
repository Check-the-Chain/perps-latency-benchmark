package accounts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"perps-latency-benchmark/internal/venues/registry"
)

func TestGenerateDeduplicatesWalletKinds(t *testing.T) {
	specs, err := ResolveVenues("hyperliquid,grvt,edgex,extended,lighter")
	if err != nil {
		t.Fatal(err)
	}

	values, wallets, err := Generate(specs, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(wallets) != 2 {
		t.Fatalf("wallets = %#v", wallets)
	}
	if values["HYPERLIQUID_SECRET_KEY"] == "" || values["GRVT_PRIVATE_KEY"] == "" {
		t.Fatalf("missing evm generated values: %#v", values)
	}
	if values["HYPERLIQUID_SECRET_KEY"] != values["GRVT_PRIVATE_KEY"] {
		t.Fatalf("expected EVM key reuse")
	}
	if values["EDGEX_STARK_PRIVATE_KEY"] == "" || values["EXTENDED_PRIVATE_KEY"] == "" {
		t.Fatalf("missing stark generated values: %#v", values)
	}
	if values["EDGEX_STARK_PRIVATE_KEY"] != values["EXTENDED_PRIVATE_KEY"] {
		t.Fatalf("expected Stark key reuse")
	}
	if values["EXTENDED_PUBLIC_KEY"] == "" {
		t.Fatalf("missing extended public key")
	}
	if values["LIGHTER_L1_PRIVATE_KEY"] == "" || values["LIGHTER_L1_ADDRESS"] == "" {
		t.Fatalf("missing lighter l1 wallet")
	}
	if values["LIGHTER_L1_PRIVATE_KEY"] != values["HYPERLIQUID_SECRET_KEY"] {
		t.Fatalf("expected Lighter L1 wallet to reuse EVM key")
	}
	if value, ok := values["LIGHTER_PRIVATE_KEY"]; !ok || value != "" {
		t.Fatalf("expected blank Lighter API key placeholder, got %q exists=%v", value, ok)
	}
}

func TestGeneratePreservesExistingValues(t *testing.T) {
	specs, err := ResolveVenues("hyperliquid,grvt")
	if err != nil {
		t.Fatal(err)
	}

	values, _, err := Generate(specs, map[string]string{"HYPERLIQUID_SECRET_KEY": "existing"})
	if err != nil {
		t.Fatal(err)
	}
	if values["HYPERLIQUID_SECRET_KEY"] != "existing" {
		t.Fatalf("overwrote existing value")
	}
	if values["GRVT_PRIVATE_KEY"] == "" {
		t.Fatalf("missing grvt key")
	}
}

func TestCheckReportsMissingRequiredEnv(t *testing.T) {
	specs, err := ResolveVenues("lighter")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LIGHTER_PRIVATE_KEY", "")
	t.Setenv("LIGHTER_ACCOUNT_INDEX", "")
	t.Setenv("LIGHTER_API_KEY_INDEX", "")

	err = Check(specs)
	if err == nil {
		t.Fatal("expected missing env error")
	}
	if !strings.Contains(err.Error(), "LIGHTER_PRIVATE_KEY") {
		t.Fatalf("error = %v", err)
	}
}

func TestDotenvRoundTripPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env.wallets.local")
	if err := WriteDotenv(path, map[string]string{"B": "2", "A": "1"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %s", info.Mode().Perm())
	}
	values, err := LoadDotenv(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["A"] != "1" || values["B"] != "2" {
		t.Fatalf("values = %#v", values)
	}
}

func TestEveryRegisteredVenueHasAccountSpec(t *testing.T) {
	for _, name := range registry.Names() {
		if _, ok := Spec(name); !ok {
			t.Fatalf("missing account spec for registered venue %q", name)
		}
	}
}
