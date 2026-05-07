package bench

import "testing"

func TestLatencyAccumulatorClampsValuesAndReportsEmpty(t *testing.T) {
	accumulator := newLatencyAccumulator()
	if accumulator.recorded() {
		t.Fatal("new accumulator should be empty")
	}

	accumulator.record(0)
	accumulator.record(2_000_000)

	if !accumulator.recorded() {
		t.Fatal("expected recorded accumulator")
	}
	if accumulator.meanMS() <= 0 {
		t.Fatalf("mean = %f", accumulator.meanMS())
	}
	if accumulator.p95MS() < 1.9 || accumulator.p95MS() > 2.1 {
		t.Fatalf("p95 = %f", accumulator.p95MS())
	}
}
