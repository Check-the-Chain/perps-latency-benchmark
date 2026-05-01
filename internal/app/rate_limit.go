package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

type rateLimitState struct {
	reserved            int
	reserveBlockedUntil time.Time
}

type hyperliquidRateStatus struct {
	CumVlm           string `json:"cumVlm"`
	NRequestsUsed    int    `json:"nRequestsUsed"`
	NRequestsCap     int    `json:"nRequestsCap"`
	NRequestsSurplus int    `json:"nRequestsSurplus"`
	UserAbstraction  string `json:"userAbstraction"`
}

func (s *rateLimitState) preflight(ctx context.Context, venueName string, cfg fileConfig) error {
	if venueName != "hyperliquid" || !cfg.RateLimit.Enabled {
		return nil
	}
	status, err := hyperliquidRateLimitCommand(ctx, cfg, "status")
	if err != nil {
		return err
	}
	remaining := hyperliquidRemaining(status)
	minRemaining := cfg.RateLimit.MinRemaining
	if minRemaining <= 0 {
		minRemaining = 100
	}
	if remaining >= minRemaining {
		s.reserveBlockedUntil = time.Time{}
		return nil
	}
	if !cfg.RateLimit.ReserveWhenBelow {
		return fmt.Errorf("hyperliquid request capacity below minimum: remaining=%d min=%d used=%d cap=%d surplus=%d", remaining, minRemaining, status.NRequestsUsed, status.NRequestsCap, status.NRequestsSurplus)
	}
	if status.UserAbstraction != "" && status.UserAbstraction != "disabled" {
		return fmt.Errorf("hyperliquid request capacity below minimum and reserveRequestWeight requires perps balance; account abstraction=%s remaining=%d min=%d", status.UserAbstraction, remaining, minRemaining)
	}
	if time.Now().Before(s.reserveBlockedUntil) {
		return fmt.Errorf("hyperliquid request capacity below minimum and reserve is backing off until %s: remaining=%d min=%d", s.reserveBlockedUntil.UTC().Format(time.RFC3339), remaining, minRemaining)
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
		return fmt.Errorf("hyperliquid auto reserve is enabled but rate_limit.max_reserve_weight is not positive")
	}
	if s.reserved+weight > maxWeight {
		return fmt.Errorf("hyperliquid auto reserve would exceed max_reserve_weight: requested=%d already_reserved=%d max=%d", weight, s.reserved, maxWeight)
	}
	if _, err := hyperliquidRateLimitCommand(ctx, cfg, "reserve", strconv.Itoa(weight)); err != nil {
		s.reserveBlockedUntil = time.Now().Add(time.Hour)
		return err
	}
	s.reserveBlockedUntil = time.Time{}
	s.reserved += weight
	status, err = hyperliquidRateLimitCommand(ctx, cfg, "status")
	if err != nil {
		return err
	}
	remaining = hyperliquidRemaining(status)
	if remaining < minRemaining {
		return fmt.Errorf("hyperliquid request capacity still below minimum after reserve: remaining=%d min=%d", remaining, minRemaining)
	}
	return nil
}

func hyperliquidRemaining(status hyperliquidRateStatus) int {
	return status.NRequestsCap + status.NRequestsSurplus - status.NRequestsUsed
}

func hyperliquidRateLimitCommand(ctx context.Context, cfg fileConfig, args ...string) (hyperliquidRateStatus, error) {
	timeout := durationMS(cfg.RateLimit.TimeoutMS)
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := []string{
		"uv",
		"run",
		"--with",
		"hyperliquid-python-sdk",
		"--with",
		"eth-account",
		"python",
		filepath.FromSlash("internal/venues/hyperliquid/rate_limit.py"),
	}
	command = append(command, args...)
	cmd := exec.CommandContext(cmdCtx, command[0], command[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return hyperliquidRateStatus{}, fmt.Errorf("hyperliquid rate limit %s: %w: %s", args[0], err, stderr.String())
		}
		return hyperliquidRateStatus{}, fmt.Errorf("hyperliquid rate limit %s: %w", args[0], err)
	}
	var status hyperliquidRateStatus
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		return hyperliquidRateStatus{}, fmt.Errorf("decode hyperliquid rate limit response: %w", err)
	}
	return status, nil
}
