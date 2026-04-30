package lifecycle

import "testing"

func TestValidateRisk(t *testing.T) {
	postOnly := true
	if err := ValidateRisk(RiskConfig{RequirePostOnly: true}, OrderProfile{PostOnly: &postOnly}); err != nil {
		t.Fatal(err)
	}

	notPostOnly := false
	if err := ValidateRisk(RiskConfig{RequirePostOnly: true}, OrderProfile{PostOnly: &notPostOnly}); err == nil {
		t.Fatal("expected post-only validation error")
	}

	if err := ValidateRisk(RiskConfig{AllowFill: true}, OrderProfile{}); err == nil {
		t.Fatal("expected neutralize_on_fill validation error")
	}

	if err := ValidateRisk(RiskConfig{}, OrderProfile{OrderType: "market"}); err == nil {
		t.Fatal("expected fill-likely validation error")
	}
}

func TestProfileFromParamsAndFillLikely(t *testing.T) {
	profile := ProfileFromParams(map[string]any{
		"type":          "MARKET",
		"time_in_force": "IOC",
		"post_only":     "false",
		"side":          "buy",
	})
	if profile.OrderType != "MARKET" || profile.TimeInForce != "IOC" || profile.Side != "buy" {
		t.Fatalf("profile = %#v", profile)
	}
	if profile.PostOnly == nil || *profile.PostOnly {
		t.Fatalf("post_only = %#v", profile.PostOnly)
	}
	if !FillLikely(profile) {
		t.Fatalf("expected fill-likely profile")
	}
}
