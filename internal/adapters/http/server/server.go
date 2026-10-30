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
	// Fallback serves every other path. When nil those paths answer 404.
	Fallback http.Handler
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
	if err := options.validate(); err != nil {
		return nil, err
	}

	return &Server{
		httpServer: &http.Server{
			Handler:           newHandler(options),
			ReadHeaderTimeout: ReadHeaderTimeout,
			IdleTimeout:       IdleTimeout,
			// WriteTimeout stays 0 on purpose: media streaming (milestone 3) writes one
			// response for as long as playback runs. ReadTimeout stays 0 for the same
			// reason with uploads; ReadHeaderTimeout covers slow clients.
			ErrorLog: slog.NewLogLogger(options.Logger.Handler(), slog.LevelWarn),
		},
		address:  options.Address,
		listener: options.Listener,
		logger:   options.Logger,
	}, nil
}

func (options Options) validate() error {
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

	if len(problems) > 0 {
		return fmt.Errorf("invalid http server options: %w: %w", errors.Join(problems...), domain.ErrInvalid)
	}

	return nil
}

// newHandler routes /api/ to the API and everything else to the fallback, behind the
// middleware chain.
func newHandler(options Options) http.Handler {
	fallback := options.Fallback
	if fallback == nil {
		fallback = http.NotFoundHandler()
	}

	mux := http.NewServeMux()
	mux.Handle(apiPathPrefix, options.APIHandler)
	mux.Handle("/", fallback)

	return withMiddleware(mux, options.Logger)
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
