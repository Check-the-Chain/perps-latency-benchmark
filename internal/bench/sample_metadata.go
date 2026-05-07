package bench

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

func OrderRefsFromMetadata(metadata map[string]any, key string) []OrderRef {
	raw, ok := metadata[key]
	if !ok || raw == nil {
		return nil
	}
	values := metadataList(raw)
	refs := make([]OrderRef, 0, len(values))
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		ref := OrderRef{
			Venue:            textField(item, "venue"),
			Symbol:           textField(item, "symbol"),
			Market:           textField(item, "market"),
			MarketIndex:      intField(item, "market_index", "marketIndex"),
			Side:             textField(item, "side"),
			Size:             textField(item, "size"),
			Asset:            intField(item, "asset"),
			ClientOrderID:    textField(item, "client_order_id", "clientOrderId", "newClientOrderId"),
			ClientOrderIndex: textField(item, "client_order_index", "orderIndex"),
			OrderIndex:       textField(item, "order_index", "orderIndex"),
			ExternalID:       textField(item, "external_id", "externalId", "externalOrderId"),
			Cloid:            textField(item, "cloid"),
		}
		if ref != (OrderRef{}) {
			refs = append(refs, ref)
		}
	}
	return refs
}

func metadataList(raw any) []any {
	switch typed := raw.(type) {
	case []any:
		return typed
	case []map[string]any:
		values := make([]any, len(typed))
		for i, value := range typed {
			values[i] = value
		}
		return values
	default:
		return nil
	}
}

func OrderRefsToMetadata(refs []OrderRef) []map[string]any {
	if len(refs) == 0 {
		return nil
	}
	items := make([]map[string]any, 0, len(refs))
	for _, ref := range refs {
		item := map[string]any{}
		setString(item, "venue", ref.Venue)
		setString(item, "symbol", ref.Symbol)
		setString(item, "market", ref.Market)
		if ref.MarketIndex != 0 {
			item["market_index"] = ref.MarketIndex
		}
		setString(item, "side", ref.Side)
		setString(item, "size", ref.Size)
		if ref.Asset != 0 || ref.Venue == "hyperliquid" {
			item["asset"] = ref.Asset
		}
		setString(item, "client_order_id", ref.ClientOrderID)
		setString(item, "client_order_index", ref.ClientOrderIndex)
		setString(item, "order_index", ref.OrderIndex)
		setString(item, "external_id", ref.ExternalID)
		setString(item, "cloid", ref.Cloid)
		items = append(items, item)
	}
	return items
}

func DebugMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	copied := make(map[string]any, len(metadata))
	for key, value := range metadata {
		switch key {
		case MetadataCleanupOrdersKey, MetadataCleanupMetadataKey, MetadataExpectedEntryFillKey, MetadataExpectedExitFillKey, MetadataSpeedBumpNSKey, MetadataSpeedBumpMSKey, MetadataSpeedBumpSourceKey, MetadataOrderTypeKey:
			continue
		default:
			copied[key] = value
		}
	}
	if len(copied) == 0 {
		return nil
	}
	return copied
}

func ExpectedFillFromMetadata(metadata map[string]any, key string) *ExpectedFill {
	raw, ok := metadata[key]
	if !ok || raw == nil {
		return nil
	}
	item, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	fill := &ExpectedFill{
		Phase:         textField(item, "phase"),
		Side:          textField(item, "side"),
		Size:          floatField(item, "size"),
		ExpectedPrice: floatField(item, "expected_price"),
		TopBid:        floatField(item, "top_bid"),
		TopAsk:        floatField(item, "top_ask"),
		TopBidSize:    floatField(item, "top_bid_size"),
		TopAskSize:    floatField(item, "top_ask_size"),
		TopAvailable:  floatField(item, "top_available"),
		TopSufficient: boolField(item, "top_sufficient"),
		BookAvailable: floatField(item, "book_available"),
		BookLevels:    intField(item, "book_levels"),
		DepthWeighted: boolField(item, "depth_weighted"),
		BookAgeNS:     int64Field(item, "book_age_ns"),
	}
	if _, ok := item["book_sufficient"]; ok {
		fill.BookSufficient = boolPtr(boolField(item, "book_sufficient"))
	}
	fill.BookReceivedAt = timeField(item, "book_received_at")
	if exchangeAt := timeField(item, "book_exchange_at"); !exchangeAt.IsZero() {
		fill.BookExchangeAt = &exchangeAt
	}
	return fill
}

func textField(item map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := item[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			return typed
		case json.Number:
			return typed.String()
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64)
		case float32:
			return strconv.FormatFloat(float64(typed), 'f', -1, 32)
		default:
			return fmt.Sprint(typed)
		}
	}
	return ""
}

func setString(item map[string]any, key string, value string) {
	if value != "" {
		item[key] = value
	}
}

func intField(item map[string]any, keys ...string) int {
	return int(int64Field(item, keys...))
}

func int64Field(item map[string]any, keys ...string) int64 {
	var value any
	for _, key := range keys {
		var ok bool
		value, ok = item[key]
		if ok && value != nil {
			break
		}
	}
	if value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		got, _ := typed.Int64()
		return got
	case string:
		got, _ := strconv.ParseInt(typed, 10, 64)
		return got
	default:
		return 0
	}
}

func floatField(item map[string]any, key string) float64 {
	value, ok := item[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		got, _ := typed.Float64()
		return got
	case string:
		got, _ := strconv.ParseFloat(typed, 64)
		return got
	default:
		return 0
	}
}

func boolField(item map[string]any, key string) bool {
	value, ok := item[key]
	if !ok || value == nil {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		got, _ := strconv.ParseBool(typed)
		return got
	default:
		return false
	}
}

func timeField(item map[string]any, key string) time.Time {
	text := textField(item, key)
	if text == "" {
		return time.Time{}
	}
	parsed, _ := time.Parse(time.RFC3339Nano, text)
	return parsed
}
