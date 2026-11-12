package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
)

const (
	manageLibraryID = "3f6c1d2a-9b8e-4f70-8a1c-2d3e4f5a6b7c"
	manageHost      = "mivo.local:8460"
	jsonType        = "application/json"
)

// guardedRequest describes one request to the management guard.
type guardedRequest struct {
	name        string
	method      string
	target      string
	remoteAddr  string
	headers     map[string]string
	body        string
	allowPublic bool
	wantPassed  bool
}

func newGuardedRequest(t *testing.T, test guardedRequest) *http.Request {
	t.Helper()

	var body *strings.Reader
	if test.body != "" {
		body = strings.NewReader(test.body)
	} else {
		body = strings.NewReader("")
	}

	request := httptest.NewRequestWithContext(t.Context(), test.method, test.target, body)
	request.Host = manageHost
	request.RemoteAddr = test.remoteAddr

	if test.remoteAddr == "" {
		request.RemoteAddr = lanPeer
	}

	if test.body == "" {
		request.ContentLength = 0
	}

	for name, value := range test.headers {
		request.Header.Set(name, value)
	}

	return request
}

func manageCases() []guardedRequest {
	create := func(name string, mutate func(*guardedRequest)) guardedRequest {
		test := guardedRequest{
			name: name, method: http.MethodPost, target: "/api/v1/libraries", body: `{"name":"x"}`,
			headers: map[string]string{"Content-Type": jsonType}, wantPassed: true,
		}
		mutate(&test)

		return test
	}

	return []guardedRequest{
		create("lan peer, JSON, no origin", func(*guardedRequest) {}),
		create("loopback peer", func(test *guardedRequest) { test.remoteAddr = "127.0.0.1:5000" }),
		create("ipv4-mapped lan peer", func(test *guardedRequest) { test.remoteAddr = "[::ffff:192.168.1.20]:5000" }),
		create("on-link global ipv6 peer", func(test *guardedRequest) { test.remoteAddr = "[2001:db8:1:2::77]:5000" }),
		create("public peer", func(test *guardedRequest) { test.remoteAddr = publicPeer; test.wantPassed = false }),
		create("public peer with lan_only off", func(test *guardedRequest) {
			test.remoteAddr = publicPeer
			test.allowPublic = true
			test.wantPassed = false
		}),
		create("lan peer with lan_only off", func(test *guardedRequest) { test.allowPublic = true }),
		create("public ipv6 outside the on-link prefix", func(test *guardedRequest) {
			test.remoteAddr = "[2606:4700:4700::1111]:443"
			test.wantPassed = false
		}),
		create("mapped public peer", func(test *guardedRequest) { test.remoteAddr = "[::ffff:8.8.8.8]:1"; test.wantPassed = false }),
		create("unparsable peer", func(test *guardedRequest) { test.remoteAddr = "garbage"; test.wantPassed = false }),
		create("forwarded headers do not make a public peer local", func(test *guardedRequest) {
			test.remoteAddr = publicPeer
			test.headers["X-Forwarded-For"] = "192.168.1.5"
			test.headers["X-Real-IP"] = "192.168.1.5"
			test.headers["Forwarded"] = "for=192.168.1.5"
			test.wantPassed = false
		}),
		create("forwarded headers do not change the origin rule", func(test *guardedRequest) {
			test.headers["Origin"] = "https://evil.example"
			test.headers["X-Forwarded-Host"] = "evil.example"
			test.headers["X-Forwarded-Proto"] = "https"
			test.wantPassed = false
		}),
		create("matching origin", func(test *guardedRequest) { test.headers["Origin"] = "http://mivo.local:8460" }),
		create("matching origin in other letter case", func(test *guardedRequest) { test.headers["Origin"] = "http://MIVO.local:8460" }),
		create("foreign origin", func(test *guardedRequest) { test.headers["Origin"] = "http://evil.example"; test.wantPassed = false }),
		create("other port", func(test *guardedRequest) { test.headers["Origin"] = "http://mivo.local:5173"; test.wantPassed = false }),
		create("other scheme", func(test *guardedRequest) {
			test.headers["Origin"] = "https://mivo.local:8460"
			test.wantPassed = false
		}),
		create("null origin", func(test *guardedRequest) { test.headers["Origin"] = "null"; test.wantPassed = false }),
		create("origin with a path", func(test *guardedRequest) {
			test.headers["Origin"] = "http://mivo.local:8460/x"
			test.wantPassed = false
		}),
		create("unparsable origin", func(test *guardedRequest) { test.headers["Origin"] = "://"; test.wantPassed = false }),
		create("same-origin fetch metadata", func(test *guardedRequest) { test.headers["Sec-Fetch-Site"] = "same-origin" }),
		create("none fetch metadata", func(test *guardedRequest) { test.headers["Sec-Fetch-Site"] = "none" }),
		create("same-site fetch metadata", func(test *guardedRequest) { test.headers["Sec-Fetch-Site"] = "same-site" }),
		create("cross-site fetch metadata", func(test *guardedRequest) { test.headers["Sec-Fetch-Site"] = "cross-site"; test.wantPassed = false }),
		create("JSON with charset", func(test *guardedRequest) { test.headers["Content-Type"] = "application/json; charset=utf-8" }),
		create("form content type", func(test *guardedRequest) {
			test.headers["Content-Type"] = "application/x-www-form-urlencoded"
			test.wantPassed = false
		}),
		create("text content type", func(test *guardedRequest) { test.headers["Content-Type"] = "text/plain"; test.wantPassed = false }),
		create("missing content type", func(test *guardedRequest) { delete(test.headers, "Content-Type"); test.wantPassed = false }),
		create("malformed content type", func(test *guardedRequest) { test.headers["Content-Type"] = ";;"; test.wantPassed = false }),
	}
}

