package server

import (
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/logtext"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
)

// manageRefusalDetail is the fixed detail of the management guard's 403 problem.
const manageRefusalDetail = "Library management is only available from the local network until accounts exist."

// Request headers and values the management guard reads.
const (
	headerOrigin       = "Origin"
	headerFetchSite    = "Sec-Fetch-Site"
	fetchSiteCrossSite = "cross-site"
	jsonMediaType      = "application/json"
)

// Default ports of the origin scheme, which browsers leave out of the Origin header.
const (
	schemeHTTP  = "http"
	schemeHTTPS = "https"
	portHTTP    = "80"
	portHTTPS   = "443"
)

// Paths of the guarded routes (api/openapi.yaml).
const (
	apiV1Libraries = "/api/v1/libraries"
	apiV1Roots     = "/api/v1/folder-roots"
)

// manageGuard protects the library management operations until accounts exist (milestone 7,
// ADR 0028): creating, changing and deleting libraries, starting a scan and browsing the
// server's folders. It refuses a request with 403 forbidden when
//
//   - the socket peer is not in the local network, also when server.lan_only is off: that
//     setting opens the media endpoints to the internet, never the administration;
//   - an Origin header names another origin than the request's own, or Sec-Fetch-Site says
//     cross-site: a page on another site must not drive a LAN-side browser into the API;
//   - a request with a body does not declare application/json, which keeps simple form
//     posts (no CORS preflight) from reaching the handlers.
//
// Only RemoteAddr counts for the peer, as in lanGuard; X-Forwarded-* headers are ignored.
//
// Residual risks, accepted until accounts exist (ADR 0028): a reverse proxy or tunnel that
// connects from the LAN or the host itself makes every client look local, and its
// https origin differs from the plain-http request seen here, so such a setup is refused
// rather than trusted; DNS rebinding can make a hostile page address the server under its
// own name, in which case Origin and Host match. The Host header is therefore not a
// defence, only the local-network peer rule and the later account check are.
type manageGuard struct {
	logger *slog.Logger
	lan    lanGuard
}

func newManageGuard(settings chainSettings, lan lanGuard) manageGuard {
	return manageGuard{logger: settings.logger, lan: lan}
}

func (guard manageGuard) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !isManagementRequest(request) {
			next.ServeHTTP(writer, request)

			return
		}

		if reason := guard.refusal(request); reason != "" {
			guard.logger.DebugContext(request.Context(), "library management request refused",
				logtext.RequestAttributes(request, slog.String("reason", reason),
					slog.String("remote_address", logtext.Truncate(request.RemoteAddr)))...)
			restapi.WriteProblem(writer, manageRefusalProblem())

			return
		}

		next.ServeHTTP(writer, request)
	})
}

// refusal names why the request is refused, or returns "" when it may pass.
func (guard manageGuard) refusal(request *http.Request) string {
	switch {
	case !guard.lan.admitsPeer(request):
		return "peer outside the local network"
	case request.Header.Get(headerFetchSite) == fetchSiteCrossSite:
		return "cross-site request"
	case !originMatches(request):
		return "foreign origin"
	case hasBody(request) && !isJSON(request):
		return "body is not JSON"
	default:
		return ""
	}
}

func manageRefusalProblem() apigen.Problem {
	problem := restapi.NewProblem(http.StatusForbidden, restapi.CodeForbidden)
	detail := manageRefusalDetail
	problem.Detail = &detail

	return problem
}

// isManagementRequest reports the requests the guard protects: every write to the library
// collection, to one library and to its scan trigger, and every request to the folder
// browser, which shows the server's directory tree. Reads of the libraries stay open to the
// normal clients. The decoded path is matched, like the router does, so an encoded variant
// of a path cannot slip past.
func isManagementRequest(request *http.Request) bool {
	segments := strings.Split(strings.Trim(request.URL.Path, "/"), "/")
	isRead := request.Method == http.MethodGet || request.Method == http.MethodHead

	switch {
	case isPrefixOf(segments, apiV1Roots):
		// /api/v1/folder-roots and /api/v1/folder-roots/{rootId}/folders
		return len(segments) == 3 || (len(segments) == 5 && segments[4] == "folders")
	case isPrefixOf(segments, apiV1Libraries):
		// /api/v1/libraries, /api/v1/libraries/{id} and /api/v1/libraries/{id}/scan
		return !isRead && (len(segments) == 3 || len(segments) == 4 || (len(segments) == 5 && segments[4] == "scan"))
	default:
		return false
	}
}

// isPrefixOf reports whether the leading segments spell prefix, a slash-separated path.
func isPrefixOf(segments []string, prefix string) bool {
	want := strings.Split(strings.Trim(prefix, "/"), "/")
	if len(segments) < len(want) {
		return false
	}

	for index, segment := range want {
		if segments[index] != segment {
			return false
		}
	}

	return true
}

func hasBody(request *http.Request) bool {
	return request.ContentLength != 0 || len(request.TransferEncoding) > 0
}

func isJSON(request *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))

	return err == nil && mediaType == jsonMediaType
}

// originMatches reports true without an Origin header (same-origin GET requests and
// non-browser clients send none); otherwise scheme, host and port of the origin must equal
// those the request arrived with. The server speaks plain http itself, so the scheme is
// https only on a TLS connection.
func originMatches(request *http.Request) bool {
	origin := request.Header.Get(headerOrigin)
	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" || parsed.Path != "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}

	scheme := schemeHTTP
	if request.TLS != nil {
		scheme = schemeHTTPS
	}

	requestHost, requestPort := splitHostPort(request.Host, scheme)
	originHost, originPort := splitHostPort(parsed.Host, strings.ToLower(parsed.Scheme))

	return strings.EqualFold(parsed.Scheme, scheme) &&
		strings.EqualFold(originHost, requestHost) && originPort == requestPort
}

// splitHostPort separates host and port and fills in the default port of scheme.
func splitHostPort(hostport, scheme string) (host, port string) {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		host, port = strings.Trim(hostport, "[]"), ""
	}

	if port == "" {
		port = portHTTP
		if scheme == schemeHTTPS {
			port = portHTTPS
		}
	}

	return host, port
}
