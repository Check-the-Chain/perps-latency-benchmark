package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"perps-latency-benchmark/internal/bench"
)

const extendedVenueSpeedBumpNS int64 = 150_000_000

type DashboardSamplesReadModel struct {
	Samples []DashboardSample `json:"samples"`
}

type DashboardLatencySeriesReadModel struct {
	Samples []DashboardLatencySample `json:"samples"`
}

type DashboardTakerCostSamplesReadModel struct {
	Samples []DashboardTakerCostSample `json:"samples"`
}

type DashboardSample struct {
	Venue             string                 `json:"venue"`
	RunID             string                 `json:"run_id,omitempty"`
	Scenario          string                 `json:"scenario"`
	Transport         string                 `json:"transport"`
	OrderType         string                 `json:"order_type,omitempty"`
	Iteration         int                    `json:"iteration"`
	Warmup            bool                   `json:"warmup"`
	BatchSize         int                    `json:"batch_size"`
	ScheduledAt       time.Time              `json:"scheduled_at,omitempty"`
	SentAt            time.Time              `json:"sent_at,omitempty"`
	NetworkNS         int64                  `json:"network_ns"`
	RawNetworkNS      int64                  `json:"raw_network_ns,omitempty"`
	AdjustedNetworkNS int64                  `json:"adjusted_network_ns,omitempty"`
	NetworkFloorNS    int64                  `json:"network_floor_ns,omitempty"`
	SpeedBumpNS       int64                  `json:"speed_bump_ns,omitempty"`
	SubmissionNS      int64                  `json:"submission_ns,omitempty"`
	OK                bool                   `json:"ok"`
	Cleanup           *DashboardCleanup      `json:"cleanup,omitempty"`
	Cost              *DashboardSampleCost   `json:"cost,omitempty"`
	ExpectedEntryFill *DashboardExpectedFill `json:"expected_entry_fill,omitempty"`
	ExpectedExitFill  *DashboardExpectedFill `json:"expected_exit_fill,omitempty"`
	BatchSubmission   string                 `json:"batch_submission,omitempty"`
	MeasurementMode   string                 `json:"measurement_mode,omitempty"`
	CompletedAt       time.Time              `json:"completed_at"`
}

type DashboardLatencySample struct {
	PlotAt             time.Time `json:"plot_at"`
	Venue              string    `json:"venue"`
	Scenario           string    `json:"scenario"`
	OrderType          string    `json:"order_type,omitempty"`
	BatchSize          int       `json:"batch_size"`
	ConfirmNS          int64     `json:"confirm_ns,omitempty"`
	NetworkFloorNS     int64     `json:"network_floor_ns,omitempty"`
	OK                 bool      `json:"ok"`
	CleanupConfirmNS   int64     `json:"cleanup_confirm_ns,omitempty"`
	CleanupAccountFeed bool      `json:"cleanup_account_feed,omitempty"`
	BatchSubmission    string    `json:"batch_submission,omitempty"`
}

type DashboardTakerCostSample struct {
	PlotAt            time.Time              `json:"plot_at"`
	CompletedAt       time.Time              `json:"completed_at"`
	Venue             string                 `json:"venue"`
	Scenario          string                 `json:"scenario"`
	OrderType         string                 `json:"order_type,omitempty"`
	BatchSize         int                    `json:"batch_size"`
	OK                bool                   `json:"ok"`
	Cost              DashboardSampleCost    `json:"cost"`
	ExpectedEntryFill *DashboardExpectedFill `json:"expected_entry_fill,omitempty"`
	ExpectedExitFill  *DashboardExpectedFill `json:"expected_exit_fill,omitempty"`
}

