package bench

import (
	"fmt"
	"strings"
)

const (
	CleanupConfirmationMetadataKey          = "cleanup_confirmation"
	CleanupConfirmationAccountFeed          = "account_feed"
	CleanupConfirmationTransportMetadataKey = "cleanup_confirmation_transport"
)

func CancelLatencyNS(sample Sample) (int64, bool) {
	if sample.Cleanup == nil || !sample.Cleanup.OK || sample.Cleanup.DurationNS <= 0 {
		return 0, false
	}
	if !IsAccountFeedCancelCleanup(sample.Cleanup) {
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

func IsAccountFeedCancelCleanup(cleanup *CleanupResult) bool {
	if !IsCancelCleanup(cleanup) {
		return false
	}
	confirmation, ok := cleanup.Metadata[CleanupConfirmationMetadataKey]
	return ok && strings.EqualFold(strings.TrimSpace(anyString(confirmation)), CleanupConfirmationAccountFeed)
}

func anyString(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}
