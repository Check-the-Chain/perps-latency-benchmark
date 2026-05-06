package bench

const (
	MetadataCleanupOrdersKey     = "cleanup_orders"
	MetadataCleanupMetadataKey   = "cleanup_metadata"
	MetadataExpectedEntryFillKey = "expected_entry_fill"
	MetadataExpectedExitFillKey  = "expected_exit_fill"
	MetadataOrderTypeKey         = "order_type"
	MetadataSpeedBumpNSKey       = "speed_bump_ns"
	MetadataSpeedBumpMSKey       = "speed_bump_ms"
	MetadataSpeedBumpSourceKey   = "speed_bump_source"
)

type OrderMetadata struct {
	OrderType        string
	CleanupOrderRefs []OrderRef
	SpeedBumpNS      int64
	SpeedBumpSource  string
	Debug            map[string]any
}

func ParseOrderMetadata(metadata map[string]any) OrderMetadata {
	speedBumpNS, speedBumpSource := speedBumpFromMetadata(metadata)
	return OrderMetadata{
		OrderType:        orderType(metadata),
		CleanupOrderRefs: OrderRefsFromMetadata(metadata, MetadataCleanupOrdersKey),
		SpeedBumpNS:      speedBumpNS,
		SpeedBumpSource:  speedBumpSource,
		Debug:            DebugMetadata(metadata),
	}
}
