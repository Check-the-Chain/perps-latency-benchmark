package app

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

type serviceTopologyOptions struct {
	configPaths     []string
	envFiles        []string
	storePath       string
	listen          string
	corsOrigin      string
	authUser        string
	authPasswordEnv string
	chunkIterations int
	retainHours     int
}

type serviceTopology struct {
	StorePath  string                     `json:"store_path"`
	API        serviceTopologyAPI         `json:"api"`
	Collectors []serviceTopologyCollector `json:"collectors"`
}

type serviceTopologyAPI struct {
	Command         []string `json:"command"`
	Listen          string   `json:"listen"`
	CORSOrigin      string   `json:"cors_origin"`
	AuthUser        string   `json:"auth_user"`
	AuthPasswordEnv string   `json:"auth_password_env"`
	RequiresAuth    bool     `json:"requires_auth"`
}

type serviceTopologyCollector struct {
	ConfigPath      string   `json:"config_path"`
	EnvFiles        []string `json:"env_files,omitempty"`
	Command         []string `json:"command"`
	ChunkIterations int      `json:"chunk_iterations"`
	RetainHours     int      `json:"retain_hours"`
}

func newServiceTopologyCommand() *cobra.Command {
	opts := &serviceTopologyOptions{}
	cmd := &cobra.Command{
		Use:   "service-topology",
		Short: "Print the repo-owned collector/API service topology.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			topology, err := buildServiceTopology(opts)
			if err != nil {
				return err
			}
			encoded, err := json.MarshalIndent(topology, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&opts.configPaths, "config", nil, "Collector JSON config path. Repeatable.")
	cmd.Flags().StringArrayVar(&opts.envFiles, "env-file", nil, "Collector dotenv file. Repeatable.")
	cmd.Flags().StringVar(&opts.storePath, "store", "data/bench.db", "SQLite result store path shared by collectors and API.")
	cmd.Flags().StringVar(&opts.listen, "listen", "127.0.0.1:8080", "API listen address.")
	cmd.Flags().StringVar(&opts.corsOrigin, "cors-origin", "*", "API CORS origin.")
	cmd.Flags().StringVar(&opts.authUser, "auth-user", "bench", "API Basic auth username.")
	cmd.Flags().StringVar(&opts.authPasswordEnv, "auth-password-env", "PERPS_BENCH_API_PASSWORD", "API Basic auth password environment variable.")
	cmd.Flags().IntVar(&opts.chunkIterations, "chunk-iterations", 10, "Measured iterations per collector chunk.")
	cmd.Flags().IntVar(&opts.retainHours, "retain-hours", 168, "Stored sample retention hours.")
	return cmd
}

func buildServiceTopology(opts *serviceTopologyOptions) (serviceTopology, error) {
	if opts.chunkIterations <= 0 {
		return serviceTopology{}, fmt.Errorf("chunk-iterations must be positive")
	}
	if opts.retainHours < 0 {
		return serviceTopology{}, fmt.Errorf("retain-hours cannot be negative")
	}
	topology := serviceTopology{
		StorePath: opts.storePath,
		API: serviceTopologyAPI{
			Command:         serveCommandArgs(opts),
			Listen:          opts.listen,
			CORSOrigin:      opts.corsOrigin,
			AuthUser:        opts.authUser,
			AuthPasswordEnv: opts.authPasswordEnv,
			RequiresAuth:    requiresServeAuth(opts.listen),
		},
	}
	for _, configPath := range opts.configPaths {
		topology.Collectors = append(topology.Collectors, serviceTopologyCollector{
			ConfigPath:      configPath,
			EnvFiles:        append([]string(nil), opts.envFiles...),
			Command:         collectorCommandArgs(opts, configPath),
			ChunkIterations: opts.chunkIterations,
			RetainHours:     opts.retainHours,
		})
	}
	return topology, nil
}

func serveCommandArgs(opts *serviceTopologyOptions) []string {
	return []string{
		"perps-bench",
		"serve",
		"--store", opts.storePath,
		"--listen", opts.listen,
		"--cors-origin", opts.corsOrigin,
		"--auth-user", opts.authUser,
		"--auth-password-env", opts.authPasswordEnv,
	}
}

func collectorCommandArgs(opts *serviceTopologyOptions, configPath string) []string {
	args := []string{
		"perps-bench",
		"run-continuous",
		"--config", configPath,
		"--store", opts.storePath,
		"--chunk-iterations", fmt.Sprint(opts.chunkIterations),
		"--retain-hours", fmt.Sprint(opts.retainHours),
		"--confirm-live",
	}
	for _, envFile := range opts.envFiles {
		args = append(args, "--env-file", envFile)
	}
	return args
}
