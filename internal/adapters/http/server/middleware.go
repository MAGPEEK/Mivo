package server

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/logtext"
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

// maxLoggedPanicBytes bounds the panic value in the panic record. It is mostly server text
// and worth more room than a client value, but may quote client input.
const maxLoggedPanicBytes = 4 << 10

// headersKeptOnPanic are the response headers the panic problem keeps. Everything else a
// panicking handler set (Content-Type, Content-Length, caching, ...) belongs to the response
// it never finished and would contradict the problem.
func headersKeptOnPanic() []string {
	return []string{headerContentTypeOptions, headerReferrerPolicy, requestid.HeaderName}
}

// chainSettings are the middleware settings New derives from Options.
type chainSettings struct {
	logger *slog.Logger
	// quiet selects the requests whose success is logged at debug level.
	quiet quietRoutes
	// bodyReadTimeout is BodyReadTimeout; tests shorten it.
	bodyReadTimeout time.Duration
	// allowPublicClients is Options.AllowPublicClients.
	allowPublicClients bool
	// interfacePrefixes is Options.InterfacePrefixes or the system default.
	interfacePrefixes InterfacePrefixProvider
	// now is the clock of the interface prefix cache, time.Now outside tests.
	now func() time.Time
}

// newChainSettings validates the parts of options only the middleware understands.
func newChainSettings(options Options) (chainSettings, error) {
	quiet, err := newQuietRoutes(options.QuietPatterns)
	if err != nil {
		return chainSettings{}, err
	}

	interfacePrefixes := options.InterfacePrefixes
	if interfacePrefixes == nil {
		interfacePrefixes = systemInterfacePrefixes{}
	}

	return chainSettings{
		logger: options.Logger, quiet: quiet, bodyReadTimeout: BodyReadTimeout,
		allowPublicClients: options.AllowPublicClients, interfacePrefixes: interfacePrefixes, now: time.Now,
	}, nil
}

// withMiddleware wraps next in the chain, outermost first: the request ID exists before
// anything logs, the body read deadline is set before any handler can answer (or leave the
// body unread), security headers are set even on panics, the access log sees the status the
// recover middleware writes. The LAN guard is innermost, so a refusal carries the request
// ID and security headers and appears in the access log like any other answer.
func withMiddleware(next http.Handler, settings chainSettings) http.Handler {
	handler := next
	if !settings.allowPublicClients {
		handler = newLANGuard(settings).wrap(handler)
	}

	handler = recoverPanics(handler, settings.logger)
	handler = logAccess(handler, settings.logger, settings.quiet)
	handler = setSecurityHeaders(handler)
	handler = limitBodyReadTime(handler, settings.logger, settings.bodyReadTimeout)

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
		// A deliberate abort is no bug. net/http drops the connection silently only for the
		// sentinel itself (it compares with ==), so a wrapped abort is passed on unwrapped.
		panic(http.ErrAbortHandler) //nolint:forbidigo // net/http contract to abort a response
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
		logtext.RequestAttributes(request,
			slog.String("panic", logtext.TruncateMiddleTo(fmt.Sprint(recovered), maxLoggedPanicBytes)),
			slog.String("stack", string(debug.Stack())))...)
}

// isAbort reports whether recovered is or wraps http.ErrAbortHandler, the sentinel handlers
// panic with to make net/http drop the connection.
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
