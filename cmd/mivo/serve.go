package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/http/server"
	"github.com/magpeek/mivo/internal/adapters/http/webui"
	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/adapters/uuidgen"
	appsystem "github.com/magpeek/mivo/internal/app/system"
	"github.com/magpeek/mivo/internal/platform/brand"
	"github.com/magpeek/mivo/internal/platform/buildinfo"
	"github.com/magpeek/mivo/internal/platform/config"
	"github.com/magpeek/mivo/internal/platform/logging"
	"github.com/magpeek/mivo/internal/platform/paths"
)

const (
	// databaseFileName is the SQLite file inside the data directory (ADR 0004).
	databaseFileName = "server.db"

	// listenNetwork is the network of the HTTP listener.
	listenNetwork = "tcp"
)

// application is the configured server: everything the startup steps after loading the
// configuration share.
type application struct {
	config config.Config
	logger *slog.Logger
	build  buildinfo.Info
	hooks  runHooks
}

// serve loads the configuration, builds the logger and runs the server until ctx is done.
// The startup order is config → logger → directories → database → identity → HTTP; every
// step closes what it opened when a later one fails.
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

	app := application{config: cfg, logger: logger, build: buildinfo.Read(), hooks: hooks}

	return app.start(ctx)
}

// start creates the directories, opens the database and serves. The database is closed on
// every path once it is open; a close error is reported with the result.
func (app application) start(ctx context.Context) error {
	app.logger.InfoContext(ctx, "starting "+brand.DisplayName,
		slog.String("version", app.build.Summary()),
		slog.String("config_dir", app.config.ConfigDir),
		slog.String("data_dir", app.config.Paths.Data))

	directories := app.config.Paths

	err := paths.Ensure(directories.Data, directories.Cache, directories.Transcode)
	if err != nil {
		return fmt.Errorf("prepare directories: %w", err)
	}

	db, err := app.openDatabase(ctx)
	if err != nil {
		return err
	}

	serveErr := app.serveWith(ctx, db)

	return withCloseError(ctx, serveErr, db.Close())
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
		return nil, withCloseError(ctx, fmt.Errorf("start database %s: %w", path, err), db.Close())
	}

	return db, nil
}

// withCloseError adds the error of closing the database to result, the outcome of the work
// before. run treats every error that wraps the shutdown request as a clean stop, so when
// result is only that request it is dropped and the close error returned alone; joined, the
// close failure would be swallowed as part of the cancellation.
func withCloseError(ctx context.Context, result, closeErr error) error {
	if closeErr == nil {
		return result
	}

	closeErr = fmt.Errorf("close database: %w", closeErr)

	if result == nil || stoppedWhileStarting(ctx, result) {
		return closeErr
	}

	return errors.Join(result, closeErr)
}

// serveWith wires the use cases and the HTTP adapter on the open database and serves until
// ctx is done.
func (app application) serveWith(ctx context.Context, db *sqlite.DB) error {
	handlers, err := app.newHTTPHandlers(ctx, db)
	if err != nil {
		return err
	}

	listener, err := app.listen(ctx)
	if err != nil {
		return err
	}

	httpServer, err := server.New(server.Options{
		Listener:   listener,
		Logger:     app.logger,
		APIHandler: handlers.api,
		Fallback:   handlers.webUI,
	})
	if err != nil {
		return errors.Join(fmt.Errorf("create http server: %w", err), listener.Close())
	}

	app.hooks.reportListening(listener.Addr())

	err = httpServer.Run(ctx)
	if err != nil {
		return fmt.Errorf("run http server: %w", err)
	}

	app.logger.InfoContext(ctx, brand.DisplayName+" stopped")

	return nil
}

// httpHandlers are the handlers the HTTP server routes between: the REST API below /api/ and
// the web UI for every other path.
type httpHandlers struct {
	api   http.Handler
	webUI http.Handler
}

func (app application) newHTTPHandlers(ctx context.Context, db *sqlite.DB) (httpHandlers, error) {
	service, err := app.newSystemService(ctx, db)
	if err != nil {
		return httpHandlers{}, err
	}

	apiHandler, err := restapi.NewHandler(restapi.Options{System: service, Logger: app.logger})
	if err != nil {
		return httpHandlers{}, fmt.Errorf("create api handler: %w", err)
	}

	webUIHandler, err := app.newWebUIHandler(ctx)
	if err != nil {
		return httpHandlers{}, err
	}

	return httpHandlers{api: apiHandler, webUI: webUIHandler}, nil
}

// newWebUIHandler serves the web app embedded with the webui build tag, or the page that
// explains how to build it (ADR 0020).
func (app application) newWebUIHandler(ctx context.Context) (*webui.Handler, error) {
	files, err := webui.Files()
	if err != nil {
		return nil, fmt.Errorf("load web ui: %w", err)
	}

	handler, err := webui.NewHandler(files, app.logger)
	if err != nil {
		return nil, fmt.Errorf("create web ui handler: %w", err)
	}

	if !webui.Embedded {
		// Normal during development, where Vite serves the app; worth a line for operators
		// who expected the app in this binary.
		app.logger.InfoContext(ctx, "web app not embedded, serving build instructions instead")
	}

	return handler, nil
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

// listen binds the configured address here rather than in the server, so the bound address
// (with port 0 chosen by the OS) is known before serving starts.
func (app application) listen(ctx context.Context) (net.Listener, error) {
	address := net.JoinHostPort(app.config.Server.Host, strconv.Itoa(app.config.Server.Port))

	var listenConfig net.ListenConfig

	listener, err := listenConfig.Listen(ctx, listenNetwork, address)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", address, err)
	}

	return listener, nil
}
