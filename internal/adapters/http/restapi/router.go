package restapi

import (
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/statuswriter"
)

// routeProblems serves the generated routes and turns the mux's own text/plain 404 and 405
// answers into problems. It deliberately registers no "/api/" catch-all pattern: a
// method-less catch-all would match a known path requested with the wrong method and hide
// the 405, and it could shadow generated routes. Instead the mux decides, and only its
// answer for unmatched requests is rewritten.
type routeProblems struct {
	mux *http.ServeMux
}

func (router routeProblems) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	// An empty pattern means no route matched: the mux answers 404, 405 (with an Allow
	// header) or redirects to the cleaned path. Matched routes are served unchanged; the
	// recorder only lets writeResponseError see whether the status line is already out.
	if _, pattern := router.mux.Handler(request); pattern != "" {
		router.mux.ServeHTTP(statuswriter.New(writer), request)

		return
	}

	router.mux.ServeHTTP(&routingProblemWriter{ResponseWriter: writer}, request)
}

// routingProblemWriter replaces a 404 or 405 written by the mux with the matching problem
// and drops the mux's plain-text body. Other statuses, such as path-cleaning redirects,
// pass through unchanged.
type routingProblemWriter struct {
	http.ResponseWriter
	replaced bool
}

// WriteHeader writes the problem instead of the mux's status line and body.
func (writer *routingProblemWriter) WriteHeader(status int) {
	problem, ok := routingProblem(status)
	if !ok {
		writer.ResponseWriter.WriteHeader(status)

		return
	}

	writer.replaced = true
	WriteProblem(writer.ResponseWriter, problem)
}

// Write discards the mux's plain-text body once the problem was written.
func (writer *routingProblemWriter) Write(body []byte) (int, error) {
	if writer.replaced {
		return len(body), nil
	}

	// A transparent decorator: the caller sees the underlying writer's result unchanged.
	return writer.ResponseWriter.Write(body)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (writer *routingProblemWriter) Unwrap() http.ResponseWriter {
	return writer.ResponseWriter
}

func routingProblem(status int) (apigen.Problem, bool) {
	switch status {
	case http.StatusNotFound:
		return NewProblem(http.StatusNotFound, CodeNotFound), true
	case http.StatusMethodNotAllowed:
		return NewProblem(http.StatusMethodNotAllowed, CodeMethodNotAllowed), true
	default:
		return apigen.Problem{}, false
	}
}
