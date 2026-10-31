package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/magpeek/mivo/packaging/synology/internal/spkmeta"
)

// sourceDateEpochVariable is the reproducible-builds variable that supplies the file time when
// -source-date-epoch is not given (https://reproducible-builds.org/specs/source-date-epoch/).
const sourceDateEpochVariable = "SOURCE_DATE_EPOCH"

// unsetEpoch marks -source-date-epoch as not given on the command line.
const unsetEpoch = -1

// errMissingFlag reports a required flag that was not given.
var errMissingFlag = errors.New("missing required flag")

// errInvalidFlag reports a flag value outside its range.
var errInvalidFlag = errors.New("invalid flag value")

// options are the parsed command line.
type options struct {
	family          string
	version         string
	binary          string
	ffmpegDir       string
	out             string
	repo            string
	packageRevision int
	modTime         time.Time
	force           bool
}

// lookupEnv reads an environment variable; tests pass a map instead of os.LookupEnv.
type lookupEnv func(name string) (string, bool)

// run parses the command line, stages the project with the product's package values and
// prints a summary. It is everything main does apart from the exit status.
func run(args []string, stdout, stderr io.Writer) error {
	opts, err := parseOptions(args, stderr, os.LookupEnv)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}

	if err != nil {
		return err
	}

	staged, err := stage(opts, spkmeta.DefaultPackage(), stderr)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(stdout, "staged the %s project (%d files, DSM version %s) in %s\n",
		opts.family, staged.files, staged.dsmVersion, opts.out)
	if err != nil {
		return fmt.Errorf("write summary: %w", err)
	}

	return nil
}

func parseOptions(args []string, stderr io.Writer, env lookupEnv) (options, error) {
	flags := flag.NewFlagSet("spkstage", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var (
		opts  options
		epoch int64
	)

	flags.StringVar(&opts.family, "family", "", "architecture family: x86_64, armv8 or armv7 (required)")
	flags.StringVar(&opts.version, "version", "", "server version, for example 0.1.0-alpha.2 (required)")
	flags.StringVar(&opts.binary, "binary", "", "static linux server binary of the family (required)")
	flags.StringVar(&opts.ffmpegDir, "ffmpeg-dir", "", "directory written by ffmpeg/fetch.sh for the family (required)")
	flags.StringVar(&opts.out, "out", "", "project directory to write; must be missing or empty unless -force (required)")
	flags.StringVar(&opts.repo, "repo", ".", "repository root that holds LICENSE and packaging/synology/package")
	flags.IntVar(&opts.packageRevision, "package-revision", 0, "packaging-only revision of the same server version (0-9)")
	flags.Int64Var(&epoch, "source-date-epoch", unsetEpoch, "file time in seconds since 1970 (default $"+sourceDateEpochVariable+", else 0)")
	flags.BoolVar(&opts.force, "force", false, "replace a non-empty project directory")

	if err := flags.Parse(args); err != nil {
		return options{}, fmt.Errorf("parse command line: %w", err)
	}

	if flags.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected argument %q: %w", flags.Arg(0), errInvalidFlag)
	}

	if err := requireFlags(opts); err != nil {
		return options{}, err
	}

	modTime, err := resolveModTime(epoch, env)
	if err != nil {
		return options{}, err
	}

	opts.modTime = modTime

	return opts, nil
}

func requireFlags(opts options) error {
	required := []struct{ name, value string }{
		{name: "family", value: opts.family},
		{name: "version", value: opts.version},
		{name: "binary", value: opts.binary},
		{name: "ffmpeg-dir", value: opts.ffmpegDir},
		{name: "out", value: opts.out},
		{name: "repo", value: opts.repo},
	}

	for _, entry := range required {
		if entry.value == "" {
			return fmt.Errorf("-%s: %w", entry.name, errMissingFlag)
		}
	}

	return nil
}

// resolveModTime prefers the flag, then SOURCE_DATE_EPOCH, then the Unix epoch, so a build
// without either is still reproducible.
func resolveModTime(epoch int64, env lookupEnv) (time.Time, error) {
	source := "-source-date-epoch"

	if epoch == unsetEpoch {
		epoch = 0

		if text, found := env(sourceDateEpochVariable); found && text != "" {
			parsed, err := strconv.ParseInt(text, 10, 64)
			if err != nil {
				return time.Time{}, fmt.Errorf("%s %q is not a number: %w", sourceDateEpochVariable, text, errInvalidFlag)
			}

			epoch = parsed
			source = sourceDateEpochVariable
		}
	}

	if epoch < 0 {
		return time.Time{}, fmt.Errorf("%s %d is negative: %w", source, epoch, errInvalidFlag)
	}

	return time.Unix(epoch, 0).UTC(), nil
}