type DashboardSampleCost struct {
	Venue                 string    `json:"venue"`
	RunID                 string    `json:"run_id,omitempty"`
	CompletedAt           time.Time `json:"completed_at,omitempty"`
	EntryOrderID          string    `json:"entry_order_id,omitempty"`
	ExitOrderID           string    `json:"exit_order_id,omitempty"`
	EntryQty              float64   `json:"entry_qty,omitempty"`
	ExitQty               float64   `json:"exit_qty,omitempty"`
	EntryValueUSD         float64   `json:"entry_value_usd,omitempty"`
	ExitValueUSD          float64   `json:"exit_value_usd,omitempty"`
	EntryFeeUSD           float64   `json:"entry_fee_usd,omitempty"`
	ExitFeeUSD            float64   `json:"exit_fee_usd,omitempty"`
	PriceMoveCostUSD      float64   `json:"price_move_cost_usd,omitempty"`
	TradeCostUSD          float64   `json:"trade_cost_usd,omitempty"`
	BalanceBeforeUSD      float64   `json:"balance_before_usd,omitempty"`
	BalanceAfterUSD       float64   `json:"balance_after_usd,omitempty"`
	BalanceDeltaUSD       float64   `json:"balance_delta_usd,omitempty"`
	ReconciliationDiffUSD float64   `json:"reconciliation_diff_usd,omitempty"`
	Clean                 bool      `json:"clean"`
	QualityReason         string    `json:"quality_reason,omitempty"`
}

type DashboardCleanup struct {
	OK                  bool   `json:"ok"`
	DurationNS          int64  `json:"duration_ns,omitempty"`
	Description         string `json:"description,omitempty"`
	CleanupConfirmation string `json:"cleanup_confirmation,omitempty"`
}

type DashboardExpectedFill struct {
	Side           string  `json:"side,omitempty"`
	ExpectedPrice  float64 `json:"expected_price,omitempty"`
	BookSufficient *bool   `json:"book_sufficient,omitempty"`
	TopSufficient  *bool   `json:"top_sufficient,omitempty"`
}

func (s *SQLite) RecentDashboardSamples(ctx context.Context, since time.Time, limit int) (DashboardSamplesReadModel, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT completed_at, scheduled_at, sent_at, venue, run_id, scenario, transport, order_type, measurement_mode, iteration, batch_size,
       network_ns, raw_network_ns, adjusted_network_ns, network_floor_ns, speed_bump_ns, submission_ns, ok,
       cleanup_attempted, cleanup_ok, cleanup_duration_ns, cleanup_description, cleanup_confirmation, cleanup_metadata_json,
       batch_submission, metadata_json,
       expected_entry_side, expected_entry_expected_price, expected_entry_book_sufficient, expected_entry_top_sufficient, expected_entry_fill_json,
       expected_exit_side, expected_exit_expected_price, expected_exit_book_sufficient, expected_exit_top_sufficient, expected_exit_fill_json
FROM samples
WHERE completed_at >= ?
  AND transport != ''
  AND order_type != ''
  AND measurement_mode != ''
ORDER BY completed_at DESC
LIMIT ?
`, since.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return DashboardSamplesReadModel{}, err
	}
	defer rows.Close()

	var samples []DashboardSample
	for rows.Next() {
		var row dashboardSampleRow
		if err := rows.Scan(row.scanDestinations()...); err != nil {
			return DashboardSamplesReadModel{}, err
		}
		sample, err := row.toDashboardSample()
		if err != nil {
			return DashboardSamplesReadModel{}, err
		}
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return DashboardSamplesReadModel{}, err
	}
	costs, err := s.recentDashboardSampleCosts(ctx, since)
	if err != nil {
		return DashboardSamplesReadModel{}, err
	}
	for index := range samples {
		if cost, ok := costs[sampleCostKeyFrom(samples[index].CompletedAt, samples[index].Venue, samples[index].RunID)]; ok {
			copied := cost
			samples[index].Cost = &copied
		}
	}
	return DashboardSamplesReadModel{Samples: samples}, nil
}

func (s *SQLite) RecentDashboardLatencySeries(ctx context.Context, since time.Time, limit int) (DashboardLatencySeriesReadModel, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT completed_at, scheduled_at, sent_at, venue, scenario, order_type, batch_size,
       network_ns, raw_network_ns, adjusted_network_ns, network_floor_ns, speed_bump_ns, ok,
       cleanup_ok, cleanup_duration_ns, cleanup_description, cleanup_confirmation,
       CASE WHEN cleanup_confirmation = '' THEN cleanup_metadata_json ELSE '' END,
       batch_submission,
       CASE WHEN batch_submission = '' AND scenario = 'batch' THEN metadata_json ELSE '' END
FROM samples
WHERE completed_at >= ?
  AND transport != ''
  AND order_type != ''
  AND measurement_mode != ''
ORDER BY completed_at DESC
LIMIT ?
`, since.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return DashboardLatencySeriesReadModel{}, err
	}
	defer rows.Close()

	var samples []DashboardLatencySample
	for rows.Next() {
		var row dashboardLatencyRow
		if err := rows.Scan(row.scanDestinations()...); err != nil {
			return DashboardLatencySeriesReadModel{}, err
		}
		sample, err := row.toDashboardLatencySample()
		if err != nil {
			return DashboardLatencySeriesReadModel{}, err
		}
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return DashboardLatencySeriesReadModel{}, err
	}
	return DashboardLatencySeriesReadModel{Samples: samples}, nil
}

