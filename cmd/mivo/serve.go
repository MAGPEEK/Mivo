package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/adapters/fs/libraryfs"
	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/adapters/uuidgen"
	appsystem "github.com/magpeek/mivo/internal/app/system"
	"github.com/magpeek/mivo/internal/platform/brand"
	"github.com/magpeek/mivo/internal/platform/buildinfo"
	"github.com/magpeek/mivo/internal/platform/config"
	"github.com/magpeek/mivo/internal/platform/instancelock"
	"github.com/magpeek/mivo/internal/platform/logging"
	"github.com/magpeek/mivo/internal/platform/paths"
)

const (
	// databaseFileName is the SQLite file inside the data directory (ADR 0004).
	databaseFileName = "server.db"

	// closeDatabaseStep names the cleanup step in the error when closing the database fails.
	closeDatabaseStep = "close database"

	// releaseLockStep names the cleanup step in the error when releasing the data directory
	// lock fails.
	releaseLockStep = "release data directory lock"

	// shutdownBudget is how long the whole shutdown may take after SIGTERM: Docker's
	// --stop-timeout 20 and the Synology package's stop script both kill the server after
	// 20 s, so the bounded steps (HTTP, playback, scans) plus databaseCloseReserve stay below
	// it with two seconds to spare.
	shutdownBudget = 18 * time.Second

	// databaseCloseReserve is what the budget keeps for closing the database and releasing
	// the lock; both are local file operations that finish in milliseconds.
	databaseCloseReserve = time.Second
)

// application is the configured server: everything the startup steps after loading the
// configuration share.
type application struct {
	config config.Config
	logger *slog.Logger
	build  buildinfo.Info
	hooks  runHooks
	// tools resolves tools.ffmpeg and tools.ffprobe.
	tools toolHost
}

// serve loads the configuration, builds the logger and runs the server until ctx is done.
//
// Startup order: config → logger → directories → data directory lock → database and
// migrations → server identity → FFmpeg tools and capabilities → library sync → scan service →
// playback service and workspace purge → HTTP listener → startup scan → serve. Shutdown runs
// backwards: HTTP server → playback sessions → scans → database → lock. Every step closes
// what it opened when a later one fails, and every shutdown step is bounded.
func serve(ctx context.Context, opts options, env runEnv, hooks runHooks) error {
	cfg, err := config.Load(config.Options{
		ConfigDirFlag: opts.configDir,
		Environment:   paths.HostEnvironment(env.lookupEnv),
	})
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	logger, err := logging.New(logging.Options{Level: cfg.Log.Level, Format: cfg.Log.Format, Writer: env.stderr})
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}

	app := application{
		config: cfg, logger: logger, build: buildinfo.Read(), hooks: hooks,
		tools: toolHost{goos: runtime.GOOS, lookPath: exec.LookPath},
	}

	return app.start(ctx)
}

// start creates the directories, locks the data directory and serves from it. The lock is
// released last, after the database is closed; a release error is reported with the result.
func (app application) start(ctx context.Context) error {
	directories := app.config.Paths

	err := paths.Ensure(directories.Data, directories.Cache, directories.Transcode)
	if err != nil {
		return fmt.Errorf("prepare directories: %w", err)
	}

	lock, err := lockDataDirectory(directories.Data)
	if err != nil {
		return err
	}

	// Logged after the lock, so a refused second start writes only its error line.
	app.logger.InfoContext(ctx, "starting "+brand.DisplayName,
		slog.String("version", app.build.Summary()),
		slog.String("config_dir", app.config.ConfigDir),
		slog.String("data_dir", directories.Data))

	serveErr := app.openAndServe(ctx)
	result := withCleanupError(ctx, serveErr, releaseLockStep, lock.Release())

	if result == nil {
		app.logger.InfoContext(ctx, brand.DisplayName+" stopped")
	}

	return result
}

// lockDataDirectory keeps a second server off the data directory before it can race the
// migrations or purge this server's transcode sessions (ADR 0024). The lock's own error names
// the lock file inside dataDir, so only the sentinel is kept to name the path once.
func lockDataDirectory(dataDir string) (*instancelock.Lock, error) {
	lock, err := instancelock.Acquire(dataDir)
	if errors.Is(err, instancelock.ErrLocked) {
		return nil, fmt.Errorf("another server is already using %s: %w", dataDir, instancelock.ErrLocked)
	}

	if err != nil {
		return nil, fmt.Errorf("lock data directory: %w", err)
	}

	return lock, nil
}

// openAndServe opens the database and serves. The database is closed on every path once it
// is open; a close error is reported with the result.
func (app application) openAndServe(ctx context.Context) error {
	db, err := app.openDatabase(ctx)
	if err != nil {
		return err
	}

	serveErr := app.serveWith(ctx, db)

	return withCleanupError(ctx, serveErr, closeDatabaseStep, app.closeDatabase(ctx, db))
}

