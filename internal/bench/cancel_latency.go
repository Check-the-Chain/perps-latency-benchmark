package bench

import "strings"

func CancelLatencyNS(sample Sample) (int64, bool) {
	if sample.Cleanup == nil || !sample.Cleanup.OK || sample.Cleanup.DurationNS <= 0 {
		return 0, false
	}
	if !IsCancelCleanup(sample.Cleanup) {
		return 0, false
	}
	return sample.Cleanup.DurationNS, true
}

func IsCancelCleanup(cleanup *CleanupResult) bool {
	if cleanup == nil {
		return false
	}
	operation := strings.ToLower(strings.TrimSpace(cleanup.Description))
	return strings.Contains(operation, "cancel")
}
