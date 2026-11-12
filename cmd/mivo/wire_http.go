package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strconv"

	imagehttp "github.com/magpeek/mivo/internal/adapters/http/images"
	"github.com/magpeek/mivo/internal/adapters/http/posterproxy"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/http/server"
	"github.com/magpeek/mivo/internal/adapters/http/stream"
	"github.com/magpeek/mivo/internal/adapters/http/webui"
	"github.com/magpeek/mivo/internal/adapters/sqlite"
	appactivity "github.com/magpeek/mivo/internal/app/activity"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	approws "github.com/magpeek/mivo/internal/app/rows"
	appscan "github.com/magpeek/mivo/internal/app/scan"
	appsearch "github.com/magpeek/mivo/internal/app/search"
	appsystem "github.com/magpeek/mivo/internal/app/system"
	appwatch "github.com/magpeek/mivo/internal/app/watch"
)

// listenNetwork is the network of the HTTP listener.
const listenNetwork = "tcp"

// catalogServices are the use cases of the system, library and scan endpoints.
type catalogServices struct {
	system    *appsystem.Service
	libraries *applibrary.Service
	// managed reads the libraries for the library endpoints; libraries serves the items.
	managed *applibrary.ManagedCatalog
	scans   *appscan.Service
	// images serves artwork; trigger keeps the libraries current after the startup scan.
	images  imageServices
	trigger runner
	// management creates, changes and removes libraries and browses folders.
	management management
	// store is the one library store (newLibraryStore); rows serves the Home and Overview
	// rows and search the catalog search; both are built from the others by serveWithPlayback.
	store  *sqlite.ManagedLibraryStore
	rows   *approws.Service
	search *appsearch.Service
	// online is the online metadata feature: its API services, the enrichment worker and
	// the periodic refresh.
	online onlineMetadata
	// watch is the watch state; the rows and the item responses read it (built by
	// withCatalogReaders).
	watch *appwatch.Service
	// accounts are sign-in and user administration; every request goes through them.
	accounts accountServices
}

// wiredServices is everything the HTTP adapter serves.
type wiredServices struct {
	catalogServices

	playback *appplayback.Service
	streams  *stream.Handlers
	// activity joins the running scans and sessions for the administrator's activity view.
	activity *appactivity.Service
	// streaming are the hardware status, subtitle and trickplay services of the streaming
	// extension; the session list, transcoder and trickplay endpoints answer through them.
	streaming streamingServices
}

// httpHandlers are the handlers the HTTP server routes between: the REST API below /api/ and
// the web UI for every other path.
type httpHandlers struct {
	api   http.Handler
	webUI http.Handler
}

// mounts are the extra API routes: the stream delivery, the image endpoint and the
// candidate posters of the identify dialog.
func (services wiredServices) mounts() []restapi.Mount {
	return slices.Concat(services.streams.Mounts(), services.images.handlers.Mounts(), services.online.posters.Mounts())
}

// serveHTTP binds the listener, starts the startup scan and serves until ctx is done. The
// scan starts only once the listener is bound, so a port conflict fails the start before any
// ffprobe runs.
func (app application) serveHTTP(ctx context.Context, services wiredServices) error {
	handlers, err := app.newHTTPHandlers(ctx, services)
	if err != nil {
		return err
	}

	listener, err := app.listen(ctx)
	if err != nil {
		return err
	}

	httpServer, err := server.New(app.serverOptions(listener, handlers, services.accounts))
	if err != nil {
		return errors.Join(fmt.Errorf("create http server: %w", err), listener.Close())
	}

	app.hooks.reportListening(listener.Addr())
	app.startLibraryScans(ctx, services.scans)

	// The trigger starts scans, so it stops before the caller closes the scan service; the
	// enrichment worker, the periodic refresh and the session purge stop before the database
	// closes. A stopped worker gives its running jobs back to the queue, so a restart resumes
	// them.
	stopTrigger := startBackground(ctx, services.trigger)
	stopWorker := startBackground(ctx, services.online.worker)
	stopPeriodic := startBackground(ctx, services.online.periodic)
	stopSessionPurge := startBackground(ctx, sessionPurger{
		purge: services.accounts.auth.PurgeExpired, interval: sessionPurgeInterval, logger: app.logger,
	})
	runErr := httpServer.Run(ctx)

	stopTrigger()
	stopPeriodic()
	stopWorker()
	stopSessionPurge()

	if runErr != nil {
		return fmt.Errorf("run http server: %w", runErr)
	}

	return nil
}

