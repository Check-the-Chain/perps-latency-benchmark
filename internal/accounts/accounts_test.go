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

func TestGenerateEd25519WalletProducesSeedAndPublicKey(t *testing.T) {
	wallet, err := GenerateWallet(WalletEd25519)
	if err != nil {
		t.Fatalf("generate ed25519 wallet: %v", err)
	}
	if len(wallet.PrivateKey) != 64 {
		t.Fatalf("private key hex length = %d, want 64 (32-byte seed)", len(wallet.PrivateKey))
	}
	// The public key is base58, the encoding Bullet's API accepts; a 32-byte key
	// encodes to 43 or 44 characters.
	if len(wallet.PublicKey) < 43 || len(wallet.PublicKey) > 44 {
		t.Fatalf("public key length = %d, want 43-44 (base58 of a 32-byte key)", len(wallet.PublicKey))
	}
	if strings.ContainsAny(wallet.PublicKey, "0OIl+/") {
		t.Fatalf("public key %q contains characters outside the base58 alphabet", wallet.PublicKey)
	}
	if wallet.Kind != WalletEd25519 {
		t.Fatalf("kind = %q, want %q", wallet.Kind, WalletEd25519)
	}
}

func TestBulletSpecGeneratesDelegateKey(t *testing.T) {
	var spec VenueSpec
	for _, candidate := range Specs() {
		if candidate.Name == "bullet" {
			spec = candidate
			break
		}
	}
	if spec.Name == "" {
		t.Fatal("bullet venue spec must exist")
	}
	values, wallets, err := Generate([]VenueSpec{spec}, map[string]string{})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if values["BULLET_DELEGATE_PRIVATE_KEY"] == "" {
		t.Fatal("delegate private key must be generated")
	}
	if _, ok := wallets[WalletEd25519]; !ok {
		t.Fatal("an ed25519 wallet must be generated for bullet")
	}
	if _, present := values["BULLET_ACCOUNT_ADDRESS"]; !present {
		t.Fatal("account address must be present as a blank required value")
	}
}

func TestPublicFromEnvDerivesBulletDelegatePublicKey(t *testing.T) {
	seed := "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	wallet, err := PublicFromEnv(WalletEd25519, map[string]string{"BULLET_DELEGATE_PRIVATE_KEY": seed})
	if err != nil {
		t.Fatalf("derive public key: %v", err)
	}
	if wallet.PublicKey != "FAe4sisG95oZ42w7buUn5qEE4TAnfTTFPiguZUHmhiF" {
		t.Fatalf("public key = %q, want the known base58 vector for this seed", wallet.PublicKey)
	}
	if wallet.PrivateKey != "" {
		t.Fatal("PublicFromEnv must not leak the private key")
	}
}

func TestEncodeBase58MatchesKnownVectors(t *testing.T) {
	cases := []struct {
		in   []byte
		want string
	}{
		{[]byte{0}, "1"},
		{[]byte{0, 0, 1}, "112"},
		{[]byte("hello world"), "StV1DL6CwTryKyV"},
	}
	for _, testCase := range cases {
		if got := encodeBase58(testCase.in); got != testCase.want {
			t.Fatalf("encodeBase58(%v) = %q, want %q", testCase.in, got, testCase.want)
		}
	}
}
