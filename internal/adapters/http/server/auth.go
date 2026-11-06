package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/logtext"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/app/auth"
	"github.com/magpeek/mivo/internal/domain"
)

// SessionResolver turns a session token into the signed-in session and its viewer. Errors
// wrapping domain.ErrUnauthenticated mean the token is unknown, expired or revoked.
// Implemented by *auth.Service.
type SessionResolver interface {
	Resolve(ctx context.Context, token string, peer auth.Peer) (auth.Resolution, error)
}

// SetupStateReader reports whether the server is still in setup state (no account yet).
// Implemented by *auth.Service.
type SetupStateReader interface {
	SetupRequired(ctx context.Context) (bool, error)
}

// Authentication switches the session middleware of ADR 0032 on. The zero value leaves it
// off, so the server keeps its pre-account behaviour until the composition root wires both
// ports (task M7-25); setting only one of them is invalid.
type Authentication struct {
	// Sessions resolves the session cookie or bearer token of every protected request.
	Sessions SessionResolver
	// Setup tells setup_required from unauthenticated and refuses a repeated setup.
	Setup SetupStateReader
}

func (authentication Authentication) enabled() bool {
	return authentication.Sessions != nil
}

func (authentication Authentication) valid() bool {
	return (authentication.Sessions == nil) == (authentication.Setup == nil)
}

// bearerScheme is the Authorization scheme of non-browser clients (RFC 6750); schemes are
// case-insensitive (RFC 9110 §11.1).
const bearerScheme = "bearer"

// credential is the session token of a request and how it arrived.
type credential struct {
	token string
	// cookie is true for the ambient browser cookie, which the write rules of ADR 0032 §3
	// apply to; a bearer token is never sent by a browser on its own.
	cookie bool
}

// authGuard authenticates every request outside the open routes (see authRoute) and stores
// the viewer with appaccess.WithViewer. It answers 401 unauthenticated, or 401
// setup_required while no account exists, without a WWW-Authenticate challenge, so browsers
// show no basic-auth prompt; 403 forbidden when a cookie-authenticated write breaks the
// same-origin rule; 403 profile_required when the operation needs a selected profile.
type authGuard struct {
	logger         *slog.Logger
	lan            lanGuard
	authentication Authentication
}

func newAuthGuard(settings chainSettings, lan lanGuard) authGuard {
	return authGuard{logger: settings.logger, lan: lan, authentication: settings.authentication}
}

func (guard authGuard) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch route := authRoute(request); route {
		case routeStatic, routeOpen:
			next.ServeHTTP(writer, request)
		case routeOpenWrite:
			guard.serveOpenWrite(writer, request, next)
		case routeSession, routeProfile:
			guard.serveProtected(writer, request, next, route == routeProfile)
		}
	})
}

// serveOpenWrite applies the same-origin rule to the sign-in and setup writes: they carry no
// credential, but a cross-site form must not sign a browser into a foreign account.
func (guard authGuard) serveOpenWrite(writer http.ResponseWriter, request *http.Request, next http.Handler) {
	if reason := crossOriginRefusal(request); reason != "" {
		guard.refuse(writer, request, reason, restapi.NewProblem(http.StatusForbidden, restapi.CodeForbidden))

		return
	}

	next.ServeHTTP(writer, request)
}

// serveProtected resolves the credential and passes the request on with its viewer. The
// same-origin rule runs before the session lookup, so a forged write costs no lookup.
func (guard authGuard) serveProtected(writer http.ResponseWriter, request *http.Request, next http.Handler, needsProfile bool) {
	presented, ok := requestCredential(request)
	if !ok {
		guard.refuseUnauthenticated(writer, request, "no credential")

		return
	}

	if presented.cookie && isUnsafeMethod(request.Method) {
		if reason := crossOriginRefusal(request); reason != "" {
			guard.refuse(writer, request, reason, restapi.NewProblem(http.StatusForbidden, restapi.CodeForbidden))

			return
		}
	}

	resolution, err := guard.authentication.Sessions.Resolve(request.Context(), presented.token, guard.peer(request))
	if errors.Is(err, domain.ErrUnauthenticated) {
		guard.refuseUnauthenticated(writer, request, "session not resolved")

		return
	}

	if err != nil {
		guard.fail(writer, request, "resolve session", err)

		return
	}

	if needsProfile && !resolution.ProfileSelected {
		guard.refuse(writer, request, "no profile selected", restapi.ProblemForError(appaccess.ErrProfileRequired))

		return
	}

	next.ServeHTTP(writer, request.WithContext(appaccess.WithViewer(request.Context(), resolution.Viewer)))
}

// refuseUnauthenticated answers 401 setup_required while no account exists, else 401
// unauthenticated.
func (guard authGuard) refuseUnauthenticated(writer http.ResponseWriter, request *http.Request, reason string) {
	setupRequired, err := guard.authentication.Setup.SetupRequired(request.Context())
	if err != nil {
		guard.fail(writer, request, "read setup state", err)

		return
	}

	refusal := domain.ErrUnauthenticated
	if setupRequired {
		refusal = auth.ErrSetupRequired
	}

	guard.refuse(writer, request, reason, restapi.ProblemForError(refusal))
}

// peer classifies the socket peer like the LAN guard; Address is always set when RemoteAddr
// parses, so the sign-in throttle and the device record never see an empty address.
func (guard authGuard) peer(request *http.Request) auth.Peer {
	address, _ := peerAddress(request.RemoteAddr)
	class := auth.AddressRemote

	if guard.lan.admitsPeer(request) {
		class = auth.AddressLAN
	}

	return auth.Peer{Address: address, Class: class}
}

func (guard authGuard) refuse(writer http.ResponseWriter, request *http.Request, reason string, problem apigen.Problem) {
	guard.logger.DebugContext(request.Context(), "request not authenticated",
		logtext.RequestAttributes(request, slog.String("reason", reason), slog.String("code", problem.Code))...)
	restapi.WriteProblem(writer, problem)
}

func (guard authGuard) fail(writer http.ResponseWriter, request *http.Request, operation string, err error) {
	guard.logger.ErrorContext(request.Context(), "authentication failed",
		logtext.RequestAttributes(request, slog.String("operation", operation), slog.Any("error", err))...)
	restapi.WriteProblem(writer, restapi.InternalProblem())
}

// requestCredential reads the bearer token or, without an Authorization header, the session
// cookie. A malformed Authorization header counts as no credential rather than falling back to
// the cookie, so a client cannot mix both.
func requestCredential(request *http.Request) (credential, bool) {
	if header := request.Header.Get("Authorization"); header != "" {
		scheme, token, found := strings.Cut(header, " ")
		token = strings.TrimSpace(token)

		if !found || !strings.EqualFold(scheme, bearerScheme) || token == "" {
			return credential{}, false
		}

		return credential{token: token}, true
	}

	cookie, err := request.Cookie(restapi.SessionCookieName)
	if err != nil || cookie.Value == "" {
		return credential{}, false
	}

	return credential{token: cookie.Value, cookie: true}, true
}
