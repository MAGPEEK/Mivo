package server

import (
	"net/http"
	"path"
	"strings"
)

// authRouteClass is what the authentication middleware demands of a request.
type authRouteClass int

const (
	// routeProfile needs a session with a selected profile: catalog, media, watch state and
	// administration, and every API path not named below (fail closed).
	routeProfile authRouteClass = iota
	// routeStatic is outside the API: the static web UI and its SPA fallback.
	routeStatic
	// routeOpen needs no session: health, system info and the setup state.
	routeOpen
	// routeOpenWrite needs no session but must keep the same-origin rule: sign-in and setup.
	routeOpenWrite
	// routeSession needs a session but no profile: the own account (/me) and sign-out, so a
	// client can pick a profile in the first place.
	routeSession
)

// Paths of the routes the authentication middleware treats specially (api/openapi.yaml,
// ADR 0032 §6).
const (
	apiV1Health      = "/api/v1/health"
	apiV1SystemInfo  = "/api/v1/system/info"
	apiV1AuthSession = "/api/v1/auth/session"
	apiV1Me          = "/api/v1/me"
)

// authRoute classifies request. Only a plain path (see plainPath) can match an open or
// profile-free route, so an encoded, unclean or slash-suffixed variant falls back to the
// strictest class instead of slipping past the check.
func authRoute(request *http.Request) authRouteClass {
	if !isAPIPath(request.URL.Path) && !isAPIPath(request.URL.EscapedPath()) {
		return routeStatic
	}

	plain, ok := plainPath(request)
	if !ok {
		return routeProfile
	}

	isRead := request.Method == http.MethodGet || request.Method == http.MethodHead

	switch {
	case isRead && (plain == apiV1Health || plain == apiV1SystemInfo || plain == apiV1Setup):
		return routeOpen
	case request.Method == http.MethodPost && (plain == apiV1Setup || plain == apiV1AuthSession):
		return routeOpenWrite
	case request.Method == http.MethodDelete && plain == apiV1AuthSession:
		return routeSession
	case plain == apiV1Me || strings.HasPrefix(plain, apiV1Me+"/"):
		return routeSession
	default:
		return routeProfile
	}
}

// plainPath returns the request path and true when it is clean and carries no percent
// encoding, so every layer (middleware, mux, handler) reads the same path from it.
func plainPath(request *http.Request) (string, bool) {
	decoded := request.URL.Path

	return decoded, decoded != "" && request.URL.EscapedPath() == decoded && path.Clean(decoded) == decoded
}