func TestManageGuardOnCreateLibrary(t *testing.T) {
	t.Parallel()

	for _, test := range manageCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assertGuarded(t, test)
		})
	}
}

func assertGuarded(t *testing.T, test guardedRequest) {
	t.Helper()

	logger, _ := newDebugLogger()
	options := Options{
		Logger: logger, APIHandler: textHandler(http.StatusOK, "api"), AllowPublicClients: test.allowPublic,
		InterfacePrefixes: &fakePrefixProvider{prefixes: []netip.Prefix{netip.MustParsePrefix(onLinkPrefix)}},
	}

	recorder := serveChain(t, options, newGuardedRequest(t, test))
	passed := recorder.Code == http.StatusOK && recorder.Body.String() == "api"

	if passed != test.wantPassed {
		t.Fatalf("%s %s = %d %q, want passed %v", test.method, test.target, recorder.Code, recorder.Body, test.wantPassed)
	}

	if !test.wantPassed {
		assertManageRefusal(t, recorder)
	}
}

func assertManageRefusal(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()

	if recorder.Code != http.StatusForbidden {
		// A public peer behind the LAN guard gets that guard's refusal; both are 403.
		t.Fatalf("status = %d, want 403", recorder.Code)
	}

	if contentType := recorder.Header().Get("Content-Type"); contentType != restapi.ProblemMediaType {
		t.Errorf("Content-Type = %q, want %q", contentType, restapi.ProblemMediaType)
	}

	var problem apigen.Problem
	decodeJSON(t, recorder.Body.Bytes(), &problem)

	if problem.Code != restapi.CodeForbidden || problem.Detail == nil || *problem.Detail == "" {
		t.Errorf("problem = %+v, want code forbidden with a detail", problem)
	}
}

func TestManageGuardRefusalDetailIsFixed(t *testing.T) {
	t.Parallel()

	test := guardedRequest{
		method: http.MethodDelete, target: "/api/v1/libraries/" + manageLibraryID, remoteAddr: publicPeer, allowPublic: true,
	}
	logger, _ := newDebugLogger()
	options := Options{Logger: logger, APIHandler: textHandler(http.StatusOK, "api"), AllowPublicClients: true}

	recorder := serveChain(t, options, newGuardedRequest(t, test))

	var problem apigen.Problem
	decodeJSON(t, recorder.Body.Bytes(), &problem)

	if problem.Detail == nil || *problem.Detail != manageRefusalDetail {
		t.Errorf("detail = %v, want %q", problem.Detail, manageRefusalDetail)
	}

	if want := "Library management needs an administrator on the local network."; manageRefusalDetail != want {
		t.Errorf("manageRefusalDetail = %q, want %q", manageRefusalDetail, want)
	}
}