func (s *SQLite) RecentDashboardTakerCostSamples(ctx context.Context, since time.Time, limit int) (DashboardTakerCostSamplesReadModel, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT s.completed_at, s.scheduled_at, s.sent_at, s.venue, s.scenario, s.order_type, s.batch_size, s.ok,
       s.expected_entry_side, s.expected_entry_expected_price, s.expected_entry_book_sufficient, s.expected_entry_top_sufficient,
       CASE WHEN s.expected_entry_side = '' AND s.expected_entry_expected_price <= 0 THEN s.expected_entry_fill_json ELSE '' END,
       s.expected_exit_side, s.expected_exit_expected_price, s.expected_exit_book_sufficient, s.expected_exit_top_sufficient,
       CASE WHEN s.expected_exit_side = '' AND s.expected_exit_expected_price <= 0 THEN s.expected_exit_fill_json ELSE '' END,
       c.entry_order_id, c.exit_order_id, c.entry_qty, c.exit_qty,
       c.entry_value_usd, c.exit_value_usd, c.entry_fee_usd, c.exit_fee_usd,
       c.price_move_cost_usd, c.trade_cost_usd, c.balance_before_usd, c.balance_after_usd,
       c.balance_delta_usd, c.reconciliation_diff_usd, c.clean, c.quality_reason, c.run_id
FROM samples s
JOIN sample_costs c
  ON c.completed_at = s.completed_at
 AND c.venue = s.venue
 AND c.run_id = s.run_id
WHERE s.completed_at >= ?
  AND s.transport != ''
  AND s.order_type != ''
  AND s.measurement_mode != ''
ORDER BY s.completed_at DESC
LIMIT ?
`, since.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return DashboardTakerCostSamplesReadModel{}, err
	}
	defer rows.Close()

	var samples []DashboardTakerCostSample
	for rows.Next() {
		var row dashboardTakerCostRow
		if err := rows.Scan(row.scanDestinations()...); err != nil {
			return DashboardTakerCostSamplesReadModel{}, err
		}
		sample, err := row.toDashboardTakerCostSample()
		if err != nil {
			return DashboardTakerCostSamplesReadModel{}, err
		}
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return DashboardTakerCostSamplesReadModel{}, err
	}
	return DashboardTakerCostSamplesReadModel{Samples: samples}, nil
}

type dashboardSampleRow struct {
	completedAt                 string
	scheduledAt                 string
	sentAt                      string
	venue                       string
	runID                       string
	scenario                    string
	transport                   string
	orderType                   string
	measurementMode             string
	iteration                   int
	batchSize                   int
	networkNS                   int64
	rawNetworkNS                int64
	adjustedNetworkNS           int64
	networkFloorNS              int64
	speedBumpNS                 int64
	submissionNS                int64
	ok                          int
	cleanupAttempted            int
	cleanupOK                   int
	cleanupDurationNS           int64
	cleanupDescription          string
	cleanupConfirmation         string
	cleanupMetadataJSON         string
	batchSubmission             string
	metadataJSON                string
	expectedEntrySide           string
	expectedEntryExpectedPrice  float64
	expectedEntryBookSufficient int
	expectedEntryTopSufficient  int
	expectedEntryJSON           string
	expectedExitSide            string
	expectedExitExpectedPrice   float64
	expectedExitBookSufficient  int
	expectedExitTopSufficient   int
	expectedExitJSON            string
}

func (r *dashboardSampleRow) scanDestinations() []any {
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
		&r.speedBumpNS,
		&r.submissionNS,
		&r.ok,
		&r.cleanupAttempted,
		&r.cleanupOK,
		&r.cleanupDurationNS,
		&r.cleanupDescription,
		&r.cleanupConfirmation,
		&r.cleanupMetadataJSON,
		&r.batchSubmission,
		&r.metadataJSON,
		&r.expectedEntrySide,
		&r.expectedEntryExpectedPrice,
		&r.expectedEntryBookSufficient,
		&r.expectedEntryTopSufficient,
		&r.expectedEntryJSON,
		&r.expectedExitSide,
		&r.expectedExitExpectedPrice,
		&r.expectedExitBookSufficient,
		&r.expectedExitTopSufficient,
		&r.expectedExitJSON,
	}
}

func (r dashboardSampleRow) toDashboardSample() (DashboardSample, error) {
	completedAt, err := time.Parse(time.RFC3339Nano, r.completedAt)
	if err != nil {
		return DashboardSample{}, err
	}
	sample := DashboardSample{
		CompletedAt:       completedAt,
		ScheduledAt:       parseOptionalTime(r.scheduledAt),
		SentAt:            parseOptionalTime(r.sentAt),
		Venue:             r.venue,
		RunID:             r.runID,
		Scenario:          r.scenario,
		Transport:         r.transport,
		OrderType:         r.orderType,
		MeasurementMode:   r.measurementMode,
		Iteration:         r.iteration,
		BatchSize:         r.batchSize,
		NetworkNS:         r.networkNS,
		RawNetworkNS:      r.rawNetworkNS,
		AdjustedNetworkNS: r.adjustedNetworkNS,
		NetworkFloorNS:    r.networkFloorNS,
		SpeedBumpNS:       r.speedBumpNS,
		SubmissionNS:      r.submissionNS,
		OK:                r.ok == 1,
		BatchSubmission:   r.batchSubmission,
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
	if sample.BatchSubmission == "" && sample.Scenario == string(bench.ScenarioBatch) {
		sample.BatchSubmission = legacyBatchSubmission(r.metadataJSON)
	}
	if r.cleanupAttempted == 1 || r.cleanupDurationNS > 0 || r.cleanupDescription != "" {
		confirmation := r.cleanupConfirmation
		if confirmation == "" {
			confirmation = legacyCleanupConfirmation(r.cleanupMetadataJSON)
		}
		sample.Cleanup = &DashboardCleanup{
			OK:                  r.cleanupOK == 1,
			DurationNS:          r.cleanupDurationNS,
			Description:         r.cleanupDescription,
			CleanupConfirmation: confirmation,
		}
	}
	sample.ExpectedEntryFill = dashboardExpectedFill(
		r.expectedEntrySide,
		r.expectedEntryExpectedPrice,
		r.expectedEntryBookSufficient,
		r.expectedEntryTopSufficient,
		r.expectedEntryJSON,
	)
	sample.ExpectedExitFill = dashboardExpectedFill(
		r.expectedExitSide,
		r.expectedExitExpectedPrice,
		r.expectedExitBookSufficient,
		r.expectedExitTopSufficient,
		r.expectedExitJSON,
	)
	return sample, nil
}

func dashboardExpectedFill(side string, expectedPrice float64, bookSufficient int, topSufficient int, legacyJSON string) *DashboardExpectedFill {
	if side == "" && expectedPrice <= 0 && bookSufficient < 0 && topSufficient < 0 {
		return legacyDashboardExpectedFill(legacyJSON)
	}
	return &DashboardExpectedFill{
		Side:           side,
		ExpectedPrice:  expectedPrice,
		BookSufficient: boolPtrFromState(bookSufficient),
		TopSufficient:  boolPtrFromState(topSufficient),
	}
}

func boolPtrFromState(value int) *bool {
	if value < 0 {
		return nil
	}
	result := value == 1
	return &result
}

func legacyBatchSubmission(metadataJSON string) string {
	if metadataJSON == "" {
		return "native"
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		return "native"
	}
	if native, ok := metadata["native_batch_endpoint"].(bool); ok {
		if native {
			return "native"
		}
		return "manual"
	}
	if model, ok := metadata["submission_model"].(string); ok && model != "" {
		return "manual"
	}
	return "native"
}

func legacyCleanupConfirmation(metadataJSON string) string {
	if metadataJSON == "" {
		return ""
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		return ""
	}
	return anyString(metadata[bench.CleanupConfirmationMetadataKey])
}

func legacyDashboardExpectedFill(fillJSON string) *DashboardExpectedFill {
	if fillJSON == "" {
		return nil
	}
	var fill struct {
		Side           string  `json:"side"`
		ExpectedPrice  float64 `json:"expected_price"`
		BookSufficient *bool   `json:"book_sufficient"`
		TopSufficient  *bool   `json:"top_sufficient"`
	}
	if err := json.Unmarshal([]byte(fillJSON), &fill); err != nil {
		return nil
	}
	if fill.Side == "" && fill.ExpectedPrice <= 0 && fill.BookSufficient == nil && fill.TopSufficient == nil {
		return nil
	}
	return &DashboardExpectedFill{
		Side:           fill.Side,
		ExpectedPrice:  fill.ExpectedPrice,
		BookSufficient: fill.BookSufficient,
		TopSufficient:  fill.TopSufficient,
	}
}

func (s *SQLite) recentDashboardSampleCosts(ctx context.Context, since time.Time) (map[string]DashboardSampleCost, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT completed_at, venue, run_id, entry_order_id, exit_order_id, entry_qty, exit_qty,
       entry_value_usd, exit_value_usd, entry_fee_usd, exit_fee_usd, price_move_cost_usd, trade_cost_usd,
       balance_before_usd, balance_after_usd, balance_delta_usd, reconciliation_diff_usd, clean,
       quality_reason
FROM sample_costs
WHERE completed_at >= ?
`, since.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	costs := make(map[string]DashboardSampleCost)
	for rows.Next() {
		var cost DashboardSampleCost
		var completedAt string
		var clean int
		if err := rows.Scan(
			&completedAt,
			&cost.Venue,
			&cost.RunID,
			&cost.EntryOrderID,
			&cost.ExitOrderID,
			&cost.EntryQty,
			&cost.ExitQty,
			&cost.EntryValueUSD,
			&cost.ExitValueUSD,
			&cost.EntryFeeUSD,
			&cost.ExitFeeUSD,
			&cost.PriceMoveCostUSD,
			&cost.TradeCostUSD,
			&cost.BalanceBeforeUSD,
			&cost.BalanceAfterUSD,
			&cost.BalanceDeltaUSD,
			&cost.ReconciliationDiffUSD,
			&clean,
			&cost.QualityReason,
		); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, completedAt)
		if err != nil {
			return nil, err
		}
		cost.CompletedAt = parsed
		cost.Clean = clean == 1
		costs[sampleCostKeyFrom(cost.CompletedAt, cost.Venue, cost.RunID)] = cost
	}
	return costs, rows.Err()
}

