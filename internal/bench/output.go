package bench

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"

	"perps-latency-benchmark/internal/secrets"
)

func WriteJSON(path string, result Result) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(secrets.RedactValue(result), "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func WriteComparisonJSON(path string, result ComparisonResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(secrets.RedactValue(result), "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func WriteCSV(path string, samples []Sample) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write([]string{
		"venue",
		"run_id",
		"scenario",
		"transport",
		"order_type",
		"measurement_mode",
		"index",
		"iteration",
		"warmup",
		"batch_size",
		"scheduled_at",
		"sent_at",
		"prepared_ns",
		"network_ns",
		"raw_network_ns",
		"adjusted_network_ns",
		"speed_bump_ns",
		"speed_bump_source",
		"submission_ns",
		"corrected_ns",
		"start_delay_ns",
		"write_delay_ns",
		"status_code",
		"bytes_read",
		"ok",
		"classification",
		"classification_reason",
		"cleanup_attempted",
		"cleanup_ok",
		"cleanup_status_code",
		"cleanup_duration_ns",
		"cleanup_prepared_ns",
		"cleanup_start_delay_ns",
		"cleanup_write_delay_ns",
		"cleanup_error",
		"error",
		"completed_at",
	}); err != nil {
		return err
	}

	for _, sample := range samples {
		cleanupAttempted, cleanupOK, cleanupStatusCode, cleanupDurationNS, cleanupError := cleanupCSVFields(sample.Cleanup)
		cleanupPreparedNS, cleanupStartDelayNS, cleanupWriteDelayNS := cleanupTimingCSVFields(sample.Cleanup)
		if err := writer.Write([]string{
			sample.Venue,
			sample.RunID,
			string(sample.Scenario),
			sample.Transport,
			sample.OrderType,
			string(sample.MeasurementMode),
			strconv.Itoa(sample.Index),
			strconv.Itoa(sample.Iteration),
			strconv.FormatBool(sample.Warmup),
			strconv.Itoa(sample.BatchSize),
			sample.ScheduledAt.Format("2006-01-02T15:04:05.000000000Z07:00"),
			sample.SentAt.Format("2006-01-02T15:04:05.000000000Z07:00"),
			strconv.FormatInt(sample.PreparedNS, 10),
			strconv.FormatInt(sample.NetworkNS, 10),
			strconv.FormatInt(rawNetworkNS(sample), 10),
			strconv.FormatInt(adjustedNetworkNS(sample), 10),
			strconv.FormatInt(sample.SpeedBumpNS, 10),
			sample.SpeedBumpSource,
			strconv.FormatInt(sample.SubmissionNS, 10),
			strconv.FormatInt(sample.CorrectedNS, 10),
			strconv.FormatInt(sample.StartDelayNS, 10),
			strconv.FormatInt(sample.WriteDelayNS, 10),
			strconv.Itoa(sample.StatusCode),
			strconv.FormatInt(sample.BytesRead, 10),
			strconv.FormatBool(sample.OK),
			string(sample.Classification.Status),
			secrets.RedactString(sample.Classification.Reason),
			cleanupAttempted,
			cleanupOK,
			cleanupStatusCode,
			cleanupDurationNS,
			cleanupPreparedNS,
			cleanupStartDelayNS,
			cleanupWriteDelayNS,
			cleanupError,
			secrets.RedactString(sample.Error),
			sample.CompletedAt.Format("2006-01-02T15:04:05.000000000Z07:00"),
		}); err != nil {
			return err
		}
	}
	return nil
}

func cleanupTimingCSVFields(cleanup *CleanupResult) (string, string, string) {
	if cleanup == nil {
		return "", "", ""
	}
	return strconv.FormatInt(cleanup.PreparedNS, 10),
		strconv.FormatInt(cleanup.StartDelayNS, 10),
		strconv.FormatInt(cleanup.WriteDelayNS, 10)
}

func cleanupCSVFields(cleanup *CleanupResult) (string, string, string, string, string) {
	if cleanup == nil {
		return "", "", "", "", ""
	}
	return strconv.FormatBool(cleanup.Attempted),
		strconv.FormatBool(cleanup.OK),
		strconv.Itoa(cleanup.StatusCode),
		strconv.FormatInt(cleanup.DurationNS, 10),
		secrets.RedactString(cleanup.Error)
}
