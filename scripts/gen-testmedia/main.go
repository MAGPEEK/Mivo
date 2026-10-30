// Command gen-testmedia generates the rights-free test media corpus (ADR 0021).
//
// It builds short clips from FFmpeg lavfi sources in every container and codec the local
// FFmpeg can produce, skips cases it cannot build and writes <out>/manifest.json, which lists
// every case with its status and expected properties. Run it from the repository root:
//
//	go run ./scripts/gen-testmedia [-out testdata/media] [-ffmpeg ffmpeg] [-force]
//	                               [-only <glob>] [-jobs N]
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

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)

	stop()
	os.Exit(code)
}

// options holds the parsed command line.
type options struct {
	outDir   string
	ffmpeg   string
	force    bool
	onlyGlob string
	jobs     int
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

	flags := flag.NewFlagSet("gen-testmedia", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.outDir, "out", defaultOutDir, "output directory for media files and manifest.json")
	flags.StringVar(&opts.ffmpeg, "ffmpeg", defaultFFmpeg, "ffmpeg executable (name on PATH or path)")
	flags.BoolVar(&opts.force, "force", false, "regenerate files that already exist")
	flags.StringVar(&opts.onlyGlob, "only", "", "only build cases whose name matches this glob, e.g. 'video-mkv-*'")
	flags.IntVar(&opts.jobs, "jobs", runtime.NumCPU(), "number of parallel ffmpeg processes (minimum 1)")

	if err := flags.Parse(args); err != nil {
		return options{}, fmt.Errorf("parse flags: %w", err)
	}

	if flags.NArg() > 0 {
		return options{}, usageError(flags, fmt.Errorf("unexpected argument %q", flags.Arg(0)))
	}

	if err := validateFilter(opts.onlyGlob); err != nil {
		return options{}, usageError(flags, err)
	}

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
	if err != nil {
		return fmt.Errorf("use ffmpeg %q (install FFmpeg or pass -ffmpeg): %w", opts.ffmpeg, err)
	}

	gen := generator{ffmpeg: ffmpeg, outDir: opts.outDir, force: opts.force, jobs: opts.jobs, progress: stdout}
	entries := gen.generateAll(ctx, planCases(allCases(), caps, opts.onlyGlob))
	result := newManifest(caps.version, entries)

	if err := writeManifest(opts.outDir, result); err != nil {
		return err
	}

	// The summary is informational; the manifest is the authoritative result.
	_, _ = fmt.Fprintf(stdout, "ffmpeg %s: %d generated, %d skipped, %d failed; manifest: %s\n",
		result.FFmpegVersion, result.count(statusGenerated), result.count(statusSkipped),
		result.count(statusFailed), filepath.Join(opts.outDir, manifestFileName))

	if result.count(statusFailed) > 0 {
		return errCasesFailed
	}

	return nil
}
