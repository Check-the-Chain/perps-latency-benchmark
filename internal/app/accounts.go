package app

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"perps-latency-benchmark/internal/accounts"
)

type accountOptions struct {
	configPath string
	envFiles   []string
	venues     string
	outPath    string
}

func newAccountsCommand() *cobra.Command {
	opts := &accountOptions{}
	accountsCmd := &cobra.Command{
		Use:   "accounts",
		Short: "Inspect and prepare benchmark account credentials.",
	}

	plan := &cobra.Command{
		Use:   "plan",
		Short: "Print wallet kinds, env vars, and manual setup required for venues.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			specs, err := accountSpecsFromOptions(opts)
			if err != nil {
				return err
			}
			printAccountPlan(cmd.OutOrStdout(), specs)
			return nil
		},
	}
	addAccountSelectionFlags(plan, opts)

	generateOpts := &accountOptions{}
	generate := &cobra.Command{
		Use:   "generate",
		Short: "Generate missing local wallet keys into a dotenv file.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			specs, err := accountSpecsFromOptions(generateOpts)
			if err != nil {
				return err
			}
			existing, err := accounts.LoadDotenv(generateOpts.outPath)
			if err != nil {
				return err
			}
			values, wallets, err := accounts.Generate(specs, existing)
			if err != nil {
				return err
			}
			if err := accounts.WriteDotenv(generateOpts.outPath, values); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", generateOpts.outPath)
			printWalletSummary(cmd.OutOrStdout(), wallets)
			printAccountPlan(cmd.OutOrStdout(), specs)
			return nil
		},
	}
	addAccountSelectionFlags(generate, generateOpts)
	generate.Flags().StringVar(&generateOpts.outPath, "out", ".env.wallets.local", "Local dotenv file to create/update with generated wallet secrets.")

	checkOpts := &accountOptions{}
	check := &cobra.Command{
		Use:   "check",
		Short: "Check required benchmark account env vars are present.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, specs, err := accountCheckInputs(checkOpts)
			if err != nil {
				return err
			}
			if err := prepareRuntimeEnvironment(cfg, &runOptions{}); err != nil {
				return err
			}
			printAccountStatus(cmd.OutOrStdout(), accounts.Status(specs))
			return accounts.Check(specs)
		},
	}
	addAccountSelectionFlags(check, checkOpts)
	check.Flags().StringVar(&checkOpts.configPath, "config", "", "JSON benchmark config file. Used to infer venue/env_files.")
	check.Flags().StringArrayVar(&checkOpts.envFiles, "env-file", nil, "Load dotenv credentials file before checking. Repeatable.")

	printOpts := &accountOptions{}
	printCmd := &cobra.Command{
		Use:   "print",
		Short: "Print public wallet identifiers from loaded credentials.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			specs, err := accountSpecsFromOptions(printOpts)
			if err != nil {
				return err
			}
			cfg := fileConfig{EnvFiles: printOpts.envFiles}
			if err := prepareRuntimeEnvironment(cfg, &runOptions{}); err != nil {
				return err
			}
			env := accounts.EnvMap()
			for _, kind := range accounts.RequiredWalletKinds(specs) {
				wallet, err := accounts.PublicFromEnv(kind, env)
				if err != nil {
					return err
				}
				switch kind {
				case accounts.WalletEVM:
					if wallet.Address == "" {
						fmt.Fprintln(cmd.OutOrStdout(), "evm: missing")
					} else {
						fmt.Fprintf(cmd.OutOrStdout(), "evm address: %s\n", wallet.Address)
					}
				case accounts.WalletStark:
					if wallet.PublicKey == "" {
						fmt.Fprintln(cmd.OutOrStdout(), "stark: missing")
					} else {
						fmt.Fprintf(cmd.OutOrStdout(), "stark public key: %s\n", wallet.PublicKey)
					}
				case accounts.WalletLighterL2:
					if env["LIGHTER_PRIVATE_KEY"] == "" {
						fmt.Fprintln(cmd.OutOrStdout(), "lighter_l2: missing")
					} else {
						fmt.Fprintln(cmd.OutOrStdout(), "lighter_l2 key: present")
					}
				}
			}
			return nil
		},
	}
	addAccountSelectionFlags(printCmd, printOpts)
	printCmd.Flags().StringArrayVar(&printOpts.envFiles, "env-file", nil, "Load dotenv credentials file before printing. Repeatable.")

	checklistOpts := &accountOptions{}
	checklist := &cobra.Command{
		Use:   "checklist",
		Short: "Print the concrete setup checklist for selected venues.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			specs, err := accountSpecsFromOptions(checklistOpts)
			if err != nil {
				return err
			}
			cfg := fileConfig{EnvFiles: checklistOpts.envFiles}
			if err := prepareRuntimeEnvironment(cfg, &runOptions{}); err != nil {
				return err
			}
			printAccountChecklist(cmd.OutOrStdout(), specs, accounts.EnvMap())
			return nil
		},
	}
	addAccountSelectionFlags(checklist, checklistOpts)
	checklist.Flags().StringArrayVar(&checklistOpts.envFiles, "env-file", nil, "Load dotenv credentials file before printing. Repeatable.")

	accountsCmd.AddCommand(plan, generate, check, printCmd, checklist)
	return accountsCmd
}

