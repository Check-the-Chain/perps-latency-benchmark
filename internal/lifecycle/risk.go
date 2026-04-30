package lifecycle

import (
	"fmt"
	"strings"
)

type RiskConfig struct {
	RequirePostOnly  bool `json:"require_post_only"`
	AllowFill        bool `json:"allow_fill"`
	NeutralizeOnFill bool `json:"neutralize_on_fill"`
}

type OrderProfile struct {
	OrderType   string `json:"order_type,omitempty"`
	TimeInForce string `json:"time_in_force,omitempty"`
	Side        string `json:"side,omitempty"`
	PostOnly    *bool  `json:"post_only,omitempty"`
	ReduceOnly  *bool  `json:"reduce_only,omitempty"`
}

func ProfileFromParams(params map[string]any) OrderProfile {
	if len(params) == 0 {
		return OrderProfile{}
	}
	return OrderProfile{
		OrderType:   firstString(params, "order_type", "type"),
		TimeInForce: firstString(params, "time_in_force", "tif"),
		Side:        firstString(params, "side"),
		PostOnly:    firstBool(params, "post_only"),
		ReduceOnly:  firstBool(params, "reduce_only"),
	}
}

func ValidateRisk(risk RiskConfig, profile OrderProfile) error {
	if risk.RequirePostOnly && !isTrue(profile.PostOnly) && !isPostOnlyTIF(profile.TimeInForce) {
		return fmt.Errorf("risk.require_post_only is true but order profile is not post-only")
	}
	if FillLikely(profile) && !risk.AllowFill {
		return fmt.Errorf("order profile is fill-likely; set risk.allow_fill only after venue cleanup and inventory reconciliation are available")
	}
	if risk.AllowFill && !risk.NeutralizeOnFill {
		return fmt.Errorf("risk.allow_fill requires risk.neutralize_on_fill so repeated runs can return inventory to flat")
	}
	return nil
}

func FillLikely(profile OrderProfile) bool {
	orderType := strings.ToLower(profile.OrderType)
	tif := strings.ToLower(profile.TimeInForce)
	if strings.Contains(orderType, "market") {
		return true
	}
	switch tif {
	case "ioc", "immediate_or_cancel", "fok", "fill_or_kill":
		return true
	default:
		return isFalse(profile.PostOnly)
	}
}

func firstString(params map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := params[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			return typed
		case fmt.Stringer:
			return typed.String()
		}
	}
	return ""
}

func firstBool(params map[string]any, keys ...string) *bool {
	for _, key := range keys {
		value, ok := params[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return &typed
		case string:
			parsed := strings.EqualFold(typed, "true") || typed == "1" || strings.EqualFold(typed, "yes")
			return &parsed
		}
	}
	return nil
}

func isTrue(value *bool) bool {
	return value != nil && *value
}

func isFalse(value *bool) bool {
	return value != nil && !*value
}

func isPostOnlyTIF(value string) bool {
	normalized := strings.ToLower(value)
	return normalized == "alo" || normalized == "post_only" || normalized == "postonly"
}
