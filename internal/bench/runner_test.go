package bench_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/netlatency"
	"perps-latency-benchmark/internal/venues/mock"
)

func TestRunnerExcludesWarmups(t *testing.T) {
	result, err := bench.Runner{
		Config: bench.Config{Scenario: bench.ScenarioSingle, Iterations: 2, Warmups: 1},
		Client: NewTestClient(),
		Venue:  mock.New(mock.Config{}),
	}.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Samples) != 3 {
		t.Fatalf("samples = %d", len(result.Samples))
	}
	if result.RunID == "" || result.Samples[0].RunID != result.RunID {
		t.Fatalf("run ids: result=%q sample=%q", result.RunID, result.Samples[0].RunID)
	}
	if len(result.MeasuredSamples()) != 2 {
		t.Fatalf("measured samples = %d", len(result.MeasuredSamples()))
	}

	summary := bench.Summarize(result.Samples)
	if summary.Count != 2 || summary.OK != 2 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestRunnerBatchSize(t *testing.T) {
	result, err := bench.Runner{
		Config: bench.Config{Scenario: bench.ScenarioBatch, Iterations: 1, BatchSize: 4},
		Client: NewTestClient(),
		Venue:  mock.New(mock.Config{}),
	}.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Samples[0].BatchSize != 4 {
		t.Fatalf("batch size = %d", result.Samples[0].BatchSize)
	}
}

func TestRunnerUsesPreparedClassifier(t *testing.T) {
	result, err := bench.Runner{
		Config: bench.Config{Scenario: bench.ScenarioSingle, Iterations: 1},
		Client: NewTestClient(),
		Venue:  classifierVenue{Venue: mock.New(mock.Config{})},
	}.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sample := result.Samples[0]
	if sample.OK {
		t.Fatalf("sample unexpectedly ok: %+v", sample)
	}
	if sample.Classification.Status != lifecycle.StatusRejected {
		t.Fatalf("classification = %+v", sample.Classification)
	}
}

func TestRunnerRunsCleanupOutsideMeasuredRequest(t *testing.T) {
	cleanup := &fakeCleanup{result: bench.CleanupResult{Attempted: true, OK: true, Description: "cancel"}}
	result, err := bench.Runner{
		Config: bench.Config{
			Scenario:   bench.ScenarioSingle,
			Iterations: 1,
			Cleanup:    bench.CleanupConfig{Enabled: true, Mode: bench.CleanupModeBestEffort, Scope: bench.CleanupScopeAfterSample},
		},
		Client:  NewTestClient(),
		Venue:   mock.New(mock.Config{}),
		Cleanup: cleanup,
	}.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cleanup.calls != 1 {
		t.Fatalf("cleanup calls = %d", cleanup.calls)
	}
	if result.Samples[0].Cleanup == nil || !result.Samples[0].Cleanup.OK {
		t.Fatalf("cleanup = %#v", result.Samples[0].Cleanup)
	}
	summary := bench.Summarize(result.Samples)
	if summary.Cleanup.Attempted != 1 || summary.Cleanup.OK != 1 {
		t.Fatalf("summary cleanup = %+v", summary.Cleanup)
	}
}

func TestRunnerStrictCleanupFailureFailsSample(t *testing.T) {
	result, err := bench.Runner{
		Config: bench.Config{
			Scenario:   bench.ScenarioSingle,
			Iterations: 1,
			Cleanup:    bench.CleanupConfig{Enabled: true, Mode: bench.CleanupModeStrict, Scope: bench.CleanupScopeAfterSample},
		},
		Client:  NewTestClient(),
		Venue:   mock.New(mock.Config{}),
		Cleanup: &fakeCleanup{result: bench.CleanupResult{Attempted: true, OK: false, Error: "cancel failed"}},
	}.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sample := result.Samples[0]
	if sample.OK {
		t.Fatalf("sample unexpectedly ok: %+v", sample)
	}
	if sample.Cleanup == nil || sample.Cleanup.Error != "cancel failed" {
		t.Fatalf("cleanup = %#v", sample.Cleanup)
	}
	if !strings.Contains(sample.Error, "cleanup: cancel failed") {
		t.Fatalf("error = %q", sample.Error)
	}
}

