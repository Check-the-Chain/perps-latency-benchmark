package bench

import hdrhistogram "github.com/HdrHistogram/hdrhistogram-go"

const maxLatencyNS = 10 * 60 * 1_000_000_000

type latencyAccumulator struct {
	hist  *hdrhistogram.Histogram
	total int64
	count int
}

func newLatencyAccumulator() latencyAccumulator {
	return latencyAccumulator{hist: hdrhistogram.New(1, maxLatencyNS, 3)}
}

func (a *latencyAccumulator) record(value int64) {
	if value < 1 {
		value = 1
	}
	_ = a.hist.RecordValue(value)
	a.total += value
	a.count++
}

func (a latencyAccumulator) recorded() bool {
	return a.count > 0
}

func (a latencyAccumulator) meanMS() float64 {
	if a.count == 0 {
		return 0
	}
	return nsToMS(a.total / int64(a.count))
}

func (a latencyAccumulator) minMS() float64 {
	if a.count == 0 {
		return 0
	}
	return nsToMS(a.hist.Min())
}

func (a latencyAccumulator) p50MS() float64 {
	return a.quantileMS(50)
}

func (a latencyAccumulator) p95MS() float64 {
	return a.quantileMS(95)
}

func (a latencyAccumulator) p99MS() float64 {
	return a.quantileMS(99)
}

func (a latencyAccumulator) maxMS() float64 {
	if a.count == 0 {
		return 0
	}
	return nsToMS(a.hist.Max())
}

func (a latencyAccumulator) quantileMS(quantile float64) float64 {
	if a.count == 0 {
		return 0
	}
	return nsToMS(a.hist.ValueAtQuantile(quantile))
}
