package app

import "testing"

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
}