func addAccountSelectionFlags(cmd *cobra.Command, opts *accountOptions) {
	cmd.Flags().StringVar(&opts.venues, "venues", "all", "Comma-separated venues or all.")
}

func accountSpecsFromOptions(opts *accountOptions) ([]accounts.VenueSpec, error) {
	return accounts.ResolveVenues(opts.venues)
}

func accountCheckInputs(opts *accountOptions) (fileConfig, []accounts.VenueSpec, error) {
	cfg, err := loadFileConfig(opts.configPath)
	if err != nil {
		return fileConfig{}, nil, err
	}
	cfg.EnvFiles = append(cfg.EnvFiles, opts.envFiles...)

	venues := opts.venues
	if venues == "" || venues == "all" {
		if cfg.Venue != "" {
			venues = cfg.Venue
		}
	}
	specs, err := accounts.ResolveVenues(venues)
	if err != nil {
		return fileConfig{}, nil, err
	}
	return cfg, specs, nil
}

func printAccountPlan(w io.Writer, specs []accounts.VenueSpec) {
	fmt.Fprintln(w, "\naccount setup plan")
	kinds := accounts.RequiredWalletKinds(specs)
	if len(kinds) > 0 {
		fmt.Fprintf(w, "wallet kinds: %s\n", joinWalletKinds(kinds))
	}
	for _, spec := range specs {
		fmt.Fprintf(w, "\n[%s]\n", spec.Name)
		if !spec.Supported {
			fmt.Fprintln(w, "status: not ready for live benchmark account setup")
		}
		if kinds := accounts.WalletKinds(spec); len(kinds) > 0 {
			fmt.Fprintf(w, "wallets: %s\n", joinWalletKinds(kinds))
		}
		if len(spec.Env) > 0 {
			fmt.Fprintln(w, "env:")
			for _, env := range spec.Env {
				kind := "manual"
				if env.Generated {
					kind = "generated"
				}
				required := "optional"
				if env.Required {
					required = "required"
				}
				secret := ""
				if env.Secret {
					secret = ", secret"
				}
				fmt.Fprintf(w, "  %s (%s, %s%s)\n", env.Name, kind, required, secret)
			}
		}
		if len(spec.ManualSteps) > 0 {
			fmt.Fprintln(w, "manual:")
			for _, step := range spec.ManualSteps {
				fmt.Fprintf(w, "  - %s\n", step)
			}
		}
	}
}

func printWalletSummary(w io.Writer, wallets map[accounts.WalletKind]accounts.Wallet) {
	if len(wallets) == 0 {
		fmt.Fprintln(w, "no new wallets generated")
		return
	}
	fmt.Fprintln(w, "generated wallets:")
	kinds := make([]accounts.WalletKind, 0, len(wallets))
	for kind := range wallets {
		kinds = append(kinds, kind)
	}
	sort.Slice(kinds, func(i, j int) bool {
		return kinds[i] < kinds[j]
	})
	for _, kind := range kinds {
		wallet := wallets[kind]
		switch kind {
		case accounts.WalletEVM:
			fmt.Fprintf(w, "  evm address: %s\n", wallet.Address)
		case accounts.WalletStark:
			fmt.Fprintf(w, "  stark public key: %s\n", wallet.PublicKey)
		case accounts.WalletLighterL2:
			fmt.Fprintln(w, "  lighter_l2 key: generated")
		}
	}
}

func printAccountStatus(w io.Writer, statuses []accounts.VenueStatus) {
	for _, status := range statuses {
		fmt.Fprintf(w, "\n[%s]\n", status.Name)
		if !status.Supported {
			fmt.Fprintln(w, "status: unsupported")
			continue
		}
		if len(status.WalletKinds) > 0 {
			fmt.Fprintf(w, "wallets: %s\n", joinWalletKinds(status.WalletKinds))
		}
		for _, env := range status.Env {
			marker := "missing"
			if env.Present {
				marker = "present"
			}
			required := "optional"
			if env.Required {
				required = "required"
			}
			fmt.Fprintf(w, "  %s: %s (%s)\n", env.Name, marker, required)
		}
	}
}

