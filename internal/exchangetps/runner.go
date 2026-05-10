package exchangetps

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Collector interface {
	Run(context.Context, *Store) error
}

type RunnerConfig struct {
	Venue            string
	StorePath        string
	MinuteRetention  time.Duration
	AsterWSURL       string
	HyperliquidWSURL string
	LighterURL       string
	FlushInterval    time.Duration
	PollInterval     time.Duration
}

func RunCollector(ctx context.Context, cfg RunnerConfig) error {
	venue := NormalizeVenue(cfg.Venue)
	if venue == "" {
		return errors.New("venue is required")
	}

	store, err := OpenStore(cfg.StorePath)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.ApplyRetention(ctx, cfg.MinuteRetention); err != nil {
		return err
	}

	collector, err := NewCollector(venue, cfg)
	if err != nil {
		return err
	}
	if err := collector.Run(ctx, store); err != nil {
		return err
	}
	return store.ApplyRetention(context.Background(), cfg.MinuteRetention)
}

func NewCollector(venue string, cfg RunnerConfig) (Collector, error) {
	switch NormalizeVenue(venue) {
	case "aster":
		return &AsterCollector{
			WSURL:         cfg.AsterWSURL,
			Aggregator:    NewAggregator("aster", time.Minute),
			FlushInterval: cfg.FlushInterval,
		}, nil
	case "lighter":
		return &LighterCollector{
			MetricsURL:   cfg.LighterURL,
			PollInterval: cfg.PollInterval,
		}, nil
	case "hyperliquid":
		return &HyperliquidCollector{
			WSURL:         cfg.HyperliquidWSURL,
			FlushInterval: cfg.FlushInterval,
			Aggregator:    NewAggregator("hyperliquid", time.Minute),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported exchange TPS venue %q", venue)
	}
}

func NormalizeVenue(venue string) string {
	return strings.ToLower(strings.TrimSpace(venue))
}
