package bench

const CleanupSelfHealPositionMetadataKey = "self_heal_position"
const CleanupRestartBeforeMeasurementMetadataKey = "restart_before_measurement"

func IsPositionSelfHealCleanup(cleanup *CleanupResult) bool {
	return cleanupBoolMetadata(cleanup, CleanupSelfHealPositionMetadataKey)
}

func CleanupRequiresRestartBeforeMeasurement(cleanup *CleanupResult) bool {
	return cleanupBoolMetadata(cleanup, CleanupRestartBeforeMeasurementMetadataKey)
}

func cleanupBoolMetadata(cleanup *CleanupResult, key string) bool {
	if cleanup == nil || len(cleanup.Metadata) == 0 {
		return false
	}
	value, ok := cleanup.Metadata[key]
	if !ok {
		return false
	}
	if boolValue, ok := value.(bool); ok {
		return boolValue
	}
	text, ok := value.(string)
	return ok && (text == "true" || text == "1")
}
