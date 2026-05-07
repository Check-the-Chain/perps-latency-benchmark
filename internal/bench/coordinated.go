package bench

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"perps-latency-benchmark/internal/booktop"
	"perps-latency-benchmark/internal/lifecycle"
	"perps-latency-benchmark/internal/netlatency"
)

type CoordinatedItem struct {
	Config          Config
	Client          *netlatency.Client
	Venue           Venue
	Cleanup         CleanupAdapter
	NetworkBaseline NetworkBaselineObserver
	SpinLead        time.Duration
	LockThreads     bool
	Book            *booktop.Tracker
	OrderSide       string
	OrderSize       float64
}

type CoordinatedCycle struct {
	Index       int
	Iteration   int
	Warmup      bool
	ScheduledAt time.Time
	SpinLead    time.Duration
	LockThreads bool
}

type preparedCycleItem struct {
	item       CoordinatedItem
	index      int
	prepared   PreparedRequest
	preparedNS int64
	err        error
}

type cleanupCycleItem struct {
	index    int
	item     CoordinatedItem
	sample   Sample
	prepared PreparedCleanup
	err      error
}

type cleanupCycleResult struct {
	index   int
	cleanup *CleanupResult
}

type sampleCycleResult struct {
	index  int
	sample Sample
}

func RunCoordinatedCycle(ctx context.Context, items []CoordinatedItem, cycle CoordinatedCycle) []Sample {
	if len(items) == 0 {
		return nil
	}

	prepared := prepareCoordinated(ctx, items, cycle)
	samples := submitPreparedCoordinated(ctx, prepared, cycle)
	applyCoordinatedCleanup(ctx, items, samples)
	sort.Slice(samples, func(i, j int) bool {
		if samples[i].Venue == samples[j].Venue {
			return samples[i].Index < samples[j].Index
		}
		return samples[i].Venue < samples[j].Venue
	})
	return samples
}

func BeforeCoordinatedRun(ctx context.Context, items []CoordinatedItem, runID string) map[string]*CleanupResult {
	results := make(map[string]*CleanupResult, len(items))
	for _, item := range items {
		cfg := item.Config.Normalized()
		cfg.RunID = runID
		cleanup := newBenchmarkLifecycle(cfg, item.Venue, item.Cleanup).beforeRun(ctx)
		if cleanup != nil {
			results[item.Venue.Name()] = cleanup
		}
	}
	return results
}

func AfterCoordinatedRun(ctx context.Context, items []CoordinatedItem, runID string, samples []Sample) map[string]*CleanupResult {
	results := make(map[string]*CleanupResult, len(items))
	byVenue := make(map[string][]Sample)
	for _, sample := range samples {
		byVenue[sample.Venue] = append(byVenue[sample.Venue], sample)
	}
	for _, item := range items {
		cfg := item.Config.Normalized()
		cfg.RunID = runID
		lifecycle := newBenchmarkLifecycle(cfg, item.Venue, item.Cleanup)
		result := lifecycle.result(byVenue[item.Venue.Name()])
		cleanup := lifecycle.afterRun(ctx, result)
		if cleanup != nil {
			results[item.Venue.Name()] = cleanup
		}
	}
	return results
}

func CloseCoordinated(ctx context.Context, items []CoordinatedItem) {
	for _, item := range items {
		if item.Cleanup != nil {
			_ = item.Cleanup.Close(ctx)
		}
		if item.Venue != nil {
			_ = item.Venue.Close(ctx)
		}
		if item.Client != nil {
			item.Client.CloseIdleConnections()
		}
	}
}

