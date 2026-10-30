package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/magpeek/mivo/internal/adapters/http/healthprobe"
	"github.com/magpeek/mivo/internal/platform/brand"
	"github.com/magpeek/mivo/internal/platform/config"
	"github.com/magpeek/mivo/internal/platform/paths"
)

// healthcheckCommand is the subcommand the container HEALTHCHECK runs. It exits 0 when the
// server on the configured port answers its health endpoint with 200 and 1 otherwise; it
// never uses exit code 2, which Docker reserves.
const healthcheckCommand = "healthcheck"

// runHealthcheck probes the server configured by the same flags and environment as the
// server itself, so it finds the port of a server started with the same settings.
func runHealthcheck(ctx context.Context, args []string, env runEnv) int {
	configDir, err := parseHealthcheckOptions(args, env.stderr)
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}

	if err != nil {
		return exitFailure
	}

	err = probeConfiguredServer(ctx, configDir, env)
	if err != nil {
		// A failed write to stderr cannot be reported anywhere else.
		_, _ = fmt.Fprintln(env.stderr, brand.ID+" "+healthcheckCommand+": "+singleLine(err))

		return exitFailure
	}

	return exitOK
}

func probeConfiguredServer(ctx context.Context, configDir string, env runEnv) error {
	cfg, err := config.Load(config.Options{
		ConfigDirFlag: configDir,
		Environment:   paths.HostEnvironment(env.lookupEnv),
	})
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	err = healthprobe.Check(ctx, healthprobe.Options{
		Address: healthprobe.Address(cfg.Server.Host, cfg.Server.Port),
		Timeout: healthprobe.DefaultTimeout,
	})
	if err != nil {
		return fmt.Errorf("check health: %w", err)
	}

	return nil
}

// parseHealthcheckOptions returns the --config-dir value of the healthcheck subcommand.
func parseHealthcheckOptions(args []string, stderr io.Writer) (string, error) {
	var configDir string

	flags := flag.NewFlagSet(brand.ID+" "+healthcheckCommand, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&configDir, "config-dir", "", "config directory of the server to check (default: as for the server)")

	if err := flags.Parse(args); err != nil {
		return "", fmt.Errorf("parse %s flags: %w", healthcheckCommand, err)
	}

	if flags.NArg() > 0 {
		err := fmt.Errorf("unexpected argument %q", flags.Arg(0))
		// A failed write to stderr cannot be reported anywhere else.
		_, _ = fmt.Fprintln(stderr, err)
		flags.Usage()

		return "", err
	}

	return configDir, nil
}
