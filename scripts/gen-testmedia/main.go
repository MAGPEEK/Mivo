// Command gen-testmedia generates the rights-free test media corpus (ADR 0021).
//
// It builds short video clips, audio files, images and subtitle files (sidecar, embedded and
// multi-track) from FFmpeg lavfi sources and text it writes itself, in every format the local
// FFmpeg can produce. It skips cases it cannot build and writes <out>/manifest.json, which
// lists every case with its status, expected properties and the known gaps FFmpeg cannot
// produce at all. Run it from the repository root:
//
//	go run ./scripts/gen-testmedia [-out testdata/media] [-library testdata/library]
//	                               [-ffmpeg ffmpeg] [-force] [-only <glob>[,<glob>...]] [-jobs N]
//
// # Library tree
//
// Unless -library is "", it also writes a library tree for the milestone-3 scan (task
// M3-17, ADR 0027): <library>/movies and <library>/shows with folder and loose movies,
// versions, editions, parts, extras, samples, shows with seasons, specials, multi-episode
// files, absolutely numbered episodes (UX DT-24: "Dragon Ball Super - 001" … "- 131"), Kodi
// NFO files (UTF-8, ISO-8859-1, URL-only, malformed) and local and embedded artwork. Names of
// real movies and shows are only file names; all content is generated. Every video is the
// same 2 s H.264/AAC clip; files of one item share one FFmpeg run and are byte copies. Library
// case names start with "library-", so -only 'library-*' builds the tree alone.
//
// <library>/manifest.json describes the expected catalog (what a scan must produce) and the
// build result. Paths are relative to the library root (Path of the library) unless noted,
// use forward slashes and keep the raw file-system form (one folder name is Unicode NFD);
// titles and NFO text are NFC. Schema version 1:
//
//	schemaVersion  1
//	ffmpegVersion  version of the FFmpeg that ran
//	libraries[]    kind ("movies"|"shows"), path (below <library>), items[]:
//	  kind           "movie"|"show"
//	  title, year    expected catalog values after the merge (NFO before file name); year
//	                 is omitted when there is none
//	  path           movie or show folder, file of a loose movie, "" for a show whose files
//	                 lie directly in the library root
//	  versions[]     movies: label (file-name suffix or edition, "" for none), parts[]
//	                 (files in part order)
//	  extras[]       type ("trailer"|"featurette"), file
//	  ignored[]      files the scan must not list (samples)
//	  nfo            file, format ("xml"|"url"|"malformed"), encoding ("UTF-8"|
//	                 "ISO-8859-1", XML only), fields: title, originalTitle, year, plot,
//	                 aired, season, episode, genres[], providerIds{tmdb,imdb,tvdb};
//	                 fields is empty for a malformed file
//	  artwork[]      kind ("poster"|"backdrop"|"thumb"|"logo"), source ("local"|
//	                 "embedded"), file (the image, or the video embedding it)
//	  seasons[]      shows, ascending number (0 = specials): number, path ("" without a
//	                 season folder), artwork[], episodes[]: number, numberEnd (multi-episode
//	                 file), absoluteNumber (absolutely numbered files), files[], nfo, artwork[]
//	cases[]        name, category ("artwork"|"movies"|"shows"), status ("generated"|
//	               "skipped"|"failed"), skipKind, reason, files[] (relative to <library>)
//
// The catalog is static; consumers check that every case is generated (or skip with the
// reason) before they compare a scan with it.
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
	// defaultLibraryDir stays apart from defaultOutDir: tests and the smoke script use
	// testdata/media as one movies library and would change their counts otherwise.
	defaultLibraryDir = "testdata/library"
	defaultFFmpeg     = "ffmpeg"
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
	// libraryDir receives the library tree; "" skips it.
	libraryDir string
	ffmpeg     string
	force      bool
	only       caseFilter
	jobs       int
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
	flags.StringVar(&opts.libraryDir, "library", defaultLibraryDir,
		"output directory for the library tree (movies/, shows/) and its manifest.json; \"\" skips it")
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

	run := session{opts: opts, caps: caps, ffmpeg: ffmpeg, stdout: stdout}

	// An interrupted run still writes the manifests, so they match the files on disk.
	media, err := run.generateMedia(ctx)
	if err != nil {
		return err
	}

	failed := media.count(statusFailed)

	if opts.libraryDir != "" {
		library, err := run.generateLibrary(ctx)
		if err != nil {
			return err
		}

		failed += library.count(statusFailed)
	}

	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", errInterrupted, ctx.Err())
	}

	if failed > 0 {
		return errCasesFailed
	}

	return nil
}

// session is one run of the program with detected FFmpeg capabilities.
type session struct {
	opts   options
	caps   capabilities
	ffmpeg commandRunner
	stdout io.Writer
}

// generateMedia builds the playback corpus into s.opts.outDir and writes its manifest and
// summary.
func (s session) generateMedia(ctx context.Context) (manifest, error) {
	gen := generator{
		ffmpeg: s.ffmpeg, outDir: s.opts.outDir, force: s.opts.force, jobs: s.opts.jobs, progress: s.stdout,
		previous: loadPreviousEncoders(s.opts.outDir, s.stdout),
	}
	result := newManifest(s.caps.version, gen.generateAll(ctx, planCases(allCases(), s.caps, s.opts.only)))

	if err := writeManifest(s.opts.outDir, result); err != nil {
		return manifest{}, err
	}

	// The summary is informational; the manifest is the authoritative result.
	_ = writeSummary(s.stdout, result, filepath.Join(s.opts.outDir, manifestFileName))

	return result, nil
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
