package app

import (
	"bytes"
	"strings"
	"testing"

	"perps-latency-benchmark/internal/accounts"
)

func TestPrintLoadedWalletIdentifiersShowsBulletDelegatePublicKeyNotSeed(t *testing.T) {
	seed := "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	wantPublicKey := "03a107bff3ce10be1d70dd18e74bc09967e4d6309ba50d5f1ddc8664125531b8"

	specs, err := accounts.ResolveVenues("bullet")
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"BULLET_DELEGATE_PRIVATE_KEY": seed}

	var buf bytes.Buffer
	printLoadedWalletIdentifiers(&buf, specs, env)
	out := buf.String()

	if !strings.Contains(out, wantPublicKey) {
		t.Fatalf("output missing delegate public key, got: %s", out)
	}
	if !strings.Contains(strings.ToLower(out), "delegate") {
		t.Fatalf("output does not label the value as a delegate key, got: %s", out)
	}
	if strings.Contains(out, seed) {
		t.Fatalf("output leaked the private key seed: %s", out)
	}
}

func TestPrintWalletSummaryShowsBulletDelegatePublicKeyNotSeed(t *testing.T) {
	wallet, err := accounts.GenerateWallet(accounts.WalletEd25519)
	if err != nil {
		t.Fatal(err)
	}
	wallets := map[accounts.WalletKind]accounts.Wallet{accounts.WalletEd25519: wallet}

	var buf bytes.Buffer
	printWalletSummary(&buf, wallets)
	out := buf.String()

	if !strings.Contains(out, wallet.PublicKey) {
		t.Fatalf("output missing delegate public key, got: %s", out)
	}
	if !strings.Contains(strings.ToLower(out), "delegate") {
		t.Fatalf("output does not label the value as a delegate key, got: %s", out)
	}
	if strings.Contains(out, wallet.PrivateKey) {
		t.Fatalf("output leaked the private key seed: %s", out)
	}
}
