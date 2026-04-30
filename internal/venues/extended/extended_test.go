package extended

import "testing"

func TestDefinition(t *testing.T) {
	definition := Definition()

	if definition.Name != "extended" {
		t.Fatalf("name = %s", definition.Name)
	}
	if definition.DefaultBaseURL != DefaultBaseURL {
		t.Fatalf("base url = %s", definition.DefaultBaseURL)
	}
	if definition.DefaultHTTPPath != "/api/v1/user/order" {
		t.Fatalf("http path = %s", definition.DefaultHTTPPath)
	}
	if definition.DefaultWSURL != "" {
		t.Fatalf("expected no websocket order submission default, got %s", definition.DefaultWSURL)
	}
	if len(definition.Docs) == 0 {
		t.Fatal("expected docs")
	}
}
