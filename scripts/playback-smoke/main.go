// Command playback-smoke checks a running server against the playback expectations of the
// generated test corpus (ADR 0025). It is the release gate for the FFmpeg of the Docker
// image: `go test` only covers the FFmpeg of the machine it runs on.
//
// It triggers a scan of every library, waits for it, and then plays every generated video
// case of the corpus manifest, plus the seek cases of the profile, through the public API
// only: it finds the item by file name, creates a session with the reference profile,
// compares mode, stream actions, reasons and warnings with internal/adapters/corpustest,
// downloads the stream (direct play: one range request, 206; HLS: the playlist until
// EXT-X-ENDLIST, the init segment and every media segment), probes it with the local
// ffprobe and deletes the session. It prints one line per case and a summary and exits
// with 1 when any case does not match. Run it from the repository root:
//
//	go run ./scripts/playback-smoke -base http://localhost:8460 [-manifest testdata/media/manifest.json]
//	                                [-profile chromium-desktop] [-ffprobe ffprobe]
//	                                [-case-timeout 2m] [-scan-timeout 5m]
//	                                [-disc-manifest testdata/library/manifest.json]
//	                                [-user smoke -password-env MIVO_SMOKE_PASSWORD]
//
// It first reads GET /api/v1/setup (ADR 0032). A fresh server gets its first administrator
// through the first-run setup, the -user account or a generated one; a server with accounts
// needs -user, whose password comes from the environment variable -password-env names so it
// never shows up in the process list. Both sign in as client "cli" and every API and media
// request carries the bearer token; the run signs out at the end. A server without the
// endpoint (404 before M7-01, 501 while the account handlers are stubs) is used without
// sign-in.
//
// The server must serve the same corpus, for example the Docker image with
// testdata/media mounted at /media (packaging/docker/README.md). With -disc-manifest it also
// plays the clear DVD folder of the disc corpus (testdata/library/discs, M3-16k), which the
// server must serve as well.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

const (
	exitOK = 0
	// exitFailure signals a mismatch or a server, corpus or tool that could not be used.
	exitFailure = 1
	// exitUsage matches the exit code of the flag package for invalid command lines.
	exitUsage = 2

	defaultManifest    = "testdata/media/manifest.json"
	defaultProfile     = "chromium-desktop"
	defaultFFprobe     = "ffprobe"
	defaultCaseTimeout = 2 * time.Minute
	defaultScanTimeout = 5 * time.Minute
	// requestTimeout bounds one HTTP request; the largest download is a 30-second clip.
	requestTimeout = time.Minute
	// pollInterval is the pause between polls of a scan or an unfinished playlist.
	pollInterval = 250 * time.Millisecond
	// maxRetryAfter caps the server's Retry-After hints (stream_not_ready, session limit).
	maxRetryAfter = 2 * time.Second
)

var (
	errInvalidBase   = errors.New("-base must be an absolute http or https URL, e.g. http://localhost:8460")
	errAccountFlags  = errors.New("-user and -password-env go together")
	errPasswordUnset = errors.New("the -password-env variable is unset or empty")
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)

	stop()
	os.Exit(code)
}

// options holds the parsed command line.
type options struct {
	base     *url.URL
	manifest string
	// discManifest is the library manifest whose clear DVD is played too; empty: no disc.
	discManifest string
	profile      string
	ffprobe      string
	caseTimeout  time.Duration
	scanTimeout  time.Duration
	// account signs in on a server with accounts or names the first administrator of a
	// fresh one; nil: a generated administrator on a fresh server, anonymous otherwise.
	account *credentials
}

