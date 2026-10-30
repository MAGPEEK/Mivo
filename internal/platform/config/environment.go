package config

import (
	"fmt"

	"github.com/magpeek/mivo/internal/platform/paths"
)

// applyEnvironment applies every set environment variable. A variable set to an empty
// value counts as set, so empty paths are reported instead of silently ignored.
func applyEnvironment(cfg *Config, env paths.Environment, configDir string) error {
	for _, entry := range settings() {
		name := envName(entry.key)

		raw, ok := env.Lookup(name)
		if !ok {
			continue
		}

		if err := entry.apply(cfg, raw, configDir); err != nil {
			return fmt.Errorf("environment variable %s: %w", name, err)
		}
	}

	return nil
}