type dashboardLatencyRow struct {
	completedAt         string
	scheduledAt         string
	sentAt              string
	venue               string
	scenario            string
	orderType           string
	batchSize           int
	networkNS           int64
	rawNetworkNS        int64
	adjustedNetworkNS   int64
	networkFloorNS      int64
	speedBumpNS         int64
	ok                  int
	cleanupOK           int
	cleanupDurationNS   int64
	cleanupDescription  string
	cleanupConfirmation string
	cleanupMetadataJSON string
	batchSubmission     string
	metadataJSON        string
}

func (r *dashboardLatencyRow) scanDestinations() []any {
	return []any{
		&r.completedAt,
		&r.scheduledAt,
		&r.sentAt,
		&r.venue,
		&r.scenario,
		&r.orderType,
		&r.batchSize,
		&r.networkNS,
		&r.rawNetworkNS,
		&r.adjustedNetworkNS,
		&r.networkFloorNS,
		&r.speedBumpNS,
		&r.ok,
		&r.cleanupOK,
		&r.cleanupDurationNS,
		&r.cleanupDescription,
		&r.cleanupConfirmation,
		&r.cleanupMetadataJSON,
		&r.batchSubmission,
		&r.metadataJSON,
	}
}

func (r dashboardLatencyRow) toDashboardLatencySample() (DashboardLatencySample, error) {
	completedAt, err := time.Parse(time.RFC3339Nano, r.completedAt)
	if err != nil {
		return DashboardLatencySample{}, err
	}
	batchSubmission := r.batchSubmission
	if batchSubmission == "" && r.scenario == string(bench.ScenarioBatch) {
		batchSubmission = legacyBatchSubmission(r.metadataJSON)
	}
	confirmation := r.cleanupConfirmation
	if confirmation == "" {
		confirmation = legacyCleanupConfirmation(r.cleanupMetadataJSON)
	}
	return DashboardLatencySample{
		PlotAt:             firstNonZeroTime(parseOptionalTime(r.scheduledAt), parseOptionalTime(r.sentAt), completedAt),
		Venue:              r.venue,
		Scenario:           r.scenario,
		OrderType:          r.orderType,
		BatchSize:          r.batchSize,
		ConfirmNS:          latencyConfirmNS(r.venue, r.orderType, r.networkNS, r.rawNetworkNS, r.adjustedNetworkNS, r.speedBumpNS),
		NetworkFloorNS:     r.networkFloorNS,
		OK:                 r.ok == 1,
		CleanupConfirmNS:   accountFeedCancelCleanupNS(r.cleanupOK == 1, r.cleanupDurationNS, r.cleanupDescription, confirmation),
		CleanupAccountFeed: isAccountFeedCancelCleanup(r.cleanupOK == 1, r.cleanupDescription, confirmation),
		BatchSubmission:    batchSubmission,
	}, nil
}

