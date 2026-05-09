package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/lifecycle"
)

type sampleRow struct {
	sample               bench.Sample
	latencyMode          bench.LatencyMode
	completedAt          string
	scheduledAt          string
	sentAt               string
	scenario             string
	ok                   int
	status               string
	cleanupAttempted     int
	cleanupOK            int
	cleanupStatusCode    int
	cleanupDurationNS    int64
	cleanupPreparedNS    int64
	cleanupScheduledAt   string
	cleanupSentAt        string
	cleanupStartDelayNS  int64
	cleanupWriteDelayNS  int64
	cleanupBytesRead     int64
	cleanupError         string
	cleanupDescription   string
	cleanupMetadataJSON  string
	orderRefsJSON        string
	closeoutRefsJSON     string
	expectedEntryJSON    string
	expectedExitJSON     string
	metadataJSON         string
	classificationReason string
	measurementMode      string
	rawNetworkNS         int64
	adjustedNetworkNS    int64
	networkFloorNS       int64
	networkFloorSource   string
	speedBumpNS          int64
	speedBumpSource      string
	submissionNS         int64
	startDelayNS         int64
	writeDelayNS         int64
	orderType            string
	transport            string
	venue                string
	runID                string
	iteration            int
	batchSize            int
	networkNS            int64
	errorText            string
}

func sampleInsertValues(record SampleRecord) ([]any, bool, error) {
	sample := record.Sample
	if sample.Warmup {
		return nil, false, nil
	}
	cleanupAttempted, cleanupOK := cleanupFields(sample.Cleanup)
	cleanupStatusCode, cleanupDurationNS, cleanupPreparedNS, cleanupScheduledAt, cleanupSentAt, cleanupStartDelayNS, cleanupWriteDelayNS, cleanupBytesRead, cleanupError, cleanupDescription := cleanupStorageFields(sample.Cleanup)
	cleanupMetadataJSON, err := cleanupMetadataJSON(sample.Cleanup)
	if err != nil {
		return nil, false, err
	}
	metadataJSON, err := metadataJSON(sample.Metadata)
	if err != nil {
		return nil, false, err
	}
	orderRefsJSON, err := marshalJSON(sample.OrderRefs)
	if err != nil {
		return nil, false, err
	}
	closeoutRefsJSON, err := marshalJSON(sample.CloseoutOrderRefs)
	if err != nil {
		return nil, false, err
	}
	entryFillProjection := expectedFillProjection(sample.ExpectedEntryFill)
	exitFillProjection := expectedFillProjection(sample.ExpectedExitFill)
	expectedEntryJSON, err := marshalOptionalJSON(sample.ExpectedEntryFill)
	if err != nil {
		return nil, false, err
	}
	expectedExitJSON, err := marshalOptionalJSON(sample.ExpectedExitFill)
	if err != nil {
		return nil, false, err
	}
	return []any{
		sample.CompletedAt.UTC().Format(time.RFC3339Nano),
		formatOptionalTime(sample.ScheduledAt),
		formatOptionalTime(sample.SentAt),
		sample.Venue,
		sample.RunID,
		string(sample.Scenario),
		sample.Transport,
		sample.OrderType,
		string(record.LatencyMode),
		string(sample.MeasurementMode),
		sample.Iteration,
		sample.BatchSize,
		sample.NetworkNS,
		bench.RawNetworkNS(sample),
		bench.AdjustedNetworkNS(sample),
		sample.NetworkFloorNS,
		sample.NetworkFloorSource,
		sample.SpeedBumpNS,
		sample.SpeedBumpSource,
		sample.SubmissionNS,
		sample.StartDelayNS,
		sample.WriteDelayNS,
		boolInt(sample.OK),
		string(sample.Classification.Status),
		sample.Classification.Reason,
		cleanupAttempted,
		cleanupOK,
		cleanupStatusCode,
		cleanupDurationNS,
		cleanupPreparedNS,
		cleanupScheduledAt,
		cleanupSentAt,
		cleanupStartDelayNS,
		cleanupWriteDelayNS,
		cleanupBytesRead,
		cleanupError,
		cleanupDescription,
		cleanupConfirmation(sample.Cleanup),
		cleanupMetadataJSON,
		batchSubmission(sample),
		entryFillProjection.side,
		entryFillProjection.expectedPrice,
		entryFillProjection.bookSufficient,
		entryFillProjection.topSufficient,
		exitFillProjection.side,
		exitFillProjection.expectedPrice,
		exitFillProjection.bookSufficient,
		exitFillProjection.topSufficient,
		orderRefsJSON,
		closeoutRefsJSON,
		expectedEntryJSON,
		expectedExitJSON,
		metadataJSON,
		sample.Error,
	}, true, nil
}

type expectedFillStorageProjection struct {
	side           string
	expectedPrice  float64
	bookSufficient int
	topSufficient  int
}

func expectedFillProjection(fill *bench.ExpectedFill) expectedFillStorageProjection {
	if fill == nil {
		return expectedFillStorageProjection{
			bookSufficient: -1,
			topSufficient:  -1,
		}
	}
	return expectedFillStorageProjection{
		side:           fill.Side,
		expectedPrice:  fill.ExpectedPrice,
		bookSufficient: boolPtrState(fill.BookSufficient),
		topSufficient:  boolState(fill.TopSufficient),
	}
}

func boolPtrState(value *bool) int {
	if value == nil {
		return -1
	}
	return boolState(*value)
}

func boolState(value bool) int {
	if value {
		return 1
	}
	return 0
}

func cleanupConfirmation(cleanup *bench.CleanupResult) string {
	if cleanup == nil || len(cleanup.Metadata) == 0 {
		return ""
	}
	return strings.TrimSpace(anyString(cleanup.Metadata[bench.CleanupConfirmationMetadataKey]))
}

