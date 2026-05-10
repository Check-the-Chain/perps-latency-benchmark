package exchangetps

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"sort"
	"time"
)

const DefaultLighterMetricsURL = "https://mainnet.zklighter.elliot.ai/api/v1/exchangeMetrics"

type LighterCollector struct {
	MetricsURL   string
	HTTPClient   *http.Client
	PollInterval time.Duration
}

type lighterMetric struct {
	Timestamp int64   `json:"timestamp"`
	Data      float64 `json:"data"`
}

func (c *LighterCollector) Run(ctx context.Context, store *Store) error {
	pollInterval := c.PollInterval
	if pollInterval <= 0 {
		pollInterval = time.Minute
	}
	if err := c.setSourceMetadata(ctx, store); err != nil {
		return err
	}
	backoff := time.Second
	if err := c.collectOnce(ctx, store); err != nil {
		log.Printf("lighter tps initial collection failed: %v", err)
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.collectOnce(ctx, store); err != nil {
				log.Printf("lighter tps collection failed: %v", err)
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					return ctx.Err()
				}
				if backoff < time.Minute {
					backoff *= 2
				}
				continue
			}
			backoff = time.Second
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (c *LighterCollector) CollectOnce(ctx context.Context, store *Store) error {
	if err := c.setSourceMetadata(ctx, store); err != nil {
		return err
	}
	return c.collectOnce(ctx, store)
}

func (c *LighterCollector) setSourceMetadata(ctx context.Context, store *Store) error {
	return store.SetSourceMetadata(ctx, SourceMetadata{
		Venue:         "lighter",
		Quality:       SourceQualityProviderReported,
		BucketSeconds: 60,
		Description:   "Lighter exchangeMetrics period=h kind=tps folded into integer minute counts",
	})
}

func (c *LighterCollector) collectOnce(ctx context.Context, store *Store) error {
	metrics, err := c.fetchMetrics(ctx)
	if err != nil {
		return err
	}
	if len(metrics) == 0 {
		return nil
	}
	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].Timestamp < metrics[j].Timestamp
	})

	buckets := make(map[int64]float64)
	spanSeconds := inferredMetricSpanSeconds(metrics)
	for _, metric := range metrics {
		start := time.UnixMilli(metric.Timestamp).UTC()
		if metric.Data <= 0 {
			continue
		}
		bucketStart := floorUnix(start.Unix(), 60)
		buckets[bucketStart] += metric.Data * spanSeconds
	}
	var from, to time.Time
	for bucketStart, txEstimate := range buckets {
		start := time.Unix(bucketStart, 0).UTC()
		txCount := int64(math.Round(txEstimate))
		if txCount < 0 {
			txCount = 0
		}
		if err := store.RecordObservedBucket1m(ctx, BucketDelta{
			Venue:       "lighter",
			BucketStart: start,
			TxCount:     txCount,
		}); err != nil {
			return err
		}
		if from.IsZero() || start.Before(from) {
			from = start
		}
		if to.IsZero() || start.After(to) {
			to = start
		}
	}
	if !from.IsZero() {
		if err := store.RefreshRollups(ctx, "lighter", from, to); err != nil {
			return err
		}
	}
	return nil
}

func inferredMetricSpanSeconds(metrics []lighterMetric) float64 {
	if len(metrics) < 2 {
		return 60
	}
	spans := make([]int64, 0, len(metrics)-1)
	for i := 1; i < len(metrics); i++ {
		diff := metrics[i].Timestamp - metrics[i-1].Timestamp
		if diff > 0 {
			spans = append(spans, diff)
		}
	}
	if len(spans) == 0 {
		return 60
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i] < spans[j] })
	spanMs := spans[len(spans)/2]
	seconds := float64(spanMs) / float64(time.Second/time.Millisecond)
	if seconds < 1 {
		return 1
	}
	if seconds > 60 {
		return 60
	}
	return seconds
}

func (c *LighterCollector) fetchMetrics(ctx context.Context) ([]lighterMetric, error) {
	base := c.MetricsURL
	if base == "" {
		base = DefaultLighterMetricsURL
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("period", "h")
	q.Set("kind", "tps")
	u.RawQuery = q.Encode()

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("lighter exchangeMetrics status %d", resp.StatusCode)
	}
	var payload struct {
		Code    int             `json:"code"`
		Metrics []lighterMetric `json:"metrics"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Code != 200 {
		return nil, fmt.Errorf("lighter exchangeMetrics code %d", payload.Code)
	}
	return payload.Metrics, nil
}
