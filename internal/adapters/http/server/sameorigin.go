package server

import (
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Request headers and values of the ADR 0028 same-origin rule.
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

// crossOriginRefusal names why request breaks the same-origin rule of ADR 0028, or returns
// "" when it keeps it. The rule is shared by the management guard and by cookie-authenticated
// writes (ADR 0032 §3):
//
//   - an Origin header names another origin than the request's own, or Sec-Fetch-Site says
//     cross-site: a page on another site must not drive a LAN-side browser into the API;
//   - a request with a body does not declare application/json, which keeps simple form
//     posts (no CORS preflight) from reaching the handlers.
func crossOriginRefusal(request *http.Request) string {
	switch {
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

// isUnsafeMethod reports a method that may change state (RFC 9110 §9.2.1). Every method but
// the safe ones counts, so an unusual method cannot slip past the write rules.
func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
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
