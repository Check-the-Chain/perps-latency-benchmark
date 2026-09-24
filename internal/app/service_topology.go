package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"perps-latency-benchmark/internal/exchangetps"
)

type serviceTopologyOptions struct {
	configPaths                       []string
	envFiles                          []string
	storePath                         string
	listen                            string
	corsOrigin                        string
	authUser                          string
	authPasswordEnv                   string
	chunkIterations                   int
	retainHours                       int
	fundingConfig                     string
	exchangeTPSStore                  string
	exchangeTPSVenues                 []string
	exchangeTPSMinuteRetention        time.Duration
	exchangeTPSFlushInterval          time.Duration
	hyperliquidActionSamplesPerMinute int
	validateBinary                    string
}

type serviceTopology struct {
	StorePath   string                       `json:"store_path"`
	API         serviceTopologyAPI           `json:"api"`
	Collectors  []serviceTopologyCollector   `json:"collectors"`
	Funding     *serviceTopologyFunding      `json:"funding,omitempty"`
	ExchangeTPS []serviceTopologyExchangeTPS `json:"exchange_tps,omitempty"`
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

type serviceTopologyFunding struct {
	ConfigPath string   `json:"config_path"`
	Command    []string `json:"command"`
}

type serviceTopologyExchangeTPS struct {
	Venue           string   `json:"venue"`
	StorePath       string   `json:"store_path"`
	Command         []string `json:"command"`
	MinuteRetention string   `json:"minute_retention"`
	FlushInterval   string   `json:"flush_interval"`
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
			if opts.validateBinary != "" {
				if err := validateServiceTopologyBinary(cmd.Context(), opts.validateBinary, topology); err != nil {
					return err
				}
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
	cmd.Flags().StringVar(&opts.fundingConfig, "funding-config", "", "Optional funding monitor config path.")
	cmd.Flags().StringVar(&opts.exchangeTPSStore, "exchange-tps-store", "data/exchange_tps.db", "SQLite exchange TPS store path.")
	cmd.Flags().StringArrayVar(&opts.exchangeTPSVenues, "exchange-tps-venue", nil, "Exchange TPS collector venue. Repeatable.")
	cmd.Flags().DurationVar(&opts.exchangeTPSMinuteRetention, "exchange-tps-minute-retention", 365*24*time.Hour, "Exchange TPS 1m bucket retention.")
	cmd.Flags().DurationVar(&opts.exchangeTPSFlushInterval, "exchange-tps-flush-interval", time.Second, "Exchange TPS collector flush interval.")
	cmd.Flags().IntVar(&opts.hyperliquidActionSamplesPerMinute, "hyperliquid-action-samples-per-minute", exchangetps.DefaultHyperliquidActionSamplesPerMinute, "Hyperliquid blockDetails samples per minute for action breakdowns.")
	cmd.Flags().StringVar(&opts.validateBinary, "validate-binary", "", "Validate generated command flags against a perps-bench binary.")
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
	if opts.fundingConfig != "" {
		topology.Funding = &serviceTopologyFunding{
			ConfigPath: opts.fundingConfig,
			Command: []string{
				"perps-bench",
				"funding",
				"monitor",
				"--config", opts.fundingConfig,
				"--confirm-live",
			},
		}
	}
	for _, venue := range opts.exchangeTPSVenues {
		venue = strings.TrimSpace(venue)
		if venue == "" {
			continue
		}
		topology.ExchangeTPS = append(topology.ExchangeTPS, serviceTopologyExchangeTPS{
			Venue:           venue,
			StorePath:       opts.exchangeTPSStore,
			Command:         exchangeTPSCommandArgs(opts, venue),
			MinuteRetention: opts.exchangeTPSMinuteRetention.String(),
			FlushInterval:   opts.exchangeTPSFlushInterval.String(),
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

func exchangeTPSCommandArgs(opts *serviceTopologyOptions, venue string) []string {
	args := []string{
		"perps-bench",
		"collect-exchange-tps",
		"--venue", venue,
		"--store", opts.exchangeTPSStore,
		"--flush-interval", opts.exchangeTPSFlushInterval.String(),
		"--minute-retention", opts.exchangeTPSMinuteRetention.String(),
	}
	if venue == "hyperliquid" {
		args = append(args, "--hyperliquid-action-samples-per-minute", fmt.Sprint(opts.hyperliquidActionSamplesPerMinute))
	}
	return args
}

func validateServiceTopologyBinary(ctx context.Context, binary string, topology serviceTopology) error {
	var commands [][]string
	if len(topology.API.Command) > 0 {
		commands = append(commands, topology.API.Command)
	}
	for _, collector := range topology.Collectors {
		commands = append(commands, collector.Command)
	}
	if topology.Funding != nil {
		commands = append(commands, topology.Funding.Command)
	}
	for _, collector := range topology.ExchangeTPS {
		commands = append(commands, collector.Command)
	}
	for _, command := range commands {
		if err := validateCommandFlags(ctx, binary, command); err != nil {
			return err
		}
	}
	return nil
}

func validateCommandFlags(ctx context.Context, binary string, command []string) error {
	if len(command) < 2 {
		return fmt.Errorf("cannot validate command with no subcommand: %#v", command)
	}
	var subcommands []string
	argIndex := 1
	for ; argIndex < len(command); argIndex++ {
		if strings.HasPrefix(command[argIndex], "-") {
			break
		}
		subcommands = append(subcommands, command[argIndex])
	}
	helpArgs := append(append([]string(nil), subcommands...), "--help")
	helpCmd := exec.CommandContext(ctx, binary, helpArgs...)
	output, err := helpCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("validate %s help: %w: %s", strings.Join(subcommands, " "), err, strings.TrimSpace(string(output)))
	}
	for _, arg := range command[argIndex:] {
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		flag := arg
		if equals := strings.IndexByte(flag, '='); equals >= 0 {
			flag = flag[:equals]
		}
		if !bytes.Contains(output, []byte(flag)) {
			return fmt.Errorf("%s does not support flag %s for command %s", binary, flag, strings.Join(subcommands, " "))
		}
	}
	return nil
}