// serverOptions quiets the stream delivery routes, whose segment polling would flood the info
// log, but not the session resource itself: a DELETE of a session stays at info. The LAN-only
// guard is an opt-out, so the zero value keeps it on (OD-10); nil interface prefixes mean
// this host's interfaces. Every request goes through the session middleware of accounts, and
// forwarding headers count only from the configured trusted proxies (ADR 0032 §3, §9); the
// default trusts nobody.
func (app application) serverOptions(listener net.Listener, handlers httpHandlers, accounts accountServices) server.Options {
	return server.Options{
		Listener:           listener,
		Logger:             app.logger,
		APIHandler:         handlers.api,
		Fallback:           handlers.webUI,
		QuietPatterns:      slices.Concat(stream.QuietPatterns(), imagehttp.QuietPatterns(), posterproxy.QuietPatterns()),
		AllowPublicClients: !app.config.Server.LANOnly,
		Authentication:     accounts.authentication(),
		TrustedProxies:     app.config.Server.TrustedProxies,
	}
}

func (app application) newHTTPHandlers(ctx context.Context, services wiredServices) (httpHandlers, error) {
	apiHandler, err := restapi.NewHandler(restapi.Options{
		System:    services.system,
		Libraries: services.managed,
		Items:     services.libraries,
		Rows:      services.rowsService(),
		Search:    services.searchService(),
		Scan:      services.scans,
		Playback:  services.playback,
		// The scan service owns the probe step, so it also re-analyzes single items.
		Reanalysis: services.scans,
		// The manager also serves the folder browser: it marks the folders that cannot be chosen.
		Manager:      services.management.manager,
		Folders:      services.management.manager,
		Install:      services.management.install,
		LibraryKinds: enabledLibraryKinds(),
		Logger:       app.logger,
		Mounts:       services.mounts(),
		Metadata:     services.online.apiServices(),
		Activity:     services.activity,
		Auth:         services.accounts.auth,
		Users:        services.accounts.userServices(),
		Watch:        services.watch,
		Streaming:    services.streaming.api(),
	})
	if err != nil {
		return httpHandlers{}, fmt.Errorf("create api handler: %w", err)
	}

	webUIHandler, err := app.newWebUIHandler(ctx)
	if err != nil {
		return httpHandlers{}, err
	}

	return httpHandlers{api: apiHandler, webUI: webUIHandler}, nil
}

// rowsService returns the rows service as the API option. A nil *approws.Service must not
// become a non-nil interface, or the API would call it instead of answering 501.
func (services wiredServices) rowsService() restapi.RowsService {
	if services.rows == nil {
		return nil
	}

	return services.rows
}

// searchService returns the search service as the API option; nil stays nil like rowsService.
func (services wiredServices) searchService() restapi.SearchService {
	if services.search == nil {
		return nil
	}

	return services.search
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

// listen binds the configured address here rather than in the server, so the bound address
// (with port 0 chosen by the OS) is known before serving starts. Tests may wrap the listener
// (runHooks.wrapListener).
func (app application) listen(ctx context.Context) (net.Listener, error) {
	address := net.JoinHostPort(app.config.Server.Host, strconv.Itoa(app.config.Server.Port))

	var listenConfig net.ListenConfig

	listener, err := listenConfig.Listen(ctx, listenNetwork, address)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", address, err)
	}

	return app.hooks.wrap(listener), nil
}