// TestManageGuardRefusalDetailNamesTheMetadataArea: the metadata routes are refused with
// their own fixed details, so the client does not talk about libraries.
func TestManageGuardRefusalDetailNamesTheMetadataArea(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"/api/v1/settings/metadata":                           "Metadata settings need an administrator on the local network.",
		"/api/v1/metadata/posters/" + strings.Repeat("a", 32): "Metadata management needs an administrator on the local network.",
		"/api/v1/users":   "User administration needs an administrator on the local network.",
		"/api/v1/devices": "Device administration needs an administrator on the local network.",
	}
	for target, want := range tests {
		logger, _ := newDebugLogger()
		options := Options{Logger: logger, APIHandler: textHandler(http.StatusOK, "api"), AllowPublicClients: true}

		recorder := serveChain(t, options, newGuardedRequest(t, guardedRequest{
			method: http.MethodGet, target: target, remoteAddr: publicPeer, allowPublic: true,
		}))

		var problem apigen.Problem
		decodeJSON(t, recorder.Body.Bytes(), &problem)

		if recorder.Code != http.StatusForbidden || problem.Detail == nil || *problem.Detail != want {
			t.Errorf("%s = %d %v, want 403 with %q", target, recorder.Code, problem.Detail, want)
		}
	}
}

// TestManageGuardCoversExactlyTheManagementRoutes pins which routes the guard protects: the
// five management operations and the scan trigger, but not the read endpoints.
func TestManageGuardCoversExactlyTheManagementRoutes(t *testing.T) {
	t.Parallel()

	libraryPath := "/api/v1/libraries/" + manageLibraryID
	itemPath := "/api/v1/items/" + manageLibraryID
	routes := []struct {
		method, target string
		guarded        bool
	}{
		{http.MethodPost, "/api/v1/libraries", true},
		{http.MethodPatch, libraryPath, true},
		{http.MethodDelete, libraryPath, true},
		{http.MethodPost, libraryPath + "/scan", true},
		{http.MethodGet, "/api/v1/folder-roots", true},
		{http.MethodGet, "/api/v1/folder-roots/" + manageLibraryID + "/folders?path=a", true},
		{http.MethodPost, libraryPath + "/refresh", true},
		{http.MethodPost, itemPath + "/identify/search", true},
		{http.MethodPost, itemPath + "/identify", true},
		{http.MethodDelete, itemPath + "/match", true},
		{http.MethodPost, itemPath + "/refresh", true},
		{http.MethodGet, "/api/v1/settings/metadata", true},
		{http.MethodPut, "/api/v1/settings/metadata", true},
		{http.MethodGet, "/api/v1/metadata/posters/" + strings.Repeat("a", 32), true},
		{http.MethodGet, "/api/v1/metadata/status", false},
		{http.MethodGet, itemPath, false},
		{http.MethodGet, itemPath + "/children", false},
		{http.MethodGet, libraryPath + "/refresh", false},
		{http.MethodGet, "/api/v1/libraries", false},
		{http.MethodHead, "/api/v1/libraries", false},
		{http.MethodGet, libraryPath, false},
		{http.MethodGet, libraryPath + "/items", false},
		{http.MethodGet, "/api/v1/system/info", false},
		{http.MethodGet, "/api/v1/health", false},
		{http.MethodPost, "/api/v1/playback/sessions", false},
		{http.MethodGet, "/", false},
		{http.MethodGet, "/api/v1/users", true},
		{http.MethodPost, "/api/v1/users", true},
		{http.MethodGet, "/api/v1/users/" + manageLibraryID + "/profiles", true},
		{http.MethodDelete, "/api/v1/users/" + manageLibraryID, true},
		{http.MethodGet, "/api/v1/devices", true},
		{http.MethodDelete, "/api/v1/devices/" + manageLibraryID, true},
		{http.MethodPost, "/api/v1/setup", true},
		{http.MethodPost, "/api/v1/setup/", true},
		{http.MethodGet, "/api/v1/setup", false},
		{http.MethodPatch, "/api/v1/profiles/" + manageLibraryID, false},
		{http.MethodDelete, "/api/v1/profiles/" + manageLibraryID, false},
		{http.MethodGet, "/api/v1/me", false},
		// ServeMux unescapes per segment, so an encoded slash inside an ID keeps the
		// segment count; the guard must not lose these routes.
		{http.MethodPost, "/api/v1/items/a%2Fb/identify", true},
		{http.MethodPost, "/api/v1/items/a%2Fb/identify/search", true},
		{http.MethodPatch, "/api/v1/libraries/a%2Fb", true},
		{http.MethodPost, "/api/v1/libraries/a%2Fb/scan", true},
		{http.MethodPost, "/api/v1/libraries/a%2Fb/refresh", true},
		{http.MethodGet, "/api/v1/folder-roots/a%2Fb/folders", true},
		{http.MethodGet, "/api/v1/metadata/posters/a%2Fb", true},
		{http.MethodDelete, "/api/v1/devices/a%2Fb", true},
		{http.MethodPost, "/api/v1/libraries/a%2Fb/items", false},
		// The session list and the transcoder are administration (M4-14b); starting, reading
		// and stopping one's own session and the trickplay stay open to every profile.
		{http.MethodGet, "/api/v1/playback/sessions", true},
		{http.MethodHead, "/api/v1/playback/sessions", true},
		{http.MethodGet, "/api/v1/playback/sessions/", true},
		{http.MethodGet, "/api/v1/playback/%73essions", true},
		{http.MethodGet, "/api/v1/playback/sessions?x=1", true},
		{http.MethodGet, "/api/v1/playback/sessions/" + manageLibraryID, false},
		{http.MethodDelete, "/api/v1/playback/sessions/" + manageLibraryID, false},
		{http.MethodGet, "/api/v1/playback/sessions/" + manageLibraryID + "/hls/index.m3u8", false},
		{http.MethodGet, "/api/v1/system/transcoder", true},
		{http.MethodHead, "/api/v1/system/transcoder", true},
		{http.MethodPost, "/api/v1/system/transcoder/test", true},
		{http.MethodPost, "/api/v1/system/%74ranscoder/test", true},
		{http.MethodPost, "/api/v1/system/transcoder/test/", true},
		{http.MethodGet, "/api/v1/system/transcoders", false},
		{http.MethodGet, "/api/v1/media-files/" + manageLibraryID + "/trickplay", false},
		{http.MethodPost, "/api/v1/media-files/" + manageLibraryID + "/trickplay", false},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.target, func(t *testing.T) {
			t.Parallel()

			// A foreign origin and a JSON-less body: refused exactly where the guard applies.
			request := newGuardedRequest(t, guardedRequest{
				method: route.method, target: route.target,
				headers: map[string]string{"Origin": "http://evil.example", "Sec-Fetch-Site": "cross-site"},
			})

			logger, _ := newDebugLogger()
			options := Options{
				Logger: logger, APIHandler: textHandler(http.StatusOK, "api"), Fallback: textHandler(http.StatusOK, "web"),
			}

			recorder := serveChain(t, options, request)
			if refused := recorder.Code == http.StatusForbidden; refused != route.guarded {
				t.Errorf("%s %s = %d, want guarded %v", route.method, route.target, recorder.Code, route.guarded)
			}
		})
	}
}