func printAccountChecklist(w io.Writer, specs []accounts.VenueSpec, env map[string]string) {
	venues := joinSpecNames(specs)
	fmt.Fprintln(w, "\nbenchmark setup checklist")
	fmt.Fprintf(w, "venues: %s\n\n", venues)
	fmt.Fprintln(w, "1. Generate local wallets if you have not already:")
	fmt.Fprintf(w, "   go run ./cmd/perps-bench accounts generate --venues %s --out .env.wallets.local\n\n", venues)
	fmt.Fprintln(w, "2. Use these public identifiers for venue setup:")
	printLoadedWalletIdentifiers(w, specs, env)
	fmt.Fprintln(w, "\n3. Complete venue setup:")
	for _, spec := range specs {
		printVenueChecklist(w, spec, env)
	}
	fmt.Fprintln(w, "\n4. Verify env before benchmarking:")
	fmt.Fprintf(w, "   go run ./cmd/perps-bench accounts check --venues %s --env-file .env.wallets.local\n", venues)
}

func printLoadedWalletIdentifiers(w io.Writer, specs []accounts.VenueSpec, env map[string]string) {
	for _, kind := range accounts.RequiredWalletKinds(specs) {
		wallet, err := accounts.PublicFromEnv(kind, env)
		if err != nil {
			fmt.Fprintf(w, "   %s: invalid loaded key: %v\n", kind, err)
			continue
		}
		switch kind {
		case accounts.WalletEVM:
			printValueOrMissing(w, "evm address", wallet.Address)
		case accounts.WalletStark:
			printValueOrMissing(w, "stark public key", wallet.PublicKey)
		case accounts.WalletLighterL2:
			if env["LIGHTER_PRIVATE_KEY"] == "" {
				fmt.Fprintln(w, "   lighter_l2 key: missing")
			} else {
				fmt.Fprintln(w, "   lighter_l2 key: present")
			}
		}
	}
}

func printVenueChecklist(w io.Writer, spec accounts.VenueSpec, env map[string]string) {
	fmt.Fprintf(w, "\n[%s]\n", spec.Name)
	switch spec.Name {
	case "hyperliquid":
		evmAddress := evmAddress(env)
		printValueOrMissing(w, "trading/agent wallet", evmAddress)
		fmt.Fprintln(w, "   - Register or approve this wallet for Hyperliquid trading if needed.")
		fmt.Fprintln(w, "   - Fund the Hyperliquid account or agent wallet with enough collateral for the configured market.")
		fmt.Fprintln(w, "   - Keep the example order post-only while validating repeatability.")
		fmt.Fprintln(w, "   - Update examples/hyperliquid-builder.json: symbol, asset, size, price.")
		fmt.Fprintln(w, "   - Run with: go run ./cmd/perps-bench run --config examples/hyperliquid-builder.json --env-file .env.wallets.local --confirm-live")
	case "lighter":
		l1Address := env["LIGHTER_L1_ADDRESS"]
		if l1Address == "" {
			l1Address = evmAddress(env)
		}
		printValueOrMissing(w, "ethereum setup/funding wallet", l1Address)
		fmt.Fprintln(w, "   - Use the Ethereum wallet for Lighter account creation and deposits.")
		fmt.Fprintln(w, "   - Create or register the Lighter API key, then fill LIGHTER_ACCOUNT_INDEX and LIGHTER_API_KEY_INDEX in .env.wallets.local.")
		fmt.Fprintln(w, "   - Fund the Lighter account with enough collateral for the configured market.")
		fmt.Fprintln(w, "   - Update examples/lighter-builder.json: market_index, base_amount, price.")
		fmt.Fprintln(w, "   - Run with: go run ./cmd/perps-bench run --config examples/lighter-builder.json --env-file .env.wallets.local --confirm-live")
	default:
		for _, step := range spec.ManualSteps {
			fmt.Fprintf(w, "   - %s\n", step)
		}
	}
}

func evmAddress(env map[string]string) string {
	wallet, err := accounts.PublicFromEnv(accounts.WalletEVM, env)
	if err != nil {
		return ""
	}
	return wallet.Address
}

func printValueOrMissing(w io.Writer, label string, value string) {
	if value == "" {
		fmt.Fprintf(w, "   %s: missing\n", label)
		return
	}
	fmt.Fprintf(w, "   %s: %s\n", label, value)
}

func joinWalletKinds(kinds []accounts.WalletKind) string {
	parts := make([]string, len(kinds))
	for i, kind := range kinds {
		parts[i] = string(kind)
	}
	return strings.Join(parts, ", ")
}

func joinSpecNames(specs []accounts.VenueSpec) string {
	parts := make([]string, len(specs))
	for i, spec := range specs {
		parts[i] = spec.Name
	}
	return strings.Join(parts, ",")
}
