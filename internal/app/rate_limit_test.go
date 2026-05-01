package app

import "testing"

func TestHyperliquidRemaining(t *testing.T) {
	status := hyperliquidRateStatus{
		NRequestsUsed:    13419,
		NRequestsCap:     13340,
		NRequestsSurplus: 500,
	}
	if got := hyperliquidRemaining(status); got != 421 {
		t.Fatalf("hyperliquidRemaining() = %d, want 421", got)
	}
}
