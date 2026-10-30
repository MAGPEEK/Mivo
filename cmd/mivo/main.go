// Command mivo is the composition root of the media server: it parses flags and wires
// adapters to use cases. It contains no business logic.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/magpeek/mivo/internal/platform/brand"
	"github.com/magpeek/mivo/internal/platform/buildinfo"
)

const (
	exitOK = 0
	// exitFailure signals a runtime error after flags were parsed successfully.
	exitFailure = 1
	// exitUsage matches the exit code of the flag package for invalid command lines.
	exitUsage = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// options holds the parsed command line.
type options struct {
	showVersion bool
}

// run executes the command with the given arguments and returns the process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	opts, err := parseOptions(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}

	if err != nil {
		return exitUsage
	}

	if opts.showVersion {
		return printVersion(stdout, stderr)
	}

	// M1-10 starts the HTTP server here.
	return exitOK
}

func parseOptions(args []string, stderr io.Writer) (options, error) {
	var opts options

	flags := flag.NewFlagSet(brand.ID, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.BoolVar(&opts.showVersion, "version", false, "print the version and exit")

	if err := flags.Parse(args); err != nil {
		return options{}, fmt.Errorf("parse flags: %w", err)
	}

	if flags.NArg() > 0 {
		// A failed write to stderr cannot be reported anywhere else.
		_, _ = fmt.Fprintf(stderr, "unexpected argument %q\n", flags.Arg(0))
		flags.Usage()

		return options{}, fmt.Errorf("unexpected argument %q", flags.Arg(0))
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
