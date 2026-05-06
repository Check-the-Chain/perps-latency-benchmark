package bench

import (
	"math"
	"testing"

	"perps-latency-benchmark/internal/booktop"
)

func TestExpectedFillFromDepthUsesWeightedAverage(t *testing.T) {
	fill := expectedFillFromDepth([]booktop.Level{
		{Price: 100, Size: 1},
		{Price: 101, Size: 2},
		{Price: 102, Size: 10},
	}, 2)

	if !fill.Sufficient {
		t.Fatalf("expected sufficient fill: %+v", fill)
	}
	if fill.LevelsUsed != 2 || !fill.Weighted {
		t.Fatalf("expected two weighted levels: %+v", fill)
	}
	if fill.Price != 100.5 {
		t.Fatalf("price = %v", fill.Price)
	}
}

func TestExpectedFillFromDepthMarksInsufficientVisibleDepth(t *testing.T) {
	fill := expectedFillFromDepth([]booktop.Level{
		{Price: 100, Size: 1},
		{Price: 101, Size: 0.5},
	}, 2)

	if fill.Sufficient {
		t.Fatalf("expected insufficient fill: %+v", fill)
	}
	if fill.Available != 1.5 {
		t.Fatalf("available = %v", fill.Available)
	}
	if math.Abs(fill.Price-(100+(1.0/3.0))) > 1e-12 {
		t.Fatalf("price = %v", fill.Price)
	}
}
