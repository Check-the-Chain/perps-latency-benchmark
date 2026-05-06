package app

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"perps-latency-benchmark/internal/venues/spec"
)

type rateLimitState struct {
	reserved            int
	reserveBlockedUntil time.Time
}

func (s *rateLimitState) preflight(ctx context.Context, venueName string, cfg fileConfig) error {
	if !cfg.RateLimit.Enabled {
		return nil
	}
	runtime, ok := resolveVenueRuntime(venueName, cfg)
	if !ok || len(runtime.Definition.RateLimitCommand.Command) == 0 {
		return nil
	}
	status, err := rateLimitCommand(ctx, runtime.Definition.RateLimitCommand, cfg, "status")
	if err != nil {
		return err
	}
	remaining := rateLimitRemaining(status)
	minRemaining := cfg.RateLimit.MinRemaining
	if minRemaining <= 0 {
		minRemaining = 100
	}
	if remaining >= minRemaining {
		s.reserveBlockedUntil = time.Time{}
		return nil
	}
	if cfg.Benchmark.RatePerSecond > 0 && cfg.Benchmark.RatePerSecond <= 0.1 {
		return nil
	}
	if !cfg.RateLimit.ReserveWhenBelow {
		return fmt.Errorf("%s request capacity below minimum: remaining=%d min=%d used=%d cap=%d surplus=%d", venueName, remaining, minRemaining, status.RequestsUsed, status.RequestsCap, status.RequestsSurplus)
	}
	if time.Now().Before(s.reserveBlockedUntil) {
		return fmt.Errorf("%s request capacity below minimum and reserve is backing off until %s: remaining=%d min=%d", venueName, s.reserveBlockedUntil.UTC().Format(time.RFC3339), remaining, minRemaining)
	}

	target := cfg.RateLimit.ReserveTarget
	if target < minRemaining {
		target = minRemaining
	}
	weight := target - remaining
	if weight <= 0 {
		return nil
	}
	maxWeight := cfg.RateLimit.MaxReserveWeight
	if maxWeight <= 0 {
		return fmt.Errorf("%s auto reserve is enabled but rate_limit.max_reserve_weight is not positive", venueName)
	}
	if s.reserved+weight > maxWeight {
		return fmt.Errorf("%s auto reserve would exceed max_reserve_weight: requested=%d already_reserved=%d max=%d", venueName, weight, s.reserved, maxWeight)
	}
	if _, err := rateLimitCommand(ctx, runtime.Definition.RateLimitCommand, cfg, "reserve", strconv.Itoa(weight)); err != nil {
		s.reserveBlockedUntil = time.Now().Add(time.Hour)
		return err
	}
	s.reserveBlockedUntil = time.Time{}
	s.reserved += weight
	status, err = rateLimitCommand(ctx, runtime.Definition.RateLimitCommand, cfg, "status")
	if err != nil {
		return err
	}
	remaining = rateLimitRemaining(status)
	if remaining < minRemaining {
		return fmt.Errorf("%s request capacity still below minimum after reserve: remaining=%d min=%d", venueName, remaining, minRemaining)
	}
	return nil
}

func rateLimitRemaining(status spec.RateLimitStatus) int {
	return status.RequestsCap + status.RequestsSurplus - status.RequestsUsed
}

func rateLimitCommand(ctx context.Context, command spec.RateLimitCommand, cfg fileConfig, args ...string) (spec.RateLimitStatus, error) {
	timeout := durationMS(cfg.RateLimit.TimeoutMS)
	if timeout <= 0 {
		timeout = command.Timeout
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	commandArgs := append([]string{}, command.Command...)
	commandArgs = append(commandArgs, args...)
	cmd := exec.CommandContext(cmdCtx, commandArgs[0], commandArgs[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return spec.RateLimitStatus{}, fmt.Errorf("rate limit %s: %w: %s", args[0], err, stderr.String())
		}
		return spec.RateLimitStatus{}, fmt.Errorf("rate limit %s: %w", args[0], err)
	}
	if command.Decode == nil {
		return spec.RateLimitStatus{}, fmt.Errorf("rate limit command has no decoder")
	}
	status, err := command.Decode(stdout.Bytes())
	if err != nil {
		return spec.RateLimitStatus{}, fmt.Errorf("decode rate limit response: %w", err)
	}
	return status, nil
}
