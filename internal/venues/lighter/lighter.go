package lighter

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"perps-latency-benchmark/internal/booktop"
	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/venues/spec"
)

const DefaultBaseURL = "https://mainnet.zklighter.elliot.ai"
const DefaultHTTPPath = "/api/v1/sendTx"
const DefaultBatchPath = "/api/v1/sendTxBatch"
const DefaultWSURL = "wss://mainnet.zklighter.elliot.ai/stream"

const WebSocketSendTxType = "jsonapi/sendtx"
const WebSocketSendTxBatchType = "jsonapi/sendtxbatch"

func Definition() spec.Definition {
	return spec.Definition{
		Name:             "lighter",
		Aliases:          []string{"zkLighter", "zk-lighter", "zklighter"},
		DefaultBaseURL:   DefaultBaseURL,
		DefaultHTTPPath:  DefaultHTTPPath,
		DefaultBatchPath: DefaultBatchPath,
		DefaultWSURL:     DefaultWSURL,
		WSReadInitial:    true,
		Capabilities: spec.Capabilities{
			HTTPSingle:      true,
			HTTPBatch:       true,
			WebSocketSingle: true,
			WebSocketBatch:  true,
			Cleanup:         true,
			Neutralization:  true,
		},
		BuilderParams: spec.BuilderParams{
			Required: []string{"market_index", "base_amount", "price"},
			Defaults: map[string]any{
				"symbol":       "BTC",
				"market_index": 1,
				"base_amount":  100,
				"price":        750000,
				"side":         "buy",
				"post_only":    true,
			},
		},
		CleanupCommand: spec.CleanupCommand{
			Type: "persistent_command",
			Command: []string{
				"uv",
				"run",
				"--with",
				"lighter-sdk",
				"python",
				filepath.FromSlash("internal/venues/lighter/cancel_payload.py"),
			},
			Description:    "cancel lighter benchmark orders by order index",
			OrderRefsField: "cleanup_orders",
			SkipNoRefs:     true,
		},
		CostCommand: spec.CostCommand{
			Type: "persistent_command",
			Command: []string{
				"uv",
				"run",
				"--with",
				"lighter-sdk",
				"python",
				filepath.FromSlash("internal/venues/lighter/cost_payload.py"),
			},
			Timeout: 60 * time.Second,
		},
		BookTop: spec.BookTop{
			Build: func(runtime spec.RuntimeConfig) (booktop.Config, bool) {
				url := spec.CoalesceURL(runtime.WSURL, DefaultWSURL)
				symbol := spec.TextParam(runtime.Params, "market_index", "1")
				if url == "" || symbol == "" {
					return booktop.Config{}, false
				}
				return booktop.Config{
					URL:    url,
					Symbol: symbol,
					Parser: booktop.NewLighterParser(),
				}, true
			},
		},
		ExpectedFill: spec.ExpectedFill{
			Build: func(runtime spec.RuntimeConfig) (spec.ExpectedFillOrder, bool) {
				return spec.ExpectedFillOrder{
					Side: spec.TextParam(runtime.Params, "side", "buy"),
					Size: spec.FloatParam(runtime.Params, "base_amount") / math.Pow10(5),
				}, true
			},
		},
		RunLock: spec.RunLock{
			Build: func(runtime spec.RuntimeConfig) (spec.RunLockTarget, bool) {
				accountIndex := runtimeParam(runtime.Params, "account_index", "LIGHTER_ACCOUNT_INDEX")
				apiKeyIndex := lighterRuntimeAPIKeyIndex(runtime.Params)
				if accountIndex == "" || apiKeyIndex == "" {
					return spec.RunLockTarget{}, false
				}
				return spec.RunLockTarget{
					Name:        fmt.Sprintf("lighter-account-%s-api-key-%s", accountIndex, apiKeyIndex),
					BusyMessage: "lighter account/API key is already in use by another benchmark process; use one Lighter API key per concurrent runner or stop the existing process",
				}, true
			},
		},
		Classifier:         classify,
		Confirmation:       ConfirmWebSocket,
		CancelConfirmation: ConfirmCancelWebSocket,
		Docs: []string{
			"https://apidocs.lighter.xyz/docs/get-started",
			"https://apidocs.lighter.xyz/reference/sendtx",
			"https://apidocs.lighter.xyz/reference/sendtxbatch",
			"https://apidocs.lighter.xyz/docs/websocket-reference",
			"https://github.com/elliottech/lighter-python",
		},
		Notes: []string{
			"HTTP transaction submission is POST /api/v1/sendTx with tx_type and signed tx_info form fields.",
			"HTTP batch transaction submission is POST /api/v1/sendTxBatch with signed tx_types and tx_infos form fields.",
			"WebSocket transaction submission uses type=jsonapi/sendtx with data.tx_type and signed data.tx_info.",
			"WebSocket batch transaction submission uses type=jsonapi/sendtxbatch with signed data.tx_types and signed data.tx_infos.",
			"The Lighter stream sends an initial connection message; it is read before timed sends.",
			"Signing and payload generation are expected to happen before benchmarking; this venue reuses prebuilt request bodies.",
		},
	}
}