func prepareCoordinated(ctx context.Context, items []CoordinatedItem, cycle CoordinatedCycle) []preparedCycleItem {
	out := make([]preparedCycleItem, len(items))
	var wg sync.WaitGroup
	for index, item := range items {
		out[index] = preparedCycleItem{item: item, index: index}
	}
	for _, index := range randomizedOrder(len(items)) {
		item := items[index]
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg := item.Config.Normalized()
			batchSize := 1
			if cfg.Scenario == ScenarioBatch {
				batchSize = cfg.BatchSize
			}
			start := time.Now()
			prepared, err := item.Venue.Prepare(ctx, cfg.Scenario, cycle.Iteration, batchSize)
			if err == nil {
				warmPreparedHost(ctx, item.Client, prepared)
			}
			out[index].prepared = prepared
			out[index].preparedNS = time.Since(start).Nanoseconds()
			out[index].err = err
		}()
	}
	wg.Wait()
	return out
}

func warmPreparedHost(ctx context.Context, client *netlatency.Client, prepared PreparedRequest) {
	if client == nil {
		return
	}
	for _, rawURL := range preparedURLs(prepared) {
		if rawURL == "" {
			continue
		}
		_ = client.WarmHost(ctx, rawURL)
		return
	}
}

func preparedURLs(prepared PreparedRequest) []string {
	urls := []string{prepared.Request.URL}
	for _, request := range prepared.ParallelRequests {
		urls = append(urls, request.URL)
	}
	return urls
}

func submitPreparedCoordinated(ctx context.Context, prepared []preparedCycleItem, cycle CoordinatedCycle) []Sample {
	samples := make([]Sample, len(prepared))
	ready := sync.WaitGroup{}
	ready.Add(len(prepared))
	start := make(chan struct{})
	results := make(chan sampleCycleResult, len(prepared))
	timing := coordinatedTiming{
		Target:      cycle.ScheduledAt,
		SpinLead:    cycle.SpinLead,
		LockThreads: cycle.LockThreads,
	}

	for _, index := range randomizedOrder(len(prepared)) {
		preparedItem := prepared[index]
		go func() {
			timing.run(ctx, start, &ready, func() {
				results <- sampleCycleResult{
					index:  preparedItem.index,
					sample: runPreparedCycleItem(ctx, preparedItem, cycle),
				}
			})
		}()
	}
	timing.release(ctx, &ready, start)

	for range prepared {
		result := <-results
		samples[result.index] = result.sample
	}
	return samples
}

func runPreparedCycleItem(ctx context.Context, preparedItem preparedCycleItem, cycle CoordinatedCycle) Sample {
	item := preparedItem.item
	cfg := item.Config.Normalized()
	batchSize := 1
	if cfg.Scenario == ScenarioBatch {
		batchSize = cfg.BatchSize
	}
	if preparedItem.err != nil {
		return Sample{
			Venue:       item.Venue.Name(),
			RunID:       cfg.RunID,
			Scenario:    cfg.Scenario,
			Index:       cycle.Index,
			Iteration:   cycle.Iteration,
			Warmup:      cycle.Warmup,
			BatchSize:   batchSize,
			ScheduledAt: cycle.ScheduledAt.UTC(),
			PreparedNS:  preparedItem.preparedNS,
			OK:          false,
			Error:       fmt.Sprintf("prepare: %v", preparedItem.err),
			Classification: lifecycle.Classification{
				Status: lifecycle.StatusTransportError,
				Reason: preparedItem.err.Error(),
			},
			CompletedAt: time.Now().UTC(),
		}
	}
	lifecycle := sampleLifecycle{
		client:          item.Client,
		venue:           item.Venue,
		cleanup:         item.Cleanup,
		networkBaseline: item.NetworkBaseline,
	}
	sample := lifecycle.runPrepared(ctx, cfg, cycle.Index, cycle.Iteration, cycle.Warmup, cycle.ScheduledAt, preparedItem.prepared, preparedItem.preparedNS)
	addExpectedFill(&sample, item, "entry", sample.Trace.StartedAt.Add(time.Duration(sample.Trace.WroteRequestAtNS)))
	return sample
}