// run executes the command with the given arguments and returns the process exit code.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	opts, err := parseOptions(args, stderr, os.LookupEnv)
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}

	if err != nil {
		return exitUsage
	}

	executable, err := exec.LookPath(opts.ffprobe)
	if err != nil {
		// A failed write to stderr cannot be reported anywhere else.
		_, _ = fmt.Fprintf(stderr, "playback-smoke: ffprobe %q not found; install FFmpeg (docs/development.md) or pass -ffprobe: %v\n",
			opts.ffprobe, err)

		return exitFailure
	}

	dependencies := smokeDependencies{
		httpClient: &http.Client{Timeout: requestTimeout},
		prober:     ffprobe{executable: executable},
		poll:       pollSettings{interval: pollInterval, maxRetryAfter: maxRetryAfter},
		workDir:    os.TempDir(),
	}

	if err := smoke(ctx, opts, dependencies, stdout); err != nil {
		_, _ = fmt.Fprintf(stderr, "playback-smoke: %v\n", err)

		return exitFailure
	}

	return exitOK
}

// lookupEnv reads an environment variable, like os.LookupEnv.
type lookupEnv func(name string) (string, bool)

func parseOptions(args []string, stderr io.Writer, lookup lookupEnv) (options, error) {
	opts := options{}

	var base, user, passwordEnv string

	flags := flag.NewFlagSet("playback-smoke", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&base, "base", "", "base URL of the running server, e.g. http://localhost:8460 (required)")
	flags.StringVar(&opts.manifest, "manifest", defaultManifest, "corpus manifest of the files the server serves")
	flags.StringVar(&opts.discManifest, "disc-manifest", "",
		"library manifest (testdata/library/manifest.json) whose clear DVD the server also serves and the smoke plays; empty: no disc case")
	flags.StringVar(&opts.profile, "profile", defaultProfile, "reference client profile: chromium-desktop or firefox-desktop")
	flags.StringVar(&opts.ffprobe, "ffprobe", defaultFFprobe, "local ffprobe executable (name on PATH or path)")
	flags.DurationVar(&opts.caseTimeout, "case-timeout", defaultCaseTimeout, "time limit of one case")
	flags.DurationVar(&opts.scanTimeout, "scan-timeout", defaultScanTimeout, "time limit of the library scan")
	flags.StringVar(&user, "user", "",
		"account to sign in with, or to create by the first-run setup of a fresh server; empty: a generated administrator on a fresh server")
	flags.StringVar(&passwordEnv, "password-env", "",
		"name of the environment variable that holds the password of -user (never pass the password itself)")

	if err := flags.Parse(args); err != nil {
		return options{}, fmt.Errorf("parse flags: %w", err)
	}

	if flags.NArg() > 0 {
		return options{}, usageError(flags, fmt.Errorf("unexpected argument %q", flags.Arg(0)))
	}

	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return options{}, usageError(flags, errInvalidBase)
	}

	if opts.caseTimeout <= 0 || opts.scanTimeout <= 0 {
		return options{}, usageError(flags, errors.New("-case-timeout and -scan-timeout must be positive"))
	}

	account, err := accountFromFlags(user, passwordEnv, lookup)
	if err != nil {
		return options{}, usageError(flags, err)
	}

	opts.base, opts.account = parsed, account

	return opts, nil
}

// accountFromFlags reads the password of -user from the variable -password-env names, so the
// password never shows up in the process list. Both flags empty: no account (nil).
func accountFromFlags(user, passwordEnv string, lookup lookupEnv) (*credentials, error) {
	if user == "" && passwordEnv == "" {
		return nil, nil //nolint:nilnil // no account is a valid result: the smoke signs in only when the server needs it
	}

	if user == "" || passwordEnv == "" {
		return nil, errAccountFlags
	}

	password, found := lookup(passwordEnv)
	if !found || password == "" {
		return nil, fmt.Errorf("%w: %s", errPasswordUnset, passwordEnv)
	}

	return &credentials{name: user, password: password}, nil
}

// usageError prints the problem and the usage, like the flag package does for unknown flags.
func usageError(flags *flag.FlagSet, err error) error {
	_, _ = fmt.Fprintf(flags.Output(), "%v\n", err)
	flags.Usage()

	return err
}