func batchSubmission(sample bench.Sample) string {
	if sample.Scenario != bench.ScenarioBatch {
		return ""
	}
	if value, ok := sample.Metadata["native_batch_endpoint"]; ok {
		if native, ok := value.(bool); ok {
			if native {
				return "native"
			}
			return "manual"
		}
	}
	if model, ok := sample.Metadata["submission_model"].(string); ok && model != "" {
		return "manual"
	}
	return "native"
}

func anyString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}

func (r *sampleRow) scanDestinations() []any {
	return []any{
		&r.completedAt,
		&r.scheduledAt,
		&r.sentAt,
		&r.venue,
		&r.runID,
		&r.scenario,
		&r.transport,
		&r.orderType,
		&r.measurementMode,
		&r.iteration,
		&r.batchSize,
		&r.networkNS,
		&r.rawNetworkNS,
		&r.adjustedNetworkNS,
		&r.networkFloorNS,
		&r.networkFloorSource,
		&r.speedBumpNS,
		&r.speedBumpSource,
		&r.submissionNS,
		&r.startDelayNS,
		&r.writeDelayNS,
		&r.ok,
		&r.status,
		&r.classificationReason,
		&r.cleanupAttempted,
		&r.cleanupOK,
		&r.cleanupStatusCode,
		&r.cleanupDurationNS,
		&r.cleanupPreparedNS,
		&r.cleanupScheduledAt,
		&r.cleanupSentAt,
		&r.cleanupStartDelayNS,
		&r.cleanupWriteDelayNS,
		&r.cleanupBytesRead,
		&r.cleanupError,
		&r.cleanupDescription,
		&r.cleanupMetadataJSON,
		&r.orderRefsJSON,
		&r.closeoutRefsJSON,
		&r.expectedEntryJSON,
		&r.expectedExitJSON,
		&r.metadataJSON,
		&r.errorText,
	}
}

func (r sampleRow) toSample() (bench.Sample, error) {
	parsed, err := time.Parse(time.RFC3339Nano, r.completedAt)
	if err != nil {
		return bench.Sample{}, fmt.Errorf("parse completed_at: %w", err)
	}
	sample := bench.Sample{
		CompletedAt:        parsed,
		ScheduledAt:        parseOptionalTime(r.scheduledAt),
		SentAt:             parseOptionalTime(r.sentAt),
		Venue:              r.venue,
		RunID:              r.runID,
		Scenario:           bench.Scenario(r.scenario),
		Transport:          r.transport,
		OrderType:          r.orderType,
		MeasurementMode:    bench.MeasurementMode(r.measurementMode),
		Iteration:          r.iteration,
		BatchSize:          r.batchSize,
		NetworkNS:          r.networkNS,
		RawNetworkNS:       r.rawNetworkNS,
		AdjustedNetworkNS:  r.adjustedNetworkNS,
		NetworkFloorNS:     r.networkFloorNS,
		NetworkFloorSource: r.networkFloorSource,
		SpeedBumpNS:        r.speedBumpNS,
		SpeedBumpSource:    r.speedBumpSource,
		SubmissionNS:       r.submissionNS,
		StartDelayNS:       r.startDelayNS,
		WriteDelayNS:       r.writeDelayNS,
		OK:                 r.ok == 1,
		Classification: lifecycle.Classification{
			Status: lifecycle.ClassificationStatus(r.status),
			Reason: r.classificationReason,
		},
		Error: r.errorText,
		Cleanup: &bench.CleanupResult{
			Attempted:    r.cleanupAttempted == 1,
			OK:           r.cleanupOK == 1,
			StatusCode:   r.cleanupStatusCode,
			Error:        r.cleanupError,
			DurationNS:   r.cleanupDurationNS,
			PreparedNS:   r.cleanupPreparedNS,
			ScheduledAt:  parseOptionalTime(r.cleanupScheduledAt),
			SentAt:       parseOptionalTime(r.cleanupSentAt),
			StartDelayNS: r.cleanupStartDelayNS,
			WriteDelayNS: r.cleanupWriteDelayNS,
			BytesRead:    r.cleanupBytesRead,
			Description:  r.cleanupDescription,
		},
	}
	if sample.RawNetworkNS == 0 {
		sample.RawNetworkNS = sample.NetworkNS
	}
	if sample.AdjustedNetworkNS == 0 && sample.NetworkNS > 0 {
		sample.AdjustedNetworkNS = sample.NetworkNS - sample.SpeedBumpNS
		if sample.AdjustedNetworkNS < 0 {
			sample.AdjustedNetworkNS = 0
		}
	}
	if r.cleanupMetadataJSON != "" {
		_ = json.Unmarshal([]byte(r.cleanupMetadataJSON), &sample.Cleanup.Metadata)
	}
	if r.metadataJSON != "" {
		_ = json.Unmarshal([]byte(r.metadataJSON), &sample.Metadata)
	}
	if r.orderRefsJSON != "" {
		_ = json.Unmarshal([]byte(r.orderRefsJSON), &sample.OrderRefs)
	}
	if r.closeoutRefsJSON != "" {
		_ = json.Unmarshal([]byte(r.closeoutRefsJSON), &sample.CloseoutOrderRefs)
	}
	if r.expectedEntryJSON != "" {
		var fill bench.ExpectedFill
		if err := json.Unmarshal([]byte(r.expectedEntryJSON), &fill); err == nil {
			sample.ExpectedEntryFill = &fill
		}
	}
	if r.expectedExitJSON != "" {
		var fill bench.ExpectedFill
		if err := json.Unmarshal([]byte(r.expectedExitJSON), &fill); err == nil {
			sample.ExpectedExitFill = &fill
		}
	}
	return sample, nil
}
