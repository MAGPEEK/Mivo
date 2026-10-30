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
	"github.com/magpeek/mivo/internal/adapters/http/requestid"
	"github.com/magpeek/mivo/internal/domain"
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
	system SystemService
	logger *slog.Logger
}

// Compile-time check that API serves every operation of the contract.
var _ apigen.StrictServerInterface = (*API)(nil)

// NewAPI returns the API handlers. A nil service or logger is a wiring bug and is reported
// as an error wrapping domain.ErrInvalid.
func NewAPI(systemService SystemService, logger *slog.Logger) (*API, error) {
	if systemService == nil {
		return nil, fmt.Errorf("new rest api: missing system service: %w", domain.ErrInvalid)
	}

	if logger == nil {
		return nil, fmt.Errorf("new rest api: missing logger: %w", domain.ErrInvalid)
	}

	return &API{system: systemService, logger: logger}, nil
}

// NewHandler returns the routed API: every operation of the contract, plus problem
// responses for unknown routes (404) and unsupported methods (405).
func NewHandler(systemService SystemService, logger *slog.Logger) (http.Handler, error) {
	api, err := NewAPI(systemService, logger)
	if err != nil {
		return nil, err
	}

	return api.Handler(), nil
}

// Handler routes the operations of the contract. The generated constructors are always
// called with explicit error handlers: their defaults write err.Error() as text/plain,
// which would leak internals and break the problem contract.
func (api *API) Handler() http.Handler {
	strict := apigen.NewStrictHandlerWithOptions(api, nil, api.strictOptions())

	mux := http.NewServeMux()
	apigen.HandlerWithOptions(strict, api.routerOptions(mux))

	return routeProblems{mux: mux}
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
// error text describes the client's mistake but may quote internals, so it is only logged.
func (api *API) writeRequestError(writer http.ResponseWriter, request *http.Request, err error) {
	api.logger.DebugContext(request.Context(), "rejected malformed request",
		requestAttributes(request, slog.Any("error", err))...)
	WriteProblem(writer, NewProblem(http.StatusBadRequest, CodeBadRequest))
}

// writeResponseError answers an error returned by a handler or by encoding or writing its
// response.
func (api *API) writeResponseError(writer http.ResponseWriter, request *http.Request, err error) {
	if responseAbandoned(writer, request) {
		// Typically a client that went away mid-request: nobody reads a problem, and a
		// second status line would only make net/http log a superfluous WriteHeader call.
		api.logger.DebugContext(request.Context(), "api response abandoned",
			requestAttributes(request, slog.Any("error", err))...)

		return
	}

	problem := ProblemForError(err)
	if problem.Status >= http.StatusInternalServerError {
		// The client only sees the generic problem; the cause is in the log.
		api.logger.ErrorContext(request.Context(), "api request failed",
			requestAttributes(request, slog.Any("error", err))...)
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
// context is done (disconnect or server shutdown) or the status line was already sent.
func responseAbandoned(writer http.ResponseWriter, request *http.Request) bool {
	if request.Context().Err() != nil {
		return true
	}

	reporter, ok := writer.(headerReporter)

	return ok && reporter.HeadersSent()
}

func requestAttributes(request *http.Request, extra ...any) []any {
	return append([]any{
		slog.String("method", request.Method),
		slog.String("path", request.URL.Path),
		slog.String("request_id", requestid.FromContext(request.Context())),
	}, extra...)
}
