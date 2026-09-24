package app

import (
	"context"
	"fmt"
)

type runPlan struct {
	VenueName string
	Config    fileConfig
}

type runPlanOptions struct {
	ConfigPath         string
	EnvFiles           []string
	FallbackVenue      string
	ConfirmLive        bool
	AllowInlineSecrets bool
	ApplyOverrides     func(*fileConfig)
}

func prepareRunPlan(ctx context.Context, opts runPlanOptions) (runPlan, error) {
	_ = ctx
	cfg, err := loadFileConfig(opts.ConfigPath)
	if err != nil {
		return runPlan{}, err
	}
	if opts.ApplyOverrides != nil {
		opts.ApplyOverrides(&cfg)
	}
	cfg.EnvFiles = append(cfg.EnvFiles, opts.EnvFiles...)
	normalizeFileConfig(&cfg)
	if err := prepareRuntimeEnvironment(cfg, &runOptions{allowInlineSecrets: opts.AllowInlineSecrets}); err != nil {
		return runPlan{}, err
	}

	venueName := normalizedVenue(cfg.Venue, opts.FallbackVenue)
	if venueName != "mock" && !opts.ConfirmLive {
		return runPlan{}, fmt.Errorf("refusing to run live venue %q without --confirm-live", venueName)
	}
	if err := validateRunPlan(venueName, cfg); err != nil {
		return runPlan{}, err
	}
	return runPlan{VenueName: venueName, Config: cfg}, nil
}

func validateRunPlan(venueName string, cfg fileConfig) error {
	if err := validateRunConfig(venueName, cfg); err != nil {
		return err
	}
	if err := validateLifecycleForRun(venueName, cfg); err != nil {
		return err
	}
	if err := validateCleanupForRun(venueName, cfg); err != nil {
		return err
	}
	if err := checkAccountsForRun(venueName, cfg); err != nil {
		return err
	}
	return nil
}
