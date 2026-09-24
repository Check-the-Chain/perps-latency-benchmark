package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildServiceTopologyProjectsCollectorAndAPI(t *testing.T) {
	topology, err := buildServiceTopology(&serviceTopologyOptions{
		configPaths:     []string{"examples/lighter-builder.json"},
		envFiles:        []string{".env.wallets.local"},
		storePath:       "/var/lib/perps/bench.db",
		listen:          "0.0.0.0:8080",
		corsOrigin:      "",
		authUser:        "bench",
		authPasswordEnv: "PERPS_BENCH_API_PASSWORD",
		chunkIterations: 1,
		retainHours:     24,
		fundingConfig:   "examples/funding.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !topology.API.RequiresAuth {
		t.Fatalf("public API should require auth: %+v", topology.API)
	}
	if len(topology.Collectors) != 1 {
		t.Fatalf("collectors = %+v", topology.Collectors)
	}
	collector := topology.Collectors[0]
	if collector.Command[0] != "perps-bench" || collector.Command[1] != "run-continuous" {
		t.Fatalf("collector command = %#v", collector.Command)
	}
	if collector.Command[len(collector.Command)-2] != "--env-file" || collector.Command[len(collector.Command)-1] != ".env.wallets.local" {
		t.Fatalf("collector env args = %#v", collector.Command)
	}
	if topology.Funding == nil || topology.Funding.Command[1] != "funding" {
		t.Fatalf("funding service = %#v", topology.Funding)
	}
}

func TestBuildServiceTopologyProjectsExchangeTPS(t *testing.T) {
	topology, err := buildServiceTopology(&serviceTopologyOptions{
		storePath:                         "/var/lib/perps/bench.db",
		listen:                            "127.0.0.1:8080",
		corsOrigin:                        "*",
		authUser:                          "bench",
		authPasswordEnv:                   "PERPS_BENCH_API_PASSWORD",
		chunkIterations:                   1,
		retainHours:                       24,
		exchangeTPSStore:                  "/var/lib/perps/exchange_tps.db",
		exchangeTPSVenues:                 []string{"hyperliquid"},
		exchangeTPSMinuteRetention:        time.Hour,
		exchangeTPSFlushInterval:          2 * time.Second,
		hyperliquidActionSamplesPerMinute: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(topology.ExchangeTPS) != 1 {
		t.Fatalf("exchange TPS services = %+v", topology.ExchangeTPS)
	}
	service := topology.ExchangeTPS[0]
	if service.Venue != "hyperliquid" || service.StorePath != "/var/lib/perps/exchange_tps.db" {
		t.Fatalf("exchange TPS service = %+v", service)
	}
	command := strings.Join(service.Command, " ")
	for _, want := range []string{
		"collect-exchange-tps",
		"--venue hyperliquid",
		"--hyperliquid-action-samples-per-minute 60",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("exchange TPS command %q missing %q", command, want)
		}
	}
}

func TestValidateServiceTopologyBinaryRejectsUnsupportedFlag(t *testing.T) {
	binary := fakePerpsBenchBinary(t, `#!/bin/sh
case "$*" in
  "collect-exchange-tps --help")
    echo "Usage: collect-exchange-tps --venue --store --flush-interval --minute-retention"
    exit 0
    ;;
  *)
    echo "unexpected args: $*" >&2
    exit 1
    ;;
esac
`)
	topology := serviceTopology{
		ExchangeTPS: []serviceTopologyExchangeTPS{{
			Venue: "hyperliquid",
			Command: []string{
				"perps-bench",
				"collect-exchange-tps",
				"--venue", "hyperliquid",
				"--store", "data/exchange_tps.db",
				"--hyperliquid-action-samples-per-minute", "120",
			},
		}},
	}
	err := validateServiceTopologyBinary(context.Background(), binary, topology)
	if err == nil || !strings.Contains(err.Error(), "--hyperliquid-action-samples-per-minute") {
		t.Fatalf("validateServiceTopologyBinary() = %v", err)
	}
}

func TestValidateServiceTopologyBinaryAcceptsSupportedFlags(t *testing.T) {
	binary := fakePerpsBenchBinary(t, `#!/bin/sh
case "$*" in
  "collect-exchange-tps --help")
    echo "Usage: collect-exchange-tps --venue --store --hyperliquid-action-samples-per-minute"
    exit 0
    ;;
  *)
    echo "unexpected args: $*" >&2
    exit 1
    ;;
esac
`)
	topology := serviceTopology{
		ExchangeTPS: []serviceTopologyExchangeTPS{{
			Venue: "hyperliquid",
			Command: []string{
				"perps-bench",
				"collect-exchange-tps",
				"--venue", "hyperliquid",
				"--store", "data/exchange_tps.db",
				"--hyperliquid-action-samples-per-minute", "120",
			},
		}},
	}
	if err := validateServiceTopologyBinary(context.Background(), binary, topology); err != nil {
		t.Fatalf("validateServiceTopologyBinary() = %v", err)
	}
}

func fakePerpsBenchBinary(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "perps-bench")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