func applyCoordinatedCleanup(ctx context.Context, items []CoordinatedItem, samples []Sample) {
	cleanupItems := make([]cleanupCycleItem, 0, len(samples))
	itemByVenue := make(map[string]CoordinatedItem, len(items))
	for _, item := range items {
		itemByVenue[item.Venue.Name()] = item
	}
	for index, sample := range samples {
		item, ok := itemByVenue[sample.Venue]
		if !ok || item.Cleanup == nil {
			continue
		}
		cfg := item.Config.Normalized()
		if !cfg.Cleanup.Enabled || cfg.Cleanup.Scope != CleanupScopeAfterSample {
			continue
		}
		if !shouldCleanupAfterSample(sample) {
			continue
		}
		addExpectedFill(&sample, item, "exit", time.Now())
		samples[index] = sample
		cleanupItem := cleanupCycleItem{
			index:  index,
			item:   item,
			sample: sample,
		}
		start := time.Now()
		if preparer, ok := item.Cleanup.(PreparedCleanupAdapter); ok {
			prepared, err := preparer.PrepareAfterSample(ctx, sample)
			cleanupItem.prepared = prepared
			cleanupItem.err = err
		} else {
			cleanupItem.prepared = PreparedCleanup{
				PreparedNS: time.Since(start).Nanoseconds(),
				Execute: func(execCtx context.Context) CleanupResult {
					return item.Cleanup.AfterSample(execCtx, sample)
				},
			}
		}
		cleanupItems = append(cleanupItems, cleanupItem)
	}
	if len(cleanupItems) == 0 {
		return
	}

	ready := sync.WaitGroup{}
	ready.Add(len(cleanupItems))
	start := make(chan struct{})
	results := make(chan cleanupCycleResult, len(cleanupItems))
	spinLead, lockThreads := cleanupCoordinationSettings(cleanupItems)
	target := time.Now().Add(maxDuration(2*time.Millisecond, spinLead))
	timing := coordinatedTiming{
		Target:      target,
		SpinLead:    spinLead,
		LockThreads: lockThreads,
	}
	for _, index := range randomizedOrder(len(cleanupItems)) {
		cleanupItem := cleanupItems[index]
		go func() {
			timing.run(ctx, start, &ready, func() {
				cleanup := executePreparedCleanup(ctx, cleanupItem, target)
				results <- cleanupCycleResult{index: cleanupItem.index, cleanup: &cleanup}
			})
		}()
	}
	timing.release(ctx, &ready, start)

	for range cleanupItems {
		result := <-results
		if result.cleanup == nil {
			continue
		}
		samples[result.index].Cleanup = result.cleanup
		attachCleanupMetadata(&samples[result.index], result.cleanup.Metadata)
		cfg := itemByVenue[samples[result.index].Venue].Config.Normalized()
		if cfg.Cleanup.Mode == CleanupModeStrict && !result.cleanup.OK {
			samples[result.index].OK = false
			samples[result.index].Error = appendError(samples[result.index].Error, "cleanup: "+result.cleanup.Error)
		}
	}
}

func attachCleanupMetadata(sample *Sample, metadata map[string]any) {
	if len(metadata) == 0 {
		return
	}
	sample.CloseoutOrderRefs = CleanupOrderRefContract("").FromMetadata(metadata)
}

func addExpectedFill(sample *Sample, item CoordinatedItem, phase string, at time.Time) {
	if sample == nil || item.Book == nil || item.OrderSize <= 0 {
		return
	}
	if at.IsZero() {
		at = time.Now()
	}
	snapshot, ok := item.Book.SnapshotAt(at)
	if !ok {
		return
	}
	side := item.OrderSide
	if phase == "exit" {
		side = oppositeSide(side)
	}
	price := snapshot.Ask
	available := snapshot.AskSize
	levels := snapshot.Asks
	if strings.EqualFold(side, "sell") {
		price = snapshot.Bid
		available = snapshot.BidSize
		levels = snapshot.Bids
	}
	depth := expectedFillFromDepth(levels, item.OrderSize)
	if depth.Price > 0 {
		price = depth.Price
	} else {
		depth.Available = available
		depth.Sufficient = available <= 0 || available >= item.OrderSize
		if available > 0 {
			depth.LevelsUsed = 1
		}
	}
	fill := &ExpectedFill{
		Phase:          phase,
		Side:           side,
		Size:           item.OrderSize,
		ExpectedPrice:  price,
		TopBid:         snapshot.Bid,
		TopAsk:         snapshot.Ask,
		TopBidSize:     snapshot.BidSize,
		TopAskSize:     snapshot.AskSize,
		TopAvailable:   available,
		TopSufficient:  available <= 0 || available >= item.OrderSize,
		BookAvailable:  depth.Available,
		BookSufficient: boolPtr(depth.Sufficient),
		BookLevels:     depth.LevelsUsed,
		DepthWeighted:  depth.Weighted,
		BookAgeNS:      snapshot.Age(at).Nanoseconds(),
		BookReceivedAt: snapshot.ReceivedAt.UTC(),
	}
	if !snapshot.ExchangeAt.IsZero() {
		exchangeAt := snapshot.ExchangeAt.UTC()
		fill.BookExchangeAt = &exchangeAt
	}
	if phase == "exit" {
		sample.ExpectedExitFill = fill
		return
	}
	sample.ExpectedEntryFill = fill
}