type dashboardTakerCostRow struct {
	completedAt                 string
	scheduledAt                 string
	sentAt                      string
	venue                       string
	scenario                    string
	orderType                   string
	batchSize                   int
	ok                          int
	expectedEntrySide           string
	expectedEntryExpectedPrice  float64
	expectedEntryBookSufficient int
	expectedEntryTopSufficient  int
	expectedEntryJSON           string
	expectedExitSide            string
	expectedExitExpectedPrice   float64
	expectedExitBookSufficient  int
	expectedExitTopSufficient   int
	expectedExitJSON            string
	cost                        DashboardSampleCost
	clean                       int
}

func (r *dashboardTakerCostRow) scanDestinations() []any {
	return []any{
		&r.completedAt,
		&r.scheduledAt,
		&r.sentAt,
		&r.venue,
		&r.scenario,
		&r.orderType,
		&r.batchSize,
		&r.ok,
		&r.expectedEntrySide,
		&r.expectedEntryExpectedPrice,
		&r.expectedEntryBookSufficient,
		&r.expectedEntryTopSufficient,
		&r.expectedEntryJSON,
		&r.expectedExitSide,
		&r.expectedExitExpectedPrice,
		&r.expectedExitBookSufficient,
		&r.expectedExitTopSufficient,
		&r.expectedExitJSON,
		&r.cost.EntryOrderID,
		&r.cost.ExitOrderID,
		&r.cost.EntryQty,
		&r.cost.ExitQty,
		&r.cost.EntryValueUSD,
		&r.cost.ExitValueUSD,
		&r.cost.EntryFeeUSD,
		&r.cost.ExitFeeUSD,
		&r.cost.PriceMoveCostUSD,
		&r.cost.TradeCostUSD,
		&r.cost.BalanceBeforeUSD,
		&r.cost.BalanceAfterUSD,
		&r.cost.BalanceDeltaUSD,
		&r.cost.ReconciliationDiffUSD,
		&r.clean,
		&r.cost.QualityReason,
		&r.cost.RunID,
	}
}

