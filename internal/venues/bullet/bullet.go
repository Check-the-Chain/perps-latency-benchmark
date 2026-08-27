package bullet

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"perps-latency-benchmark/internal/booktop"
	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/venues/spec"
)

const DefaultBaseURL = "https://tradingapi.bullet.xyz"
const DefaultWSURL = "wss://tradingapi.bullet.xyz/ws"

// Market-maker ingress: resolves straight to the origin rather than through the
// public Cloudflare proxy. IP-allowlisted.
const DirectBaseURL = "https://tradingapi-mm.bullet.xyz"
const DirectWSURL = "wss://tradingapi-mm.bullet.xyz/ws"
const DefaultHTTPPath = "/tx/submit"
const WebSocketHeartbeatMessage = `{"method":"ping"}`
const DocsURL = "https://tradingapi.bullet.xyz/docs/"
const WebSocketDocsURL = "https://tradingapi.bullet.xyz/docs/ws/"
const SigningDocsURL = "https://tradingapi.bullet.xyz/docs/tx-signing.md"
const OrderFieldsDocsURL = "https://tradingapi.bullet.xyz/docs/order-fields.md"
const DecimalDocsURL = "https://tradingapi.bullet.xyz/docs/decimal-encoding.md"

func Definition() spec.Definition {
	return definitionFor(DefaultBaseURL, DefaultWSURL)
}

// DirectDefinition binds every host-derived field to the market-maker ingress,
// so the book-top and confirmation feeds cannot silently fall back to the public
// host while orders are submitted over the MM one.
func DirectDefinition() spec.Definition {
	return definitionFor(DirectBaseURL, DirectWSURL)
}

// definitionFor builds a Bullet definition bound to one ingress, so every
// host-derived field follows the same endpoint.
func definitionFor(baseURL string, wsURL string) spec.Definition {
	return spec.Definition{
		Name:            "bullet",
		Aliases:         []string{"bullet-xyz", "bullet_xyz", "bulletx"},
		DefaultBaseURL:  baseURL,
		DefaultHTTPPath: DefaultHTTPPath,
		DefaultWSURL:    wsURL,
		WSReadInitial:   true,
		WSHeartbeat: spec.WebSocketHeartbeat{
			Message:   WebSocketHeartbeatMessage,
			IdleAfter: 25 * time.Second,
			Timeout:   5 * time.Second,
		},
		Capabilities: spec.Capabilities{
			HTTPSingle:      true,
			HTTPBatch:       true,
			WebSocketSingle: true,
			WebSocketBatch:  true,
			Cleanup:         true,
		},
		BuilderParams: spec.BuilderParams{
			Required: []string{"symbol", "size", "side", "price"},
			Defaults: map[string]any{
				"symbol":      "BTC-USD",
				"size":        "0.0001",
				"side":        "bid",
				"price":       "50000",
				"order_type":  "post_only",
				"reduce_only": false,
			},
		},
		CleanupCommand: spec.CleanupCommand{
			Type:           "persistent_command",
			Command:        cancelCommand(),
			Description:    "cancel Bullet benchmark orders by client order id",
			OrderRefsField: "cleanup_orders",
			SkipNoRefs:     true,
		},
		BookTop: spec.BookTop{
			Build: func(runtime spec.RuntimeConfig) (booktop.Config, bool) {
				// runtime.WSURL first, then this definition's own default. Using the
				// package const here would point bullet_direct's book-top feed at the
				// public host while it submits over the MM ingress.
				url := spec.CoalesceURL(runtime.WSURL, wsURL)
				symbol := spec.TextParam(runtime.Params, "symbol", "BTC-USD")
				if url == "" || symbol == "" {
					return booktop.Config{}, false
				}
				return booktop.Config{
					URL:    url,
					Symbol: symbol,
					Parser: booktop.NewBulletParser(),
				}, true
			},
		},
		ExpectedFill: spec.ExpectedFill{
			Build: func(runtime spec.RuntimeConfig) (spec.ExpectedFillOrder, bool) {
				return spec.ExpectedFillOrder{
					Side: sideForExpectedFill(spec.TextParam(runtime.Params, "side", "bid")),
					Size: spec.FloatParam(runtime.Params, "size"),
				}, true
			},
		},
		Classifier:         Classify,
		Confirmation:       ConfirmWebSocket,
		CancelConfirmation: ConfirmCancelWebSocket,
		Docs: []string{
			DocsURL,
			WebSocketDocsURL,
			SigningDocsURL,
			OrderFieldsDocsURL,
			DecimalDocsURL,
			"https://github.com/bulletxyz/bullet-rust-sdk",
		},
		Notes: []string{
			"Bullet is a Sovereign-SDK rollup; orders are borsh-serialized ed25519-signed transactions submitted as an opaque base64 tx blob.",
			"WebSocket order.place at wss://tradingapi.bullet.xyz/ws is the fastest verified order-entry path.",
			"Batch submission places Vec<NewOrderArgs> in one transaction under a single signature, unlike venues that sign each batch action individually; batch latency is not directly comparable.",
			"UniquenessData::Window allows concurrent in-flight transactions, so prebuilt payloads need no nonce serialization.",
			"Trading uses a revocable delegate key; all read endpoints and the user.orders topic key on the main account address, not the delegate address.",
			"Bullet timestamps are microseconds since epoch, not milliseconds.",
		},
	}
}

func BuildCommand() []string {
	return nodeCommand("build_payload.mjs")
}