func TestRunnerStrictStartupCleanupFailureSkipsSamples(t *testing.T) {
	result, err := bench.Runner{
		Config: bench.Config{
			Scenario:   bench.ScenarioSingle,
			Iterations: 1,
			Cleanup:    bench.CleanupConfig{Enabled: true, Mode: bench.CleanupModeStrict, Scope: bench.CleanupScopeAfterSample},
		},
		Client:  NewTestClient(),
		Venue:   mock.New(mock.Config{}),
		Cleanup: &failingRunCleanup{},
	}.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.StartupCleanup == nil || result.StartupCleanup.OK {
		t.Fatalf("startup cleanup = %#v", result.StartupCleanup)
	}
	if len(result.Samples) != 0 {
		t.Fatalf("samples = %d, want 0", len(result.Samples))
	}
}

func TestRunnerCleansRejectedSampleWithOrderRefs(t *testing.T) {
	cleanup := &fakeCleanup{result: bench.CleanupResult{Attempted: true, OK: true, Description: "neutralize"}}
	result, err := bench.Runner{
		Config: bench.Config{
			Scenario:   bench.ScenarioSingle,
			Iterations: 1,
			Cleanup:    bench.CleanupConfig{Enabled: true, Mode: bench.CleanupModeStrict, Scope: bench.CleanupScopeAfterSample},
		},
		Client:  NewTestClient(),
		Venue:   rejectedWithOrderRefsVenue{Venue: mock.New(mock.Config{})},
		Cleanup: cleanup,
	}.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cleanup.calls != 1 {
		t.Fatalf("cleanup calls = %d", cleanup.calls)
	}
	sample := result.Samples[0]
	if sample.OK {
		t.Fatalf("sample unexpectedly ok: %+v", sample)
	}
	if sample.Cleanup == nil || !sample.Cleanup.OK {
		t.Fatalf("cleanup = %#v", sample.Cleanup)
	}
}

func TestRunnerRecordsRunCleanupHooks(t *testing.T) {
	result, err := bench.Runner{
		Config: bench.Config{
			Scenario:   bench.ScenarioSingle,
			Iterations: 1,
			Cleanup:    bench.CleanupConfig{Enabled: true, Mode: bench.CleanupModeBestEffort, Scope: bench.CleanupScopeAfterSample},
		},
		Client:  NewTestClient(),
		Venue:   mock.New(mock.Config{}),
		Cleanup: &fakeRunCleanup{},
	}.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.StartupCleanup == nil || !result.StartupCleanup.OK {
		t.Fatalf("startup cleanup = %#v", result.StartupCleanup)
	}
	if result.Reconciliation == nil || !result.Reconciliation.OK {
		t.Fatalf("reconciliation = %#v", result.Reconciliation)
	}
}

func TestRunnerOpenLoopStopsSchedulingWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := bench.Runner{
		Config: bench.Config{
			Scenario:      bench.ScenarioSingle,
			Iterations:    100,
			RatePerSecond: 100,
			MaxInFlight:   4,
		},
		Client: NewTestClient(),
		Venue:  mock.New(mock.Config{}),
	}.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Samples) != 0 {
		t.Fatalf("samples = %d, want 0", len(result.Samples))
	}
}

func TestRunnerStartsParallelConfirmationBeforeHTTPRequests(t *testing.T) {
	confirmationStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case <-confirmationStarted:
		case <-time.After(time.Second):
			t.Error("HTTP request started before parallel confirmation wait")
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	result, err := bench.Runner{
		Config: bench.Config{
			Scenario:        bench.ScenarioBatch,
			Iterations:      1,
			BatchSize:       2,
			MeasurementMode: bench.MeasurementModeWSConfirmation,
		},
		Client: NewTestClient(),
		Venue: parallelConfirmVenue{
			url:                 server.URL,
			confirmationStarted: confirmationStarted,
		},
	}.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Samples[0].OK {
		t.Fatalf("sample = %+v", result.Samples[0])
	}
	if result.Samples[0].Trace.Transport != "websocket" {
		t.Fatalf("trace transport = %q", result.Samples[0].Trace.Transport)
	}
}

