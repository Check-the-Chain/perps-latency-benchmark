package netbaseline

import (
	"testing"

	"perps-latency-benchmark/internal/netlatency"
)

func TestTargetFromURLUsesSameRouteHostAndPort(t *testing.T) {
	target, ok := TargetFromURL("wss://api.hyperliquid.xyz/ws")
	if !ok {
		t.Fatal("target not parsed")
	}
	if target.Address != "api.hyperliquid.xyz:443" {
		t.Fatalf("address = %q", target.Address)
	}
	if target.Source != "tcp_connect_p10:wss://api.hyperliquid.xyz:443" {
		t.Fatalf("source = %q", target.Source)
	}
}

func TestMonitorSnapshotPrefersRequestConnectOverFallback(t *testing.T) {
	monitor := NewMonitor(Target{Source: "tcp_connect_p10:example.test:443"}, 0, 10)
	monitor.record(5_000_000, "tcp_connect_p10:example.test:443", priorityTCPConnectFallback)
	monitor.ObserveTrace(netlatency.Trace{
		Transport: "http",
		ConnectNS: 2_000_000,
	})

	snapshot := monitor.Snapshot()
	if snapshot.FloorNS != 2_000_000 || snapshot.Source != "request_connect" {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestMonitorSnapshotPrefersWebSocketHeartbeat(t *testing.T) {
	monitor := NewMonitor(Target{Source: "tcp_connect_p10:example.test:443"}, 0, 10)
	monitor.record(5_000_000, "tcp_connect_p10:example.test:443", priorityTCPConnectFallback)
	monitor.ObserveTrace(netlatency.Trace{
		Transport: "http",
		ConnectNS: 2_000_000,
	})
	monitor.ObserveRTT(1_500_000, "ws_heartbeat")

	snapshot := monitor.Snapshot()
	if snapshot.FloorNS != 1_500_000 || snapshot.Source != "ws_heartbeat" {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}
