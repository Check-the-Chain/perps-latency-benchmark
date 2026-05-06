package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

func prepareRuntimeEnvironment(cfg fileConfig, opts *runOptions) error {
	if !opts.allowInlineSecrets {
		if err := validateNoInlineSecrets(cfg); err != nil {
			return err
		}
	}
	shellEnv := currentEnvKeys()
	for _, envFile := range cfg.EnvFiles {
		if strings.TrimSpace(envFile) == "" {
			continue
		}
		values, err := godotenv.Read(envFile)
		if err != nil {
			return fmt.Errorf("load env file %q: %w", envFile, err)
		}
		for key, value := range values {
			if _, exists := shellEnv[key]; exists {
				continue
			}
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("set env %q from %q: %w", key, envFile, err)
			}
		}
	}
	return nil
}

func currentEnvKeys() map[string]struct{} {
	keys := make(map[string]struct{})
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		keys[key] = struct{}{}
	}
	return keys
}
