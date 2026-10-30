package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/logtext"
	"github.com/magpeek/mivo/internal/adapters/http/statuswriter"
)

// abortedStatus is logged for a request aborted before its status line was sent: the
// client got no status at all, and the 200 net/http would assume never went out.
const abortedStatus = 0

// accessLogHeaders is the allowlist of request headers written to the access log. Headers
// are never logged as a whole: the log redaction works on attribute keys only and cannot
// see secrets inside an http.Header value (platform/logging). Never add Authorization,
// Cookie or headers that may carry tokens.
func accessLogHeaders() []string {
	return []string{"User-Agent"}
}

// logAccess writes one record per request, aborted ones included: at info level, or at
// debug level for successful requests that match a quiet route. Only the path is logged,
// never the query: query strings may carry tokens in later milestones.
func logAccess(next http.Handler, logger *slog.Logger, quiet quietRoutes) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		access := accessRecord{logger: logger, quiet: quiet, started: time.Now(), recorder: statuswriter.New(writer)}
		defer access.finish(request)

		next.ServeHTTP(access.recorder, request)
	})
}

// accessRecord holds what the deferred access log needs about one request.
type accessRecord struct {
	logger   *slog.Logger
	quiet    quietRoutes
	started  time.Time
	recorder *statuswriter.Recorder
}

// finish must be deferred directly, so its recover sees an abort. Only http.ErrAbortHandler
// reaches it, because recoverPanics handles every other panic; the abort is logged and then
// passed on, so net/http still drops the connection.
func (access accessRecord) finish(request *http.Request) {
	recovered := recover()
	aborted := recovered != nil
	status := access.status(aborted)

	attributes := logtext.RequestAttributes(request,
		slog.Int("status", status),
		slog.Bool("aborted", aborted),
		slog.Duration("duration", time.Since(access.started)),
		slog.Int64("bytes", access.recorder.Written()))
	attributes = append(attributes, allowlistedHeaders(request.Header)...)

	access.logger.Log(request.Context(), access.level(request, status, aborted), "http request", attributes...)

	if aborted {
		panic(recovered) //nolint:forbidigo // net/http contract to abort a response
	}
}

// status returns the status the client saw: abortedStatus when the request was aborted
// before the status line went out, else the recorded status or net/http's implicit 200.
func (access accessRecord) status(aborted bool) int {
	if aborted && !access.recorder.HeadersSent() {
		return abortedStatus
	}

	return access.recorder.StatusOrOK()
}

// level keeps successful requests of quiet routes out of the info log. Client and server
// errors and aborts stay at info, because they are what an operator looks for.
func (access accessRecord) level(request *http.Request, status int, aborted bool) slog.Level {
	if !aborted && status < http.StatusBadRequest && access.quiet.match(request) {
		return slog.LevelDebug
	}

	return slog.LevelInfo
}

// allowlistedHeaders returns the accessLogHeaders that are present, keyed in snake case
// ("User-Agent" becomes "user_agent") and cut to logtext.MaxBytes.
func allowlistedHeaders(header http.Header) []any {
	var attributes []any

	for _, name := range accessLogHeaders() {
		if value := header.Get(name); value != "" {
			key := strings.ReplaceAll(strings.ToLower(name), "-", "_")
			attributes = append(attributes, slog.String(key, logtext.Truncate(value)))
		}
	}

	return attributes
}

// quietRoutes matches requests against Options.QuietPatterns with a private ServeMux, so a
// pattern selects exactly the requests the API routes to the same handler: by method (GET
// also matching HEAD), path segments and wildcards. The zero value matches nothing.
type quietRoutes struct {
	mux *http.ServeMux
}

// newQuietRoutes registers patterns. A malformed pattern, or one that conflicts with
// another, is reported as an error: ServeMux.Handle panics on both, and a wiring mistake
// must not crash the server.
func newQuietRoutes(patterns []string) (quietRoutes, error) {
	if len(patterns) == 0 {
		return quietRoutes{}, nil
	}

	mux := http.NewServeMux()
	for _, pattern := range patterns {
		if err := registerQuietPattern(mux, pattern); err != nil {
			return quietRoutes{}, err
		}
	}

	return quietRoutes{mux: mux}, nil
}

func registerQuietPattern(mux *http.ServeMux, pattern string) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("quiet pattern %q: %v", pattern, recovered)
		}
	}()

	mux.Handle(pattern, http.NotFoundHandler())

	return nil
}

// match reports whether request matches a quiet pattern. A request whose path the server
// would redirect to its canonical form counts as the route it is redirected to.
func (routes quietRoutes) match(request *http.Request) bool {
	if routes.mux == nil {
		return false
	}

	_, pattern := routes.mux.Handler(request)

	return pattern != ""
}
