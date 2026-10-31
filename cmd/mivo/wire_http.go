package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/http/server"
	"github.com/magpeek/mivo/internal/adapters/http/stream"
	"github.com/magpeek/mivo/internal/adapters/http/webui"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	appscan "github.com/magpeek/mivo/internal/app/scan"
	appsystem "github.com/magpeek/mivo/internal/app/system"
)

// listenNetwork is the network of the HTTP listener.
const listenNetwork = "tcp"

// catalogServices are the use cases of the system, library and scan endpoints.
type catalogServices struct {
	system    *appsystem.Service
	libraries *applibrary.Service
	scans     *appscan.Service
}

// wiredServices is everything the HTTP adapter serves.
type wiredServices struct {
	catalogServices

	playback *appplayback.Service
	streams  *stream.Handlers
}

// httpHandlers are the handlers the HTTP server routes between: the REST API below /api/ and
// the web UI for every other path.
type httpHandlers struct {
	api   http.Handler
	webUI http.Handler
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

	httpServer, err := server.New(app.serverOptions(listener, handlers))
	if err != nil {
		return errors.Join(fmt.Errorf("create http server: %w", err), listener.Close())
	}

	app.hooks.reportListening(listener.Addr())
	app.startLibraryScans(ctx, services.scans)

	if err := httpServer.Run(ctx); err != nil {
		return fmt.Errorf("run http server: %w", err)
	}

	return nil
}

// serverOptions quiets the stream delivery routes, whose segment polling would flood the info
// log, but not the session resource itself: a DELETE of a session stays at info. The LAN-only
// guard is an opt-out, so the zero value keeps it on (OD-10); nil interface prefixes mean
// this host's interfaces.
func (app application) serverOptions(listener net.Listener, handlers httpHandlers) server.Options {
	return server.Options{
		Listener:           listener,
		Logger:             app.logger,
		APIHandler:         handlers.api,
		Fallback:           handlers.webUI,
		QuietPatterns:      stream.QuietPatterns(),
		AllowPublicClients: !app.config.Server.LANOnly,
	}
}

func (app application) newHTTPHandlers(ctx context.Context, services wiredServices) (httpHandlers, error) {
	apiHandler, err := restapi.NewHandler(restapi.Options{
		System:    services.system,
		Libraries: services.libraries,
		Items:     services.libraries,
		Scan:      services.scans,
		Playback:  services.playback,
		Logger:    app.logger,
		Mounts:    services.streams.Mounts(),
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
