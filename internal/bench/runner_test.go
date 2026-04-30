package bench_test

import (
	"context"
	"testing"

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

func NewTestClient() *netlatency.Client {
	return netlatency.NewClient(netlatency.ClientConfig{})
}