type fakeCleanup struct {
	result bench.CleanupResult
	calls  int
}

func (c *fakeCleanup) AfterSample(context.Context, bench.Sample) bench.CleanupResult {
	c.calls++
	return c.result
}

func (c *fakeCleanup) Close(context.Context) error {
	return nil
}

type fakeRunCleanup struct {
	fakeCleanup
}

func (c *fakeRunCleanup) BeforeRun(context.Context, bench.CleanupRun) bench.CleanupResult {
	return bench.CleanupResult{Attempted: false, OK: true, Description: "startup"}
}

func (c *fakeRunCleanup) AfterRun(context.Context, bench.Result) bench.CleanupResult {
	return bench.CleanupResult{Attempted: true, OK: true, Description: "reconcile"}
}

type failingRunCleanup struct {
	fakeCleanup
}

func (c *failingRunCleanup) BeforeRun(context.Context, bench.CleanupRun) bench.CleanupResult {
	return bench.CleanupResult{Attempted: true, OK: false, Error: "position open"}
}

func (c *failingRunCleanup) AfterRun(context.Context, bench.Result) bench.CleanupResult {
	return bench.CleanupResult{Attempted: false, OK: true}
}

type classifierVenue struct {
	*mock.Venue
}

func (v classifierVenue) Prepare(ctx context.Context, scenario bench.Scenario, iteration int, batchSize int) (bench.PreparedRequest, error) {
	prepared, err := v.Venue.Prepare(ctx, scenario, iteration, batchSize)
	if err != nil {
		return bench.PreparedRequest{}, err
	}
	prepared.Classifier = func(lifecycle.ResponseInput) lifecycle.Classification {
		return lifecycle.Classification{Status: lifecycle.StatusRejected, Reason: "venue rejected"}
	}
	return prepared, nil
}

type rejectedWithOrderRefsVenue struct {
	*mock.Venue
}

func (v rejectedWithOrderRefsVenue) Prepare(ctx context.Context, scenario bench.Scenario, iteration int, batchSize int) (bench.PreparedRequest, error) {
	prepared, err := v.Venue.Prepare(ctx, scenario, iteration, batchSize)
	if err != nil {
		return bench.PreparedRequest{}, err
	}
	prepared.Metadata = map[string]any{
		"cleanup_orders": []any{map[string]any{"venue": "mock", "client_order_id": "entry-1"}},
	}
	prepared.Classifier = func(lifecycle.ResponseInput) lifecycle.Classification {
		return lifecycle.Classification{Status: lifecycle.StatusRejected, Reason: "venue rejected"}
	}
	return prepared, nil
}

type parallelConfirmVenue struct {
	url                 string
	confirmationStarted chan struct{}
}

func (v parallelConfirmVenue) Name() string {
	return "parallel"
}

func (v parallelConfirmVenue) Prepare(context.Context, bench.Scenario, int, int) (bench.PreparedRequest, error) {
	return bench.PreparedRequest{
		Transport: "https",
		ParallelRequests: []netlatency.RequestTemplate{
			{Method: http.MethodPost, URL: v.url},
			{Method: http.MethodPost, URL: v.url},
		},
		Confirm: &bench.Confirmation{
			Wait: func(_ context.Context, submission netlatency.Result) (netlatency.Result, error) {
				close(v.confirmationStarted)
				return netlatency.Result{
					Trace: netlatency.Trace{
						StartedAt: submission.Trace.StartedAt,
						TotalNS:   int64(time.Millisecond),
						Transport: "websocket",
						TTFBNS:    int64(time.Millisecond),
					},
				}, nil
			},
		},
	}, nil
}

func (v parallelConfirmVenue) Close(context.Context) error {
	return nil
}

func NewTestClient() *netlatency.Client {
	return netlatency.NewClient(netlatency.ClientConfig{})
}