func cancelCommand() []string {
	return nodeCommand("cancel_payload.mjs")
}

func nodeCommand(script string) []string {
	return []string{
		"node",
		filepath.FromSlash("internal/venues/bullet/" + script),
	}
}

func Classify(in lifecycle.ResponseInput) lifecycle.Classification {
	generic := lifecycle.ClassifyResponse(in)
	// Matches the other venues: once the generic pass has already decided the
	// response is not OK, keep its verdict. Decoding on would collapse a 429 into
	// rejected rather than rate_limited, and the 403 the MM ingress returns to an
	// unlisted address into rejected rather than auth_error — which is exactly the
	// signal an operator needs to see.
	if in.Err != nil || len(in.Body) == 0 || !generic.OK() {
		return generic
	}
	var decoded struct {
		// WebSocket order.place / order.cancel envelope.
		Error *struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		} `json:"error"`
		Results *struct {
			Status string `json:"status"`
			TxID   string `json:"tx_id"`
		} `json:"results"`
		// HTTP POST /tx/submit returns SubmitTxResponse with id and status at
		// the top level. `status` is a TxStatus string there, but the flat HTTP
		// error shape reuses the same key for a numeric HTTP code, so it is
		// decoded raw and discriminated below.
		// "id" is deliberately not decoded: the HTTP envelope carries a string
		// transaction hash while the WebSocket envelope carries a numeric request
		// id, and a typed field for either breaks unmarshalling of the other.
		Status  json.RawMessage `json:"status"`
		Message string          `json:"message"`
		Receipt *struct {
			Result string `json:"result"`
		} `json:"receipt"`
	}
	if err := json.Unmarshal(in.Body, &decoded); err != nil {
		return generic
	}
	if decoded.Error != nil {
		return classifyError(decoded.Error.Code, decoded.Error.Msg)
	}
	if decoded.Results != nil {
		return classifyTxStatus(decoded.Results.Status)
	}
	if status, message, ok := httpSubmitOutcome(decoded.Status, decoded.Message); ok {
		if message != "" {
			return lifecycle.Classification{Status: lifecycle.StatusRejected, Reason: message}
		}
		// A processed transaction can still have reverted or been skipped on
		// chain, which is a rejection rather than a success.
		if decoded.Receipt != nil {
			switch strings.ToLower(strings.TrimSpace(decoded.Receipt.Result)) {
			case "reverted", "skipped":
				return lifecycle.Classification{Status: lifecycle.StatusRejected, Reason: "transaction " + decoded.Receipt.Result}
			}
		}
		return classifyTxStatus(status)
	}
	if generic.Status == lifecycle.StatusAccepted {
		return lifecycle.Classification{Status: lifecycle.StatusUnknown, Reason: "bullet response has neither error nor results"}
	}
	return generic
}

// httpSubmitOutcome discriminates the two shapes that reuse the top-level
// "status" key on POST /tx/submit: a TxStatus string on success, and a numeric
// HTTP status alongside "message" on failure.
func httpSubmitOutcome(raw json.RawMessage, message string) (string, string, bool) {
	if len(raw) == 0 {
		return "", "", false
	}
	var status string
	if err := json.Unmarshal(raw, &status); err == nil {
		return status, "", true
	}
	var code int
	if err := json.Unmarshal(raw, &code); err != nil {
		return "", "", false
	}
	reason := strings.TrimSpace(message)
	if reason == "" {
		reason = fmt.Sprintf("bullet http %d", code)
	}
	return "", reason, true
}

func classifyError(code int, message string) lifecycle.Classification {
	reason := strings.TrimSpace(message)
	if reason == "" {
		reason = fmt.Sprintf("bullet error %d", code)
	}
	switch code {
	case -1003, -1015:
		return lifecycle.Classification{Status: lifecycle.StatusRateLimited, Reason: reason}
	case -1002, -1022, -2014, -2015:
		return lifecycle.Classification{Status: lifecycle.StatusAuthError, Reason: reason}
	case -1021:
		return lifecycle.Classification{Status: lifecycle.StatusNonceError, Reason: reason}
	default:
		return lifecycle.Classification{Status: lifecycle.StatusRejected, Reason: reason}
	}
}

func classifyTxStatus(status string) lifecycle.Classification {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "processed", "published", "finalized":
		return lifecycle.Classification{Status: lifecycle.StatusAccepted}
	case "submitted":
		// Empirically the normal happy-path ack on testnet: every accepted order
		// returns "submitted" rather than "processed". It means the sequencer holds
		// the transaction but has not published it, so it is weaker evidence than
		// the other accepted states. Book entry is verified independently by the
		// ws_confirmation match against ADDRESS@user.orders, so treat it as
		// accepted here rather than leaving every sample classified unknown.
		return lifecycle.Classification{Status: lifecycle.StatusAccepted, Reason: "transaction submitted: acked by sequencer, not yet published"}
	case "dropped":
		return lifecycle.Classification{Status: lifecycle.StatusRejected, Reason: "transaction dropped: expired uniqueness or duplicate generation"}
	default:
		return lifecycle.Classification{Status: lifecycle.StatusUnknown, Reason: "bullet tx status " + status}
	}
}

func sideForExpectedFill(side string) string {
	switch strings.ToLower(strings.TrimSpace(side)) {
	case "bid", "buy":
		return "buy"
	case "ask", "sell":
		return "sell"
	default:
		return strings.ToLower(strings.TrimSpace(side))
	}
}
