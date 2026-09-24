package funding

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestLighterIntentAddressUsesConfiguredSourceWallet(t *testing.T) {
	t.Setenv("LIGHTER_TEST_L1_ADDRESS", "0x2222222222222222222222222222222222222222")
	const intentAddress = "0x1111111111111111111111111111111111111111"
	var posted url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		posted = r.PostForm
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"intent_address": intentAddress},
		})
	}))
	defer server.Close()

	depositor := EVMDepositor{HTTPClient: server.Client()}
	address, metadata, err := depositor.lighterIntentAddress(context.Background(), DepositPlan{
		Wallet: WalletConfig{
			ChainID:       DefaultArbitrumChainID,
			PrivateKeyEnv: "BENCHMARK_ARBITRUM_PRIVATE_KEY",
		},
		Account: AccountConfig{
			Deposit: DepositConfig{
				Type:           "lighter_cctp_intent",
				BaseURL:        server.URL,
				FromAddressEnv: "LIGHTER_TEST_L1_ADDRESS",
			},
		},
		Now: time.Unix(100, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if address != intentAddress {
		t.Fatalf("address = %s, want %s", address, intentAddress)
	}
	if posted.Get("chain_id") != "42161" || posted.Get("amount") != "0" || posted.Get("is_external_deposit") != "true" {
		t.Fatalf("posted form = %v", posted)
	}
	if got := metadata["lighter_from_address"]; got != "0x2222222222222222222222222222222222222222" {
		t.Fatalf("from address metadata = %v", got)
	}
}

func TestParseCommitmentIDUsesHexQuoteID(t *testing.T) {
	got, err := parseCommitmentID("6a19a150a424dd2cc1c671a8")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := new(big.Int).SetString("6a19a150a424dd2cc1c671a8", 16)
	if got.Cmp(want) != 0 {
		t.Fatalf("commitment id = %s, want %s", got, want)
	}
}

func TestExtendedBridgeQuoteParsesFee(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/user/bridge/quote" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("X-Api-Key") != "test-key" {
			t.Fatalf("missing api key header")
		}
		if r.URL.Query().Get("chainIn") != "ARB" || r.URL.Query().Get("chainOut") != "STRK" || r.URL.Query().Get("asset") != "USD" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "OK",
			"data": map[string]any{
				"id":  "6a19a150a424dd2cc1c671a8",
				"fee": "0.01",
			},
		})
	}))
	defer server.Close()

	quote, err := (EVMDepositor{HTTPClient: server.Client()}).extendedBridgeQuote(context.Background(), server.URL, "test-key", "ARB", "STRK", "USD", 5)
	if err != nil {
		t.Fatal(err)
	}
	if quote.ID != "6a19a150a424dd2cc1c671a8" || quote.Fee != 0.01 {
		t.Fatalf("quote = %+v", quote)
	}
}

func TestCommandDepositUsesContext(t *testing.T) {
	result, err := EVMDepositor{}.commandDeposit(context.Background(), DepositPlan{
		Account: AccountConfig{
			Name: "custom",
			Deposit: DepositConfig{
				Type:    "command",
				Command: []string{"sh", "-c", "cat >/dev/null; printf '%s' '{\"status\":\"success\",\"tx_hash\":\"0xabc\"}'"},
			},
		},
		Amount: 12,
		Now:    time.Unix(100, 0),
	}, DepositResult{AccountName: "custom", AmountUSDC: 12, Route: "command"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" || result.TxHash != "0xabc" {
		t.Fatalf("result = %+v", result)
	}
}