func boolPtr(value bool) *bool {
	return &value
}

type expectedFillDepth struct {
	Price      float64
	Available  float64
	Sufficient bool
	LevelsUsed int
	Weighted   bool
}

func expectedFillFromDepth(levels []booktop.Level, size float64) expectedFillDepth {
	if size <= 0 || len(levels) == 0 {
		return expectedFillDepth{}
	}
	var out expectedFillDepth
	var filled float64
	var notional float64
	remaining := size
	for _, level := range levels {
		if level.Price <= 0 || level.Size <= 0 {
			continue
		}
		out.Available += level.Size
		if remaining <= 0 {
			continue
		}
		take := level.Size
		if take > remaining {
			take = remaining
		}
		filled += take
		notional += take * level.Price
		remaining -= take
		out.LevelsUsed++
	}
	if filled <= 0 {
		return out
	}
	out.Price = notional / filled
	out.Sufficient = filled >= size || remaining <= 1e-12
	out.Weighted = out.Sufficient && out.LevelsUsed > 1
	return out
}

func oppositeSide(side string) string {
	if strings.EqualFold(side, "buy") {
		return "sell"
	}
	return "buy"
}

func executePreparedCleanup(ctx context.Context, item cleanupCycleItem, scheduledAt time.Time) CleanupResult {
	if item.err != nil {
		cleanup := cleanupError(item.err, item.prepared.PreparedNS)
		cleanup.ScheduledAt = scheduledAt.UTC()
		return cleanup
	}
	if item.prepared.Result != nil {
		cleanup := *item.prepared.Result
		cleanup.PreparedNS = item.prepared.PreparedNS
		cleanup.ScheduledAt = scheduledAt.UTC()
		return cleanup
	}
	if item.prepared.Execute == nil {
		return CleanupResult{Attempted: false, OK: true, PreparedNS: item.prepared.PreparedNS, ScheduledAt: scheduledAt.UTC()}
	}
	cleanup := item.prepared.Execute(ctx)
	cleanup.PreparedNS = item.prepared.PreparedNS
	cleanup.ScheduledAt = scheduledAt.UTC()
	cleanup.SentAt = cleanup.Trace.StartedAt.UTC()
	cleanup.StartDelayNS = startDelayNS(scheduledAt, cleanup.Trace.StartedAt)
	cleanup.WriteDelayNS = writeDelayNS(scheduledAt, cleanup.Trace)
	return cleanup
}

func cleanupError(err error, preparedNS int64) CleanupResult {
	return CleanupResult{
		Attempted:  true,
		OK:         false,
		Error:      err.Error(),
		PreparedNS: preparedNS,
	}
}

func cleanupCoordinationSettings(items []cleanupCycleItem) (time.Duration, bool) {
	var spinLead time.Duration
	var lockThreads bool
	for _, item := range items {
		if item.item.SpinLead > spinLead {
			spinLead = item.item.SpinLead
		}
		if item.item.LockThreads {
			lockThreads = true
		}
	}
	return spinLead, lockThreads
}
