package bench

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const (
	ExtendedSpeedBumpNS     int64 = 150_000_000
	ExtendedSpeedBumpSource       = "Extended documents a 150 ms taker order-entry speed bump"
)

func speedBumpFromMetadata(metadata map[string]any) (int64, string) {
	if len(metadata) == 0 {
		return 0, ""
	}
	speedBumpNS := numericMetadata(metadata, MetadataSpeedBumpNSKey)
	if speedBumpNS == 0 {
		speedBumpMS := numericMetadata(metadata, MetadataSpeedBumpMSKey)
		if speedBumpMS > 0 {
			speedBumpNS = speedBumpMS * 1_000_000
		}
	}
	if speedBumpNS < 0 {
		speedBumpNS = 0
	}
	return speedBumpNS, stringMetadata(metadata, MetadataSpeedBumpSourceKey)
}

func adjustedNetworkNS(sample Sample) int64 {
	return AdjustedNetworkNS(sample)
}

func rawNetworkNS(sample Sample) int64 {
	return RawNetworkNS(sample)
}

func AdjustedNetworkNS(sample Sample) int64 {
	speedBumpNS := SpeedBumpNS(sample)
	if sample.AdjustedNetworkNS > 0 && speedBumpNS == sample.SpeedBumpNS {
		return sample.AdjustedNetworkNS
	}
	return AdjustForSpeedBumpNS(RawNetworkNS(sample), speedBumpNS)
}

func NetworkAdjustedNetworkNS(sample Sample) (int64, bool) {
	if sample.NetworkFloorNS <= 0 {
		return 0, false
	}
	return ClampLatencyNS(AdjustedNetworkNS(sample) - sample.NetworkFloorNS), true
}

func SpeedBumpNS(sample Sample) int64 {
	if sample.SpeedBumpNS > 0 && !isExtendedNonTaker(sample) {
		return sample.SpeedBumpNS
	}
	if isExtendedTaker(sample) {
		return ExtendedSpeedBumpNS
	}
	return 0
}

func SpeedBumpSource(sample Sample) string {
	if sample.SpeedBumpNS > 0 && !isExtendedNonTaker(sample) {
		if source := strings.TrimSpace(sample.SpeedBumpSource); source != "" {
			return source
		}
	}
	if isExtendedTaker(sample) {
		return ExtendedSpeedBumpSource
	}
	return strings.TrimSpace(sample.SpeedBumpSource)
}

func isExtendedTaker(sample Sample) bool {
	return strings.EqualFold(sample.Venue, "extended") && isTakerOrderType(sample.OrderType)
}

func isExtendedNonTaker(sample Sample) bool {
	return strings.EqualFold(sample.Venue, "extended") && !isTakerOrderType(sample.OrderType)
}

func isTakerOrderType(orderType string) bool {
	switch strings.ToLower(strings.TrimSpace(orderType)) {
	case "market", "ioc":
		return true
	default:
		return false
	}
}

func RawNetworkNS(sample Sample) int64 {
	if sample.RawNetworkNS > 0 {
		return sample.RawNetworkNS
	}
	return sample.NetworkNS
}

func numericMetadata(metadata map[string]any, key string) int64 {
	value, ok := metadata[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	case string:
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return int64(parsed)
	case fmt.Stringer:
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(typed.String()), 64)
		return int64(parsed)
	default:
		return 0
	}
}

func stringMetadata(metadata map[string]any, key string) string {
	value, ok := metadata[key]
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