func TestManageGuardOnBodilessRequests(t *testing.T) {
	t.Parallel()

	libraryPath := "/api/v1/libraries/" + manageLibraryID
	tests := []guardedRequest{
		{name: "delete needs no content type", method: http.MethodDelete, target: libraryPath, wantPassed: true},
		{name: "scan trigger needs no content type", method: http.MethodPost, target: libraryPath + "/scan", wantPassed: true},
		{name: "folder browse needs no content type", method: http.MethodGet, target: "/api/v1/folder-roots", wantPassed: true},
		{name: "scan trigger from a public peer", method: http.MethodPost, target: libraryPath + "/scan", remoteAddr: publicPeer, allowPublic: true},
		{
			name: "scan trigger from a foreign origin", method: http.MethodPost, target: libraryPath + "/scan",
			headers: map[string]string{"Origin": "http://evil.example"},
		},
		{
			name: "browse from a foreign origin", method: http.MethodGet, target: "/api/v1/folder-roots",
			headers: map[string]string{"Origin": "http://evil.example"},
		},
		{
			name: "patch with a wrong content type", method: http.MethodPatch, target: libraryPath, body: `{"name":"x"}`,
			headers: map[string]string{"Content-Type": "text/plain"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assertGuarded(t, test)
		})
	}
}

func decodeJSON(t *testing.T, body []byte, target any) {
	t.Helper()

	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
}

func TestManageGuardLeavesReadsOfLibrariesToTheLANGuardOnly(t *testing.T) {
	t.Parallel()

	logger, _ := newDebugLogger()
	options := Options{Logger: logger, APIHandler: textHandler(http.StatusOK, "api"), AllowPublicClients: true}

	request := newGuardedRequest(t, guardedRequest{
		method: http.MethodGet, target: "/api/v1/libraries", remoteAddr: publicPeer,
		headers: map[string]string{"Origin": "http://evil.example"},
	})

	if recorder := serveChain(t, options, request); recorder.Code != http.StatusOK {
		t.Errorf("GET /libraries from a public peer with lan_only off = %d, want 200", recorder.Code)
	}
}
