// Command gen-testmedia generates the rights-free test media corpus (ADR 0021).
//
// It builds short video clips, audio files, images and subtitle files (sidecar, embedded and
// multi-track) from FFmpeg lavfi sources and text it writes itself, in every format the local
// FFmpeg can produce. It skips cases it cannot build and writes <out>/manifest.json, which
// lists every case with its status, expected properties and the known gaps FFmpeg cannot
// produce at all. Run it from the repository root:
//
//	go run ./scripts/gen-testmedia [-out testdata/media] [-ffmpeg ffmpeg] [-force]
//	                               [-only <glob>[,<glob>...]] [-jobs N]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
)

const (
	exitOK = 0
	// exitFailure signals that a case failed or FFmpeg could not be used.
	exitFailure = 1
	// exitUsage matches the exit code of the flag package for invalid command lines.
	exitUsage = 2

	defaultOutDir = "testdata/media"
	defaultFFmpeg = "ffmpeg"
)

// errCasesFailed reports that at least one buildable case failed; details are in the manifest.
var errCasesFailed = errors.New("at least one case failed, see the manifest for reasons")

// errInterrupted reports a run stopped by Ctrl+C or SIGTERM; the manifest lists what was done.
var errInterrupted = errors.New("generation interrupted, run again to finish the corpus")

func main() {
	ctx, stop := interruptContext(context.Background())
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)

	stop()
	os.Exit(code)
}

// interruptContext returns a context that the first Ctrl+C or SIGTERM cancels. The run then
// stops its FFmpeg processes and still writes the manifest. Once the context is done, the
// signal handlers are released, so a second Ctrl+C ends the program at once with the default
// behaviour instead of being swallowed while the run winds down.
func interruptContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	context.AfterFunc(ctx, stop)

	return ctx, stop
}

// options holds the parsed command line.
type options struct {
	outDir string
	ffmpeg string
	force  bool
	only   caseFilter
	jobs   int
}

// run executes the command with the given arguments and returns the process exit code.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	opts, err := parseOptions(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}

	if err != nil {
		return exitUsage
	}

	if err := generateCorpus(ctx, opts, execRunner{executable: opts.ffmpeg}, stdout); err != nil {
		// A failed write to stderr cannot be reported anywhere else.
		_, _ = fmt.Fprintf(stderr, "gen-testmedia: %v\n", err)

		return exitFailure
	}

	return exitOK
}

func parseOptions(args []string, stderr io.Writer) (options, error) {
	opts := options{}

	var only string

	flags := flag.NewFlagSet("gen-testmedia", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.outDir, "out", defaultOutDir, "output directory for media files and manifest.json")
	flags.StringVar(&opts.ffmpeg, "ffmpeg", defaultFFmpeg, "ffmpeg executable (name on PATH or path)")
	flags.BoolVar(&opts.force, "force", false, "regenerate files that already exist")
	flags.StringVar(&only, "only", "",
		"only build cases whose name matches one of these comma-separated globs,\n"+
			"e.g. 'video-mkv-*' or 'video-mp4-*,video-webm-vp9-opus'")
	flags.IntVar(&opts.jobs, "jobs", runtime.NumCPU(), "number of parallel ffmpeg processes (minimum 1)")

	if err := flags.Parse(args); err != nil {
		return options{}, fmt.Errorf("parse flags: %w", err)
	}

	if flags.NArg() > 0 {
		return options{}, usageError(flags, fmt.Errorf("unexpected argument %q", flags.Arg(0)))
	}

	filter, err := parseCaseFilter(only)
	if err != nil {
		return options{}, usageError(flags, err)
	}

	opts.only = filter
	opts.jobs = max(opts.jobs, 1)

	return opts, nil
}

// usageError prints err and the usage text and returns err.
func usageError(flags *flag.FlagSet, err error) error {
	// A failed write to stderr cannot be reported anywhere else.
	_, _ = fmt.Fprintln(flags.Output(), err)
	flags.Usage()

	return err
}

// generateCorpus detects what FFmpeg can build, generates the cases and writes the manifest.
func generateCorpus(ctx context.Context, opts options, ffmpeg commandRunner, stdout io.Writer) error {
	caps, err := detectCapabilities(ctx, ffmpeg)
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", errInterrupted, ctx.Err())
	}

	if err != nil {
		return fmt.Errorf("use ffmpeg %q (install FFmpeg or pass -ffmpeg): %w", opts.ffmpeg, err)
	}

	gen := generator{
		ffmpeg: ffmpeg, outDir: opts.outDir, force: opts.force, jobs: opts.jobs, progress: stdout,
		previous: loadPreviousEncoders(opts.outDir, stdout),
	}
	entries := gen.generateAll(ctx, planCases(allCases(), caps, opts.only))
	result := newManifest(caps.version, entries)

	// An interrupted run still writes the manifest, so it matches the files on disk.
	if err := writeManifest(opts.outDir, result); err != nil {
		return err
	}

	// The summary is informational; the manifest is the authoritative result.
	_ = writeSummary(stdout, result, filepath.Join(opts.outDir, manifestFileName))

	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", errInterrupted, ctx.Err())
	}

	if result.count(statusFailed) > 0 {
		return errCasesFailed
	}

	return nil
}

// loadPreviousEncoders reads the encoders of the files the earlier runs built. An unusable
// manifest is not fatal: the files it described are still kept, just listed without
// encoders, and the note says why.
func loadPreviousEncoders(outDir string, progress io.Writer) previousEncoders {
	previous, err := readPreviousEncoders(outDir)
	if err != nil {
		// Progress output is informational; the manifest is the authoritative result.
		_, _ = fmt.Fprintf(progress, "note: ignoring the previous manifest, kept files are listed without encoders: %v\n", err)
	}

	return previous
}
