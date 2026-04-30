package app

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"perps-latency-benchmark/internal/bench"
)

func newCompareResultsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "compare-results <result.json>...",
		Short: "Compare saved benchmark result files.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rows, err := loadResultRows(args)
			if err != nil {
				return err
			}
			writeResultRows(cmd.OutOrStdout(), rows)
			return nil
		},
	}
}

type resultRow struct {
	File           string
	Result         bench.Result
	Summary        bench.Summary
	Transport      string
	Startup        string
	Reconciliation string
}

func loadResultRows(paths []string) ([]resultRow, error) {
	var rows []resultRow
	for _, path := range paths {
		results, err := readResults(path)
		if err != nil {
			return nil, err
		}
		for _, result := range results {
			rows = append(rows, resultRow{
				File:           path,
				Result:         result,
				Summary:        bench.Summarize(result.Samples),
				Transport:      resultTransport(result),
				Startup:        cleanupStatus(result.StartupCleanup),
				Reconciliation: cleanupStatus(result.Reconciliation),
			})
		}
	}
	return rows, nil
}

func readResults(path string) ([]bench.Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var comparison bench.ComparisonResult
	if err := json.Unmarshal(data, &comparison); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(comparison.Results) > 0 {
		return comparison.Results, nil
	}
	var result bench.Result
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return []bench.Result{result}, nil
}

func writeResultRows(out interface{ Write([]byte) (int, error) }, rows []resultRow) {
	writer := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "file\tvenue\ttransport\tscenario\tmode\tcount\tok\tfailed\tmean_ms\tp50_ms\tp95_ms\tp99_ms\tcleanup\tstartup\treconciliation")
	for _, row := range rows {
		fmt.Fprintf(
			writer,
			"%s\t%s\t%s\t%s\t%s\t%d\t%d\t%d\t%.3f\t%.3f\t%.3f\t%.3f\t%d/%d\t%s\t%s\n",
			row.File,
			row.Result.Venue,
			row.Transport,
			row.Result.Scenario,
			row.Result.LatencyMode,
			row.Summary.Count,
			row.Summary.OK,
			row.Summary.Failed,
			row.Summary.MeanMS,
			row.Summary.P50MS,
			row.Summary.P95MS,
			row.Summary.P99MS,
			row.Summary.Cleanup.OK,
			row.Summary.Cleanup.Attempted,
			row.Startup,
			row.Reconciliation,
		)
	}
	_ = writer.Flush()
}

func resultTransport(result bench.Result) string {
	for _, sample := range result.Samples {
		if sample.Transport != "" {
			return sample.Transport
		}
	}
	return ""
}

func cleanupStatus(cleanup *bench.CleanupResult) string {
	if cleanup == nil {
		return ""
	}
	if cleanup.OK {
		return "ok"
	}
	return "failed"
}
