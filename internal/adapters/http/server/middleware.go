package server

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/requestid"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/http/statuswriter"
)

// Security headers sent with every response.
const (
	headerContentTypeOptions = "X-Content-Type-Options"
	valueNoSniff             = "nosniff"
	headerReferrerPolicy     = "Referrer-Policy"
	valueNoReferrer          = "no-referrer"
)

// accessLogHeaders is the allowlist of request headers written to the access log. Headers
// are never logged as a whole: the log redaction works on attribute keys only and cannot
// see secrets inside an http.Header value (platform/logging). Never add Authorization,
// Cookie or headers that may carry tokens.
func accessLogHeaders() []string {
	return []string{"User-Agent"}
}

// headersKeptOnPanic are the response headers the panic problem keeps. Everything else a
// panicking handler set (Content-Type, Content-Length, caching, ...) belongs to the response
// it never finished and would contradict the problem.
func headersKeptOnPanic() []string {
	return []string{headerContentTypeOptions, headerReferrerPolicy, requestid.HeaderName}
}

// withMiddleware wraps next in the chain, outermost first: the request ID exists before
// anything logs, security headers are set even on panics, the access log sees the status
// the recover middleware writes.
func withMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	handler := recoverPanics(next, logger)
	handler = logAccess(handler, logger)
	handler = setSecurityHeaders(handler)

	return requestid.Middleware(handler)
}

func setSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		header := writer.Header()
		header.Set(headerContentTypeOptions, valueNoSniff)
		header.Set(headerReferrerPolicy, valueNoReferrer)
		next.ServeHTTP(writer, request)
	})
}

// logAccess writes one info record per request, aborted ones included. Only the path is
// logged, never the query: query strings may carry tokens in later milestones.
func logAccess(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		access := accessRecord{logger: logger, started: time.Now(), recorder: statuswriter.New(writer)}
		defer access.finish(request)

		next.ServeHTTP(access.recorder, request)
	})
}

// accessRecord holds what the deferred access log needs about one request.
type accessRecord struct {
	logger   *slog.Logger
	started  time.Time
	recorder *statuswriter.Recorder
}

// finish must be deferred directly, so its recover sees an abort. Only http.ErrAbortHandler
// reaches it, because recoverPanics handles every other panic; the abort is logged and then
// passed on, so net/http still drops the connection.
func (access accessRecord) finish(request *http.Request) {
	recovered := recover()

	attributes := []any{
		slog.String("method", request.Method),
		slog.String("path", truncateForLog(request.URL.Path)),
		slog.Int("status", access.recorder.StatusOrOK()),
		slog.Bool("aborted", recovered != nil),
		slog.Duration("duration", time.Since(access.started)),
		slog.Int64("bytes", access.recorder.Written()),
		slog.String("request_id", requestid.FromContext(request.Context())),
	}
	attributes = append(attributes, allowlistedHeaders(request.Header)...)

	access.logger.InfoContext(request.Context(), "http request", attributes...)

	if recovered != nil {
		panic(recovered) //nolint:forbidigo // net/http contract to abort a response
	}
}

// allowlistedHeaders returns the accessLogHeaders that are present, keyed in snake case
// ("User-Agent" becomes "user_agent") and cut to maxLoggedValueBytes.
func allowlistedHeaders(header http.Header) []any {
	var attributes []any

	for _, name := range accessLogHeaders() {
		if value := header.Get(name); value != "" {
			key := strings.ReplaceAll(strings.ToLower(name), "-", "_")
			attributes = append(attributes, slog.String(key, truncateForLog(value)))
		}
	}

	return attributes
}

// recoverPanics turns a panicking handler into a 500 problem and an error log with the
// stack, so one bad request cannot kill the connection without a trace.
func recoverPanics(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		recovery := panicRecovery{logger: logger, writer: writer, recorder: statuswriter.New(writer)}
		defer recovery.handle(request)

		next.ServeHTTP(recovery.recorder, request)
	})
}

// panicRecovery holds what the deferred handle needs about one request.
type panicRecovery struct {
	logger *slog.Logger
	// writer is the response writer outside the recorder, used for the problem.
	writer http.ResponseWriter
	// recorder tells whether the handler already sent its headers.
	recorder *statuswriter.Recorder
}

// handle must be deferred directly: recover only stops a panic when called by the deferred
// function itself.
func (recovery panicRecovery) handle(request *http.Request) {
	recovered := recover()
	if recovered == nil {
		return
	}

	if isAbort(recovered) {
		// A deliberate abort is no bug: net/http drops the connection without a log line.
		panic(recovered) //nolint:forbidigo // net/http contract to abort a response
	}

	recovery.logPanic(request, recovered)

	if recovery.recorder.HeadersSent() {
		// The status and maybe part of the body are out. Finishing normally would end a
		// chunked body cleanly and present a truncated response as complete; aborting
		// makes net/http drop the connection, so the client sees an error.
		panic(http.ErrAbortHandler) //nolint:forbidigo // net/http contract to abort a response
	}

	keepOnlyHeaders(recovery.writer.Header(), headersKeptOnPanic())
	restapi.WriteProblem(recovery.writer, restapi.InternalProblem())
}

func (recovery panicRecovery) logPanic(request *http.Request, recovered any) {
	recovery.logger.ErrorContext(request.Context(), "http handler panicked",
		slog.String("panic", fmt.Sprint(recovered)),
		slog.String("stack", string(debug.Stack())),
		slog.String("method", request.Method),
		slog.String("path", truncateForLog(request.URL.Path)),
		slog.String("request_id", requestid.FromContext(request.Context())))
}

// isAbort reports whether recovered is http.ErrAbortHandler, the sentinel handlers panic
// with to make net/http drop the connection.
func isAbort(recovered any) bool {
	err, ok := recovered.(error)

	return ok && errors.Is(err, http.ErrAbortHandler)
}

// keepOnlyHeaders removes every header of header except the named ones.
func keepOnlyHeaders(header http.Header, names []string) {
	kept := make(http.Header, len(names))
	for _, name := range names {
		if values := header.Values(name); len(values) > 0 {
			kept[http.CanonicalHeaderKey(name)] = values
		}
	}

	clear(header)
	maps.Copy(header, kept)
}
