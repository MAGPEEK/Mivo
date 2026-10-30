// Command mivo is the composition root of the media server: it parses flags and wires
// adapters to use cases. It contains no business logic.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/magpeek/mivo/internal/platform/brand"
	"github.com/magpeek/mivo/internal/platform/buildinfo"
	"github.com/magpeek/mivo/internal/platform/config"
	"github.com/magpeek/mivo/internal/platform/paths"
)

const (
	exitOK = 0
	// exitFailure signals a runtime error after flags were parsed successfully.
	exitFailure = 1
	// exitUsage matches the exit code of the flag package for invalid command lines.
	exitUsage = 2
)

// errorLineSeparator replaces line breaks in the startup error, so that joined errors still
// print as the single line operators and service managers expect.
const errorLineSeparator = "; "

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	// After the first signal the default behaviour returns, so a second Ctrl+C terminates a
	// shutdown that hangs instead of being swallowed.
	context.AfterFunc(ctx, stop)

	code := run(ctx, os.Args[1:], runEnv{stdout: os.Stdout, stderr: os.Stderr, lookupEnv: os.LookupEnv}, runHooks{})

	stop()
	os.Exit(code)
}

// runEnv is the view of the process run works with, injected so tests never touch the
// host's streams or environment variables.
type runEnv struct {
	stdout io.Writer
	// stderr receives the log and the startup error.
	stderr io.Writer
	// lookupEnv reads an environment variable, os.LookupEnv in production.
	lookupEnv func(key string) (string, bool)
}

// runHooks lets tests observe the running server. The zero value observes nothing.
type runHooks struct {
	// onListening receives the bound address before the server accepts requests; with port
	// 0 it is the only way to learn the port the OS chose.
	onListening func(address net.Addr)
}

func (hooks runHooks) reportListening(address net.Addr) {
	if hooks.onListening != nil {
		hooks.onListening(address)
	}
}

// options holds the parsed command line.
type options struct {
	showVersion bool
	configDir   string
}

// run executes the command with the given arguments and returns the process exit code. The
// server runs until ctx is done.
func run(ctx context.Context, args []string, env runEnv, hooks runHooks) int {
	if len(args) > 0 && args[0] == healthcheckCommand {
		return runHealthcheck(ctx, args[1:], env)
	}

	opts, err := parseOptions(args, env.stderr)
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}

	if err != nil {
		return exitUsage
	}

	if opts.showVersion {
		return printVersion(env.stdout, env.stderr)
	}

	err = serve(ctx, opts, env, hooks)
	if err == nil || stoppedWhileStarting(ctx, err) {
		return exitOK
	}

	// A failed write to stderr cannot be reported anywhere else.
	_, _ = fmt.Fprintln(env.stderr, brand.ID+": "+singleLine(err))

	return exitFailure
}

// stoppedWhileStarting reports an error caused only by a shutdown request that arrived
// before the server was up. Stopping on request is a clean exit, not a startup failure.
func stoppedWhileStarting(ctx context.Context, err error) bool {
	return ctx.Err() != nil && errors.Is(err, ctx.Err())
}

func singleLine(err error) string {
	lineBreaks := strings.NewReplacer("\r\n", errorLineSeparator, "\n", errorLineSeparator, "\r", errorLineSeparator)

	return lineBreaks.Replace(err.Error())
}

func parseOptions(args []string, stderr io.Writer) (options, error) {
	var opts options

	flags := flag.NewFlagSet(brand.ID, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		// A failed write to stderr cannot be reported anywhere else.
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s [flags]\n  %[1]s %[2]s [flags]\n\nFlags:\n",
			brand.ID, healthcheckCommand)
		flags.PrintDefaults()
	}
	flags.BoolVar(&opts.showVersion, "version", false, "print the version and exit")
	flags.StringVar(&opts.configDir, "config-dir", "",
		"directory of the optional "+config.FileName+" (default: $"+paths.ConfigDirEnv()+" or the OS default)")

	if err := flags.Parse(args); err != nil {
		return options{}, fmt.Errorf("parse flags: %w", err)
	}

	if flags.NArg() > 0 {
		err := fmt.Errorf("unexpected argument %q", flags.Arg(0))
		// A failed write to stderr cannot be reported anywhere else.
		_, _ = fmt.Fprintln(stderr, err)
		flags.Usage()

		return options{}, err
	}

	return opts, nil
}

func printVersion(stdout, stderr io.Writer) int {
	if _, err := fmt.Fprintln(stdout, brand.DisplayName+" "+buildinfo.Read().Summary()); err != nil {
		// A failed write to stderr cannot be reported anywhere else.
		_, _ = fmt.Fprintf(stderr, "write version: %v\n", err)

		return exitFailure
	}

	return exitOK
}