func runtimeParam(params map[string]any, key string, envKey string) string {
	if value, ok := params[key]; ok && value != nil {
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" {
			return text
		}
	}
	if text := runtimePrefixedEnv(params, key); text != "" {
		return text
	}
	return strings.TrimSpace(os.Getenv(envKey))
}

func lighterRuntimeAPIKeyIndex(params map[string]any) string {
	if apiKeyIndex := paramText(params, "api_key_index"); apiKeyIndex != "" {
		return apiKeyIndex
	}
	role := strings.ToLower(runtimeParam(params, "api_key_role", ""))
	if role == "" || role == "auto" {
		role = "maker"
		if lighterRuntimeTakerOrder(params) {
			role = "taker"
		}
	}
	switch role {
	case "maker":
		return firstRuntimeParam(params, "maker_api_key_index", "LIGHTER_MAKER_API_KEY_INDEX", "LIGHTER_API_KEY_INDEX")
	case "taker":
		return firstRuntimeParam(params, "taker_api_key_index", "LIGHTER_TAKER_API_KEY_INDEX", "LIGHTER_API_KEY_INDEX")
	default:
		return runtimeParam(params, "api_key_index", "LIGHTER_API_KEY_INDEX")
	}
}

func paramText(params map[string]any, key string) string {
	if value, ok := params[key]; ok && value != nil {
		return strings.TrimSpace(fmt.Sprint(value))
	}
	return ""
}

func lighterRuntimeTakerOrder(params map[string]any) bool {
	orderType := strings.TrimSpace(fmt.Sprint(params["order_type"]))
	timeInForce := strings.TrimSpace(fmt.Sprint(params["time_in_force"]))
	return orderType == "1" || timeInForce == "0"
}

func firstRuntimeParam(params map[string]any, key string, envKeys ...string) string {
	if value, ok := params[key]; ok && value != nil {
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" {
			return text
		}
	}
	if text := runtimePrefixedEnv(params, key); text != "" {
		return text
	}
	for _, envKey := range envKeys {
		text := strings.TrimSpace(os.Getenv(envKey))
		if text != "" {
			return text
		}
	}
	return ""
}

func runtimePrefixedEnv(params map[string]any, key string) string {
	prefix := strings.TrimSpace(fmt.Sprint(params["env_prefix"]))
	if prefix == "" || prefix == "<nil>" {
		prefix = strings.TrimSpace(fmt.Sprint(params["account_env_prefix"]))
	}
	if prefix == "" || prefix == "<nil>" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(prefix + "_" + strings.ToUpper(key)))
}

func Classify(in lifecycle.ResponseInput) lifecycle.Classification {
	return classify(in)
}

func classify(in lifecycle.ResponseInput) lifecycle.Classification {
	generic := lifecycle.ClassifyResponse(in)
	if in.Err != nil || len(in.Body) == 0 {
		return generic
	}
	var decoded map[string]any
	if err := json.Unmarshal(in.Body, &decoded); err != nil {
		return generic
	}
	if code, ok := numericCode(decoded["code"]); ok && code >= 400 {
		return lifecycle.Classification{Status: lifecycle.StatusRejected, Reason: reason(decoded)}
	}
	if success, ok := decoded["success"].(bool); ok && !success {
		return lifecycle.Classification{Status: lifecycle.StatusRejected, Reason: reason(decoded)}
	}
	if !generic.OK() {
		return generic
	}
	return generic
}

func numericCode(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case int:
		return typed, true
	default:
		return 0, false
	}
}

func reason(value map[string]any) string {
	for _, key := range []string{"message", "error", "reason"} {
		if raw, ok := value[key]; ok {
			return fmt.Sprint(raw)
		}
	}
	return ""
}