// openDatabase opens and migrates <data>/server.db.
func (app application) openDatabase(ctx context.Context) (*sqlite.DB, error) {
	path := filepath.Join(app.config.Paths.Data, databaseFileName)

	db, err := sqlite.Open(ctx, sqlite.Options{Path: path, Logger: app.logger})
	if err != nil {
		return nil, fmt.Errorf("start database %s: %w", path, err)
	}

	err = db.Migrate(ctx)
	if err != nil {
		return nil, withCleanupError(ctx, fmt.Errorf("start database %s: %w", path, err), closeDatabaseStep, db.Close())
	}

	return db, nil
}

func (app application) closeDatabase(ctx context.Context, db *sqlite.DB) error {
	if err := db.Close(); err != nil {
		// withCleanupError names the step.
		return err
	}

	app.logger.DebugContext(ctx, "database closed")

	return nil
}

// withCleanupError adds the error of a cleanup step, such as closing the database, to result,
// the outcome of the work before. run treats every error that wraps the shutdown request as a
// clean stop, so when result is only that request it is dropped and the cleanup error
// returned alone; joined, the cleanup failure would be swallowed as part of the cancellation.
func withCleanupError(ctx context.Context, result error, step string, cleanupErr error) error {
	if cleanupErr == nil {
		return result
	}

	cleanupErr = fmt.Errorf("%s: %w", step, cleanupErr)

	if result == nil || stoppedWhileStarting(ctx, result) {
		return cleanupErr
	}

	return errors.Join(result, cleanupErr)
}

// serveWith wires the use cases on the open database and serves until ctx is done. The scan
// service is closed after everything that serves requests or plays has stopped.
func (app application) serveWith(ctx context.Context, db *sqlite.DB) error {
	system, err := app.newSystemService(ctx, db)
	if err != nil {
		return err
	}

	tools, err := app.prepareTools(ctx)
	if err != nil {
		return err
	}

	files, err := libraryfs.New(app.logger)
	if err != nil {
		return fmt.Errorf("create library file system: %w", err)
	}

	libraries, err := app.newLibraryService(ctx, db, files)
	if err != nil {
		return err
	}

	prober := ffmpeg.NewProber(tools.ffprobe)

	scans, err := app.newScanService(db, files, prober)
	if err != nil {
		return err
	}

	catalog := catalogServices{system: system, libraries: libraries, scans: scans}
	serveErr := app.serveWithPlayback(ctx, db, catalog, playbackAdapters{files: files, prober: prober, tools: tools})

	return withCleanupError(ctx, serveErr, closeScansStep, app.closeScans(ctx, scans))
}

// serveWithPlayback starts the playback service and serves; the playback sessions are stopped
// after the HTTP server, so no request can start one meanwhile.
func (app application) serveWithPlayback(ctx context.Context, db *sqlite.DB, catalog catalogServices, adapters playbackAdapters) error {
	workspace, err := app.newWorkspace()
	if err != nil {
		return err
	}

	adapters.workspace = workspace

	playback, err := app.newPlaybackService(ctx, db, adapters)
	if err != nil {
		return err
	}

	streams, err := app.newStreamHandlers(playback, adapters)
	if err != nil {
		return withCleanupError(ctx, err, closePlaybackStep, app.closePlayback(ctx, playback))
	}

	serveErr := app.serveHTTP(ctx, wiredServices{catalogServices: catalog, playback: playback, streams: streams})

	return withCleanupError(ctx, serveErr, closePlaybackStep, app.closePlayback(ctx, playback))
}

// newSystemService builds the system use cases and ensures the server identity, so the
// first start creates it before any client asks.
func (app application) newSystemService(ctx context.Context, db *sqlite.DB) (*appsystem.Service, error) {
	service, err := appsystem.NewService(appsystem.Dependencies{
		Identities: sqlite.NewIdentityRepository(db),
		Database:   db,
		IDs:        uuidgen.New(),
		Clock:      time.Now,
		// app/system has its own BuildInfo because app must not import platform (ADR 0003).
		Build: appsystem.BuildInfo{
			Version:   app.build.Version,
			Commit:    app.build.Commit,
			BuildTime: app.build.CommitTime,
		},
		ProductName: brand.DisplayName,
	})
	if err != nil {
		return nil, fmt.Errorf("create system service: %w", err)
	}

	identity, err := service.EnsureIdentity(ctx)
	if err != nil {
		return nil, fmt.Errorf("start system service: %w", err)
	}

	app.logger.InfoContext(ctx, "server identity ready", slog.String("server_id", identity.ID.String()))

	return service, nil
}
