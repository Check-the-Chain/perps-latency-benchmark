package secrets

import (
	"strings"
	"testing"
)

func TestFindInlineSecrets(t *testing.T) {
	cfg := map[string]any{
		"request": map[string]any{
			"builder": map[string]any{
				"params": map[string]any{
					"private_key": "abc",
					"market":      "BTC",
				},
			},
		},
	}

	findings, err := FindInlineSecrets(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %#v", findings)
	}
	if findings[0].Path != "$.request.builder.params.private_key" {
		t.Fatalf("path = %s", findings[0].Path)
	}
}

func TestLighterAPIKeyMetadataIsNotSecret(t *testing.T) {
	cfg := map[string]any{
		"params": map[string]any{
			"api_key_role":        "maker",
			"maker_api_key_index": 4,
			"taker_api_key_index": 5,
		},
	}

	findings, err := FindInlineSecrets(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestRedactString(t *testing.T) {
	input := `{"private_key":"abc","ok":true} HYPERLIQUID_SECRET_KEY=def token: ghi`
	got := RedactString(input)

	for _, leaked := range []string{"abc", "def", "ghi"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("redacted string leaked %q: %s", leaked, got)
		}
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("redacted string = %s", got)
	}
}

func TestRedactValue(t *testing.T) {
	value := map[string]any{
		"api_key": "abc",
		"nested": map[string]any{
			"market": "BTC",
		},
	}

	redacted := RedactValue(value).(map[string]any)
	if redacted["api_key"] != "[REDACTED]" {
		t.Fatalf("api_key = %#v", redacted["api_key"])
	}
	if redacted["nested"].(map[string]any)["market"] != "BTC" {
		t.Fatalf("nested = %#v", redacted["nested"])
	}
}