func (r dashboardTakerCostRow) toDashboardTakerCostSample() (DashboardTakerCostSample, error) {
	completedAt, err := time.Parse(time.RFC3339Nano, r.completedAt)
	if err != nil {
		return DashboardTakerCostSample{}, err
	}
	r.cost.CompletedAt = completedAt
	r.cost.Venue = r.venue
	r.cost.Clean = r.clean == 1
	return DashboardTakerCostSample{
		PlotAt:      firstNonZeroTime(parseOptionalTime(r.scheduledAt), parseOptionalTime(r.sentAt), completedAt),
		CompletedAt: completedAt,
		Venue:       r.venue,
		Scenario:    r.scenario,
		OrderType:   r.orderType,
		BatchSize:   r.batchSize,
		OK:          r.ok == 1,
		Cost:        r.cost,
		ExpectedEntryFill: dashboardExpectedFill(
			r.expectedEntrySide,
			r.expectedEntryExpectedPrice,
			r.expectedEntryBookSufficient,
			r.expectedEntryTopSufficient,
			r.expectedEntryJSON,
		),
		ExpectedExitFill: dashboardExpectedFill(
			r.expectedExitSide,
			r.expectedExitExpectedPrice,
			r.expectedExitBookSufficient,
			r.expectedExitTopSufficient,
			r.expectedExitJSON,
		),
	}, nil
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

func latencyConfirmNS(venue string, orderType string, networkNS int64, rawNetworkNS int64, adjustedNetworkNS int64, speedBumpNS int64) int64 {
	if adjustedNetworkNS > 0 && speedBumpNS == effectiveDashboardSpeedBumpNS(venue, orderType, speedBumpNS) {
		return adjustedNetworkNS
	}
	raw := rawNetworkNS
	if raw <= 0 {
		raw = networkNS
	}
	adjusted := raw - effectiveDashboardSpeedBumpNS(venue, orderType, speedBumpNS)
	if adjusted < 0 {
		return 0
	}
	return adjusted
}

func accountFeedCancelCleanupNS(ok bool, durationNS int64, description string, confirmation string) int64 {
	if !isAccountFeedCancelCleanup(ok, description, confirmation) || durationNS <= 0 {
		return 0
	}
	return durationNS
}

func isAccountFeedCancelCleanup(ok bool, description string, confirmation string) bool {
	return ok && strings.Contains(strings.ToLower(description), "cancel") && confirmation == "account_feed"
}

func effectiveDashboardSpeedBumpNS(venue string, orderType string, speedBumpNS int64) int64 {
	if speedBumpNS > 0 && !isExtendedDashboardNonTaker(venue, orderType) {
		return speedBumpNS
	}
	if isExtendedDashboardTaker(venue, orderType) {
		return extendedVenueSpeedBumpNS
	}
	return 0
}

func isExtendedDashboardTaker(venue string, orderType string) bool {
	return strings.EqualFold(venue, "extended") && isDashboardTakerOrderType(orderType)
}

func isExtendedDashboardNonTaker(venue string, orderType string) bool {
	return strings.EqualFold(venue, "extended") && !isDashboardTakerOrderType(orderType)
}

func isDashboardTakerOrderType(orderType string) bool {
	normalized := strings.ToLower(strings.TrimSpace(orderType))
	return normalized == "market" || normalized == "ioc"
}
