// Package restapi serves the REST API of api/openapi.yaml. It implements the generated
// apigen.StrictServerInterface on top of the use cases and owns the mapping of domain errors
// to RFC 9457 problem details (ADR 0017).
package restapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/logtext"
	"github.com/magpeek/mivo/internal/adapters/http/requestid"
	"github.com/magpeek/mivo/internal/domain/system"
)

// SystemService is the part of app/system.Service the system endpoints need.
type SystemService interface {
	// Health reports the aggregated server health; failures are part of the result.
	Health(ctx context.Context) system.Health
	// Info describes the running server.
	Info(ctx context.Context) (system.Info, error)
}

// API implements apigen.StrictServerInterface.
type API struct {
	system    SystemService
	libraries LibraryCatalog
	items     ItemCatalog
	scan      ScanService
	playback  PlaybackService
	logger    *slog.Logger
	mounts    []Mount
}

// Compile-time check that API serves every operation of the contract.
var _ apigen.StrictServerInterface = (*API)(nil)

// NewAPI returns the API handlers. Invalid options, such as a missing service, are a wiring
// bug and are reported as an error wrapping domain.ErrInvalid.
func NewAPI(options Options) (*API, error) {
	err := options.validate()
	if err != nil {
		return nil, fmt.Errorf("new rest api: %w", err)
	}

	return &API{
		system:    options.System,
		libraries: options.Libraries,
		items:     options.Items,
		scan:      options.Scan,
		playback:  options.Playback,
		logger:    options.Logger,
		mounts:    options.Mounts,
	}, nil
}

// NewHandler returns the routed API: every operation of the contract and the mounts, a
// request body limit, plus problem responses for unknown routes (404) and unsupported
// methods (405).
func NewHandler(options Options) (http.Handler, error) {
	api, err := NewAPI(options)
	if err != nil {
		return nil, err
	}

	return api.Handler()
}

// Handler routes the operations of the contract and the mounts. The generated constructors
// are always called with explicit error handlers: their defaults write err.Error() as
// text/plain, which would leak internals and break the problem contract. A mount whose
// pattern is malformed or conflicts with another route is reported as an error wrapping
// domain.ErrInvalid.
func (api *API) Handler() (http.Handler, error) {
	strict := apigen.NewStrictHandlerWithOptions(api, nil, api.strictOptions())

	mux := http.NewServeMux()
	apigen.HandlerWithOptions(strict, api.routerOptions(mux))

	// The generated routes are registered first, so a conflicting mount fails in
	// registerMount, which turns the mux's panic into an error.
	for _, mount := range api.mounts {
		err := registerMount(mux, mount)
		if err != nil {
			return nil, fmt.Errorf("new rest api: %w", err)
		}
	}

	return limitRequestBodies(routeProblems{mux: mux}), nil
}

func (api *API) strictOptions() apigen.StrictHTTPServerOptions {
	return apigen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  api.writeRequestError,
		ResponseErrorHandlerFunc: api.writeResponseError,
	}
}

func (api *API) routerOptions(mux *http.ServeMux) apigen.StdHTTPServerOptions {
	return apigen.StdHTTPServerOptions{
		BaseRouter:       mux,
		ErrorHandlerFunc: api.writeRequestError,
	}
}

// writeRequestError answers a request the generated code could not parse or bind. The
// error text describes the client's mistake but may quote internals, so it is only logged,
// cut like every client-controlled value. A body above the body limit is answered with 413
// instead of 400, also when the decoder failed on its content before it reached the limit.
func (api *API) writeRequestError(writer http.ResponseWriter, request *http.Request, err error) {
	problem := NewProblem(http.StatusBadRequest, CodeBadRequest)
	if isBodyTooLarge(err) || remainingBodyTooLarge(request) {
		problem = payloadTooLargeProblem()
	}

	api.logger.DebugContext(request.Context(), "rejected malformed request",
		logtext.RequestAttributes(request, slog.Int("status", int(problem.Status)), logtext.Error(err))...)

	WriteProblem(writer, problem)
}

// writeResponseError answers an error returned by a handler or by encoding or writing its
// response.
func (api *API) writeResponseError(writer http.ResponseWriter, request *http.Request, err error) {
	if responseAbandoned(writer, request) {
		// Typically a client that went away mid-request: nobody reads a problem, and a
		// second status line would only make net/http log a superfluous WriteHeader call.
		// The cause may be a client mistake quoting its input, so it is cut.
		api.logger.DebugContext(request.Context(), "api response abandoned",
			logtext.RequestAttributes(request, logtext.Error(err))...)

		return
	}

	problem := ProblemForError(err)

	switch {
	case problem.Status == http.StatusInternalServerError:
		// The client only sees the generic problem; the cause is in the log.
		api.logger.ErrorContext(request.Context(), "api request failed",
			logtext.RequestAttributes(request, slog.Any("error", err))...)
	case problem.Status > http.StatusInternalServerError:
		// For example a shutdown in progress (ErrUnavailable): worth a trace, but no
		// server fault.
		api.logger.WarnContext(request.Context(), "api request could not be served",
			logtext.RequestAttributes(request, slog.Any("error", err))...)
	default:
		// A client mistake such as an unknown ID; the cause helps when debugging a client.
		// It may quote the client's input, so it is cut.
		api.logger.DebugContext(request.Context(), "api request rejected",
			logtext.RequestAttributes(request, logtext.Error(err))...)
	}

	WriteProblem(writer, problem)
}

// logOperationFailure records a server-side failure of a strict handler, which knows its
// context but not the raw request.
func (api *API) logOperationFailure(ctx context.Context, operation string, err error) {
	api.logger.ErrorContext(ctx, "api operation failed",
		slog.String("operation", operation),
		slog.String("request_id", requestid.FromContext(ctx)),
		slog.Any("error", err))
}

// headerReporter is implemented by statuswriter.Recorder, which Handler wraps around the
// writer of every matched route.
type headerReporter interface {
	HeadersSent() bool
}

// responseAbandoned reports whether a problem can no longer reach the client: its request
// context is done (disconnect or server Close) or the status line was already sent.
func responseAbandoned(writer http.ResponseWriter, request *http.Request) bool {
	if request.Context().Err() != nil {
		return true
	}

	reporter, ok := writer.(headerReporter)

	return ok && reporter.HeadersSent()
}
