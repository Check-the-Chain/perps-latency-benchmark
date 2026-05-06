package bench

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
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
	if sample.AdjustedNetworkNS > 0 {
		return sample.AdjustedNetworkNS
	}
	raw := RawNetworkNS(sample)
	adjusted := raw - sample.SpeedBumpNS
	if adjusted < 0 {
		return 0
	}
	return adjusted
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
