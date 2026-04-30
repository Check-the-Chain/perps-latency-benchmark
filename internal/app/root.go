package app

import "github.com/spf13/cobra"

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "perps-bench",
		Short: "Benchmark network-only latency for crypto perps venue requests.",
	}

	runOpts := &runOptions{}
	run := &cobra.Command{
		Use:   "run",
		Short: "Run a latency benchmark.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBenchmark(cmd.Context(), cmd, runOpts)
		},
	}
	addRunFlags(run, runOpts)

	compareOpts := &runOptions{}
	compare := &cobra.Command{
		Use:   "compare-transports",
		Short: "Run the same benchmark plan over HTTPS and WebSocket transports.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTransportComparison(cmd.Context(), cmd, compareOpts)
		},
	}
	addRunFlags(compare, compareOpts)
	compare.Flags().StringVar(&compareOpts.transports, "transports", "https,websocket", "Comma-separated transports to compare.")

	root.AddCommand(run)
	root.AddCommand(newRunContinuousCommand())
	root.AddCommand(compare)
	root.AddCommand(newCompareResultsCommand())
	root.AddCommand(newServeCommand())
	root.AddCommand(newAccountsCommand())
	return root
}
