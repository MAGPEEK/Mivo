// Package server runs the HTTP server: it mounts the REST API under /api/, hands every
// other path to an optional fallback (the embedded web UI, M1-16), wraps both in the
// middleware chain and shuts down gracefully when its context ends.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

const (
	// ShutdownTimeout bounds how long Run waits for in-flight requests after its context
	// ends before it closes the remaining connections.
	ShutdownTimeout = 10 * time.Second

	// ReadHeaderTimeout limits how long a client may take to send the request headers, so
	// slow clients cannot hold connections open (Slowloris).
	ReadHeaderTimeout = 10 * time.Second

	// IdleTimeout closes keep-alive connections that stay unused this long.
	IdleTimeout = 2 * time.Minute

	// apiPathPrefix is the subtree served by Options.APIHandler.
	apiPathPrefix = "/api/"

	// apiRootPath is the API subtree root without its trailing slash.
	apiRootPath = "/api"

	// listenNetwork is the network of Options.Address.
	listenNetwork = "tcp"
)

// Options configures New. Exactly one of Address and Listener is set.
type Options struct {
	// Address is the TCP address Run listens on, for example ":8080" or "127.0.0.1:0".
	Address string
	// Listener is an already bound listener; Run serves on it and closes it on return.
	Listener net.Listener
	// Logger receives the access log, panics and server errors. Required.
	Logger *slog.Logger
	// APIHandler serves every path below /api/. Required.
	APIHandler http.Handler
	// Fallback serves every path outside the API. Paths whose decoded and cleaned form lies
	// below /api, such as /api%2Fv1%2Fx, always go to APIHandler instead. When Fallback is
	// nil, every path outside the API answers a plain 404.
	Fallback http.Handler
	// QuietPatterns are net/http ServeMux patterns, normally with a method, such as
	// "GET /api/v1/playback/sessions/{sessionId}/hls/{fileName}". Requests they match are
	// access-logged at debug level while they succeed (status below 400), so polling of
	// stream segments does not flood the info log (ADR 0024); failed and aborted requests
	// stay at info. They match like the server routes: by method (GET also matches HEAD),
	// escaped path and wildcards. A malformed or conflicting pattern is invalid.
	QuietPatterns []string
}

// Server is an HTTP server built by New. Run it once.
type Server struct {
	httpServer *http.Server
	address    string
	listener   net.Listener
	logger     *slog.Logger
}

// New validates options and builds the server without opening any socket. Invalid options
// are a wiring bug and are reported as an error wrapping domain.ErrInvalid.
func New(options Options) (*Server, error) {
	settings, err := options.validate()
	if err != nil {
		return nil, err
	}

	return &Server{
		httpServer: &http.Server{
			Handler:           newHandler(options, settings),
			ReadHeaderTimeout: ReadHeaderTimeout,
			IdleTimeout:       IdleTimeout,
			// WriteTimeout stays 0 on purpose: media streaming writes one response for as
			// long as playback runs. ReadTimeout stays 0 as well: net/http's background
			// read would hit it during a long response and cancel the request context.
			// ReadHeaderTimeout covers slow headers, BodyReadTimeout bodies (see
			// limitBodyReadTime).
			ErrorLog: slog.NewLogLogger(options.Logger.Handler(), slog.LevelWarn),
		},
		address:  options.Address,
		listener: options.Listener,
		logger:   options.Logger,
	}, nil
}

// validate reports every problem of options at once and returns the middleware settings
// derived from them.
func (options Options) validate() (chainSettings, error) {
	var problems []error

	if (options.Address == "") == (options.Listener == nil) {
		problems = append(problems, errors.New("set exactly one of address and listener"))
	}

	if options.Logger == nil {
		problems = append(problems, errors.New("missing logger"))
	}

	if options.APIHandler == nil {
		problems = append(problems, errors.New("missing api handler"))
	}

	settings, err := newChainSettings(options)
	if err != nil {
		problems = append(problems, err)
	}

	if len(problems) > 0 {
		return chainSettings{}, fmt.Errorf("invalid http server options: %w: %w", errors.Join(problems...), domain.ErrInvalid)
	}

	return settings, nil
}

// newHandler routes /api/ to the API and everything else to the fallback, behind the
// middleware chain.
func newHandler(options Options, settings chainSettings) http.Handler {
	fallback := options.Fallback
	if fallback == nil {
		fallback = http.NotFoundHandler()
	}

	mux := http.NewServeMux()
	mux.Handle(apiPathPrefix, options.APIHandler)
	mux.Handle("/", apiPathGuard{api: options.APIHandler, fallback: fallback})

	return withMiddleware(mux, settings)
}

// apiPathGuard sends a request the mux passed to the fallback to the API instead when its
// decoded path lies below /api. ServeMux matches the escaped path, so /api%2Fv1%2Fx misses
// the /api/ pattern although it means /api/v1/x; without the guard an API client would get
// the web app's index.html instead of the API's 404 problem.
type apiPathGuard struct {
	api      http.Handler
	fallback http.Handler
}

func (guard apiPathGuard) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if isAPIPath(request.URL.Path) {
		guard.api.ServeHTTP(writer, request)

		return
	}

	guard.fallback.ServeHTTP(writer, request)
}

// isAPIPath reports whether the decoded urlPath names the API root or a path below it.
// Cleaning resolves encoded dot segments such as /%2e%2e/api/v1, which the mux leaves alone.
// webui.isAPIName applies the same rule to file names as defence in depth; keep both in step.
func isAPIPath(urlPath string) bool {
	cleaned := path.Clean("/" + urlPath)

	return cleaned == apiRootPath || strings.HasPrefix(cleaned, apiPathPrefix)
}

// Run serves until ctx is done, then shuts down gracefully within ShutdownTimeout. It
// returns nil after a clean shutdown and an error when listening or serving fails.
func (server *Server) Run(ctx context.Context) error {
	listener, err := server.listen(ctx)
	if err != nil {
		return err
	}

	server.logger.InfoContext(ctx, "http server listening", slog.String("address", listener.Addr().String()))

	serveResult := make(chan error, 1)
	go func() { serveResult <- server.httpServer.Serve(listener) }()

	select {
	case err := <-serveResult:
		// Serve never returns nil; ending before ctx means it failed.
		return fmt.Errorf("serve http: %w", err)
	case <-ctx.Done():
	}

	return server.shutdown(ctx, serveResult)
}

func (server *Server) listen(ctx context.Context) (net.Listener, error) {
	if server.listener != nil {
		return server.listener, nil
	}

	var config net.ListenConfig

	listener, err := config.Listen(ctx, listenNetwork, server.address)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", server.address, err)
	}

	return listener, nil
}

// shutdown stops accepting connections and waits for in-flight requests. The wait uses a
// fresh deadline, because ctx is already done when shutdown starts.
func (server *Server) shutdown(ctx context.Context, serveResult <-chan error) error {
	server.logger.InfoContext(ctx, "http server shutting down")

	shutdownContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), ShutdownTimeout)
	defer cancel()

	if err := server.httpServer.Shutdown(shutdownContext); err != nil {
		closeErr := server.httpServer.Close()

		return errors.Join(fmt.Errorf("shut down http server: %w", err), closeErr)
	}

	if err := <-serveResult; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve http: %w", err)
	}

	return nil
}
