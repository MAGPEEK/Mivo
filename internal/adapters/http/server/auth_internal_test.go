package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/app/auth"
	"github.com/magpeek/mivo/internal/domain"
	domainaccess "github.com/magpeek/mivo/internal/domain/access"
)

// Tokens the fake resolver knows.
const (
	adminToken     = "admin-token"
	userToken      = "user-token"
	noProfileToken = "no-profile-token"
	failingToken   = "failing-token"

	adminAccountID = "a0000000-0000-4000-8000-000000000001"
	userAccountID  = "a0000000-0000-4000-8000-000000000002"
	otherAccountID = "a0000000-0000-4000-8000-000000000003"

	// viewerHeader is set by viewerEcho to the account of the stored viewer.
	viewerHeader = "X-Test-Viewer"
)

// fakeResolver resolves the fixed test tokens and records the last peer.
type fakeResolver struct {
	mutex    sync.Mutex
	lastPeer auth.Peer
	calls    int
}

func (resolver *fakeResolver) Resolve(_ context.Context, token string, peer auth.Peer) (auth.Resolution, error) {
	resolver.mutex.Lock()
	resolver.lastPeer, resolver.calls = peer, resolver.calls+1
	resolver.mutex.Unlock()

	switch token {
	case adminToken:
		return resolutionFor(adminAccountID, true, true), nil
	case userToken:
		return resolutionFor(userAccountID, false, true), nil
	case noProfileToken:
		return resolutionFor(userAccountID, false, false), nil
	case failingToken:
		return auth.Resolution{}, errors.New("database locked")
	default:
		return auth.Resolution{}, fmt.Errorf("resolve session: unknown session: %w", domain.ErrUnauthenticated)
	}
}

func (resolver *fakeResolver) peer() (auth.Peer, int) {
	resolver.mutex.Lock()
	defer resolver.mutex.Unlock()

	return resolver.lastPeer, resolver.calls
}

func resolutionFor(accountID string, admin, profileSelected bool) auth.Resolution {
	viewer := domainaccess.Viewer{AuthSessionID: "session-" + accountID, AccountID: accountID, Admin: admin}
	if profileSelected {
		viewer.ProfileID = "profile-" + accountID
	}

	return auth.Resolution{ProfileSelected: profileSelected, Viewer: viewer}
}

// fakeSetup reports a fixed setup state or error.
type fakeSetup struct {
	required bool
	err      error
}

func (setup fakeSetup) SetupRequired(context.Context) (bool, error) {
	return setup.required, setup.err
}

// viewerEcho answers 200 "api" and names the stored viewer's account in viewerHeader.
func viewerEcho() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if viewer, ok := appaccess.ViewerFrom(request.Context()); ok {
			writer.Header().Set(viewerHeader, viewer.AccountID)
		}

		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "api")
	})
}

// authCase is one request through the chain with authentication on.
type authCase struct {
	name        string
	method      string
	target      string
	remoteAddr  string
	cookie      string
	bearer      string
	headers     map[string]string
	body        string
	allowPublic bool
	setup       fakeSetup
	wantStatus  int
	wantCode    string
	wantViewer  string
}

func authOptions(test authCase, resolver SessionResolver) Options {
	logger, _ := newDebugLogger()

	return Options{
		Logger: logger, APIHandler: viewerEcho(), Fallback: textHandler(http.StatusOK, "web"),
		AllowPublicClients: test.allowPublic,
		InterfacePrefixes:  &fakePrefixProvider{prefixes: []netip.Prefix{netip.MustParsePrefix(onLinkPrefix)}},
		Authentication:     Authentication{Sessions: resolver, Setup: test.setup},
	}
}

func newAuthRequest(t *testing.T, test authCase) *http.Request {
	t.Helper()

	headers := map[string]string{}
	for name, value := range test.headers {
		headers[name] = value
	}

	if test.body != "" {
		if _, set := headers["Content-Type"]; !set {
			headers["Content-Type"] = jsonType
		}
	}

	request := newGuardedRequest(t, guardedRequest{
		method: test.method, target: test.target, remoteAddr: test.remoteAddr, headers: headers, body: test.body,
	})

	if test.cookie != "" {
		request.AddCookie(&http.Cookie{
			Name: restapi.SessionCookieName, Value: test.cookie, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
		})
	}

	if test.bearer != "" {
		request.Header.Set("Authorization", "Bearer "+test.bearer)
	}

	return request
}

func runAuthCases(t *testing.T, tests []authCase) {
	t.Helper()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			recorder := serveChain(t, authOptions(test, &fakeResolver{}), newAuthRequest(t, test))
			assertAuthAnswer(t, test, recorder)
		})
	}
}

func assertAuthAnswer(t *testing.T, test authCase, recorder *httptest.ResponseRecorder) {
	t.Helper()

	if recorder.Code != test.wantStatus {
		t.Fatalf("%s %s = %d %s, want %d", test.method, test.target, recorder.Code, recorder.Body, test.wantStatus)
	}

	if challenge := recorder.Header().Get("WWW-Authenticate"); challenge != "" {
		t.Errorf("WWW-Authenticate = %q, want none", challenge)
	}

	if test.wantCode != "" {
		var problem apigen.Problem
		decodeJSON(t, recorder.Body.Bytes(), &problem)

		if problem.Code != test.wantCode {
			t.Errorf("problem code = %q, want %q", problem.Code, test.wantCode)
		}
	}

	if got := recorder.Header().Get(viewerHeader); got != test.wantViewer {
		t.Errorf("viewer = %q, want %q", got, test.wantViewer)
	}
}

const itemsPath = "/api/v1/libraries/" + manageLibraryID + "/items"

func TestAuthOpenRoutes(t *testing.T) {
	t.Parallel()

	runAuthCases(t, []authCase{
		{name: "web ui", method: http.MethodGet, target: "/", wantStatus: http.StatusOK},
		{name: "spa fallback", method: http.MethodGet, target: "/libraries/x", wantStatus: http.StatusOK},
		{name: "health", method: http.MethodGet, target: "/api/v1/health", wantStatus: http.StatusOK},
		{name: "health head", method: http.MethodHead, target: "/api/v1/health", wantStatus: http.StatusOK},
		{name: "system info", method: http.MethodGet, target: "/api/v1/system/info", wantStatus: http.StatusOK},
		{name: "setup state", method: http.MethodGet, target: "/api/v1/setup", wantStatus: http.StatusOK},
		{name: "sign in", method: http.MethodPost, target: "/api/v1/auth/session", body: "{}", wantStatus: http.StatusOK},
		{
			name: "sign in from a matching origin", method: http.MethodPost, target: "/api/v1/auth/session", body: "{}",
			headers: map[string]string{"Origin": "http://" + manageHost}, wantStatus: http.StatusOK,
		},
		{
			name: "sign in cross-site", method: http.MethodPost, target: "/api/v1/auth/session", body: "{}",
			headers: map[string]string{"Sec-Fetch-Site": "cross-site"}, wantStatus: http.StatusForbidden, wantCode: restapi.CodeForbidden,
		},
		{
			name: "sign in from a foreign origin", method: http.MethodPost, target: "/api/v1/auth/session", body: "{}",
			headers: map[string]string{"Origin": "http://evil.example"}, wantStatus: http.StatusForbidden, wantCode: restapi.CodeForbidden,
		},
		{
			name: "sign in as a form", method: http.MethodPost, target: "/api/v1/auth/session", body: "name=a",
			headers:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
			wantStatus: http.StatusForbidden, wantCode: restapi.CodeForbidden,
		},
		{
			name: "other method on an open path needs a session", method: http.MethodPut, target: "/api/v1/health",
			wantStatus: http.StatusUnauthorized, wantCode: restapi.CodeUnauthenticated,
		},
	})
}

func TestAuthProtectedRoutes(t *testing.T) {
	t.Parallel()

	runAuthCases(t, []authCase{
		{name: "no token", method: http.MethodGet, target: itemsPath, wantStatus: http.StatusUnauthorized, wantCode: restapi.CodeUnauthenticated},
		{
			name: "no token during setup", method: http.MethodGet, target: itemsPath, setup: fakeSetup{required: true},
			wantStatus: http.StatusUnauthorized, wantCode: restapi.CodeSetupRequired,
		},
		{name: "unknown token", method: http.MethodGet, target: itemsPath, cookie: "nope", wantStatus: http.StatusUnauthorized, wantCode: restapi.CodeUnauthenticated},
		{name: "cookie", method: http.MethodGet, target: itemsPath, cookie: userToken, wantStatus: http.StatusOK, wantViewer: userAccountID},
		{name: "bearer", method: http.MethodGet, target: itemsPath, bearer: userToken, wantStatus: http.StatusOK, wantViewer: userAccountID},
		{
			name: "lowercase bearer scheme", method: http.MethodGet, target: itemsPath,
			headers: map[string]string{"Authorization": "bearer " + userToken}, wantStatus: http.StatusOK, wantViewer: userAccountID,
		},
		{
			name: "basic scheme does not fall back to the cookie", method: http.MethodGet, target: itemsPath, cookie: userToken,
			headers:    map[string]string{"Authorization": "Basic dXNlcjpwdw=="},
			wantStatus: http.StatusUnauthorized, wantCode: restapi.CodeUnauthenticated,
		},
		{
			name: "empty bearer", method: http.MethodGet, target: itemsPath, headers: map[string]string{"Authorization": "Bearer "},
			wantStatus: http.StatusUnauthorized, wantCode: restapi.CodeUnauthenticated,
		},
		{
			name: "no profile selected", method: http.MethodGet, target: itemsPath, cookie: noProfileToken,
			wantStatus: http.StatusForbidden, wantCode: restapi.CodeProfileRequired,
		},
		{name: "media without a profile", method: http.MethodGet, target: "/api/v1/images/x", bearer: noProfileToken, wantStatus: http.StatusForbidden, wantCode: restapi.CodeProfileRequired},
		{name: "own account without a profile", method: http.MethodGet, target: "/api/v1/me", cookie: noProfileToken, wantStatus: http.StatusOK, wantViewer: userAccountID},
		{name: "select a profile", method: http.MethodPut, target: "/api/v1/me/profile", body: "{}", cookie: noProfileToken, wantStatus: http.StatusOK, wantViewer: userAccountID},
		{name: "sign out without a profile", method: http.MethodDelete, target: "/api/v1/auth/session", cookie: noProfileToken, wantStatus: http.StatusOK, wantViewer: userAccountID},
		{name: "own account needs a session", method: http.MethodGet, target: "/api/v1/me", wantStatus: http.StatusUnauthorized, wantCode: restapi.CodeUnauthenticated},
		{name: "remote peer with a session", method: http.MethodGet, target: itemsPath, remoteAddr: publicPeer, allowPublic: true, bearer: userToken, wantStatus: http.StatusOK, wantViewer: userAccountID},
		{name: "unknown api path", method: http.MethodGet, target: "/api/v2/anything", wantStatus: http.StatusUnauthorized, wantCode: restapi.CodeUnauthenticated},
	})
}

func TestAuthCookieWritesKeepTheSameOriginRule(t *testing.T) {
	t.Parallel()

	const playback = "/api/v1/playback/sessions"

	crossSite := map[string]string{"Origin": "http://evil.example", "Sec-Fetch-Site": "cross-site"}
	form := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}

	runAuthCases(t, []authCase{
		{name: "cookie same origin", method: http.MethodPost, target: playback, body: "{}", cookie: userToken, wantStatus: http.StatusOK, wantViewer: userAccountID},
		{name: "cookie cross-site", method: http.MethodPost, target: playback, body: "{}", cookie: userToken, headers: crossSite, wantStatus: http.StatusForbidden, wantCode: restapi.CodeForbidden},
		{name: "cookie form", method: http.MethodPost, target: playback, body: "a=b", cookie: userToken, headers: form, wantStatus: http.StatusForbidden, wantCode: restapi.CodeForbidden},
		{name: "cookie cross-site delete", method: http.MethodDelete, target: playback + "/x", cookie: userToken, headers: crossSite, wantStatus: http.StatusForbidden, wantCode: restapi.CodeForbidden},
		{name: "cookie cross-site read", method: http.MethodGet, target: itemsPath, cookie: userToken, headers: crossSite, wantStatus: http.StatusOK, wantViewer: userAccountID},
		{name: "bearer cross-site", method: http.MethodPost, target: playback, body: "{}", bearer: userToken, headers: crossSite, wantStatus: http.StatusOK, wantViewer: userAccountID},
		{name: "bearer form", method: http.MethodPost, target: playback, body: "a=b", bearer: userToken, headers: form, wantStatus: http.StatusOK, wantViewer: userAccountID},
	})
}

func TestAuthAdminRoutes(t *testing.T) {
	t.Parallel()

	ownProfiles := "/api/v1/users/" + userAccountID + "/profiles"

	runAuthCases(t, []authCase{
		{name: "admin creates a library", method: http.MethodPost, target: "/api/v1/libraries", body: "{}", cookie: adminToken, wantStatus: http.StatusOK, wantViewer: adminAccountID},
		{name: "admin bearer", method: http.MethodPost, target: "/api/v1/libraries", body: "{}", bearer: adminToken, wantStatus: http.StatusOK, wantViewer: adminAccountID},
		{name: "non-admin creates a library", method: http.MethodPost, target: "/api/v1/libraries", body: "{}", cookie: userToken, wantStatus: http.StatusForbidden, wantCode: restapi.CodeForbidden},
		{name: "no token", method: http.MethodPost, target: "/api/v1/libraries", body: "{}", wantStatus: http.StatusUnauthorized, wantCode: restapi.CodeUnauthenticated},
		{
			name: "admin from a remote peer", method: http.MethodPost, target: "/api/v1/libraries", body: "{}", bearer: adminToken,
			remoteAddr: publicPeer, allowPublic: true, wantStatus: http.StatusForbidden, wantCode: restapi.CodeForbidden,
		},
		{
			name: "admin cookie cross-site", method: http.MethodPost, target: "/api/v1/libraries", body: "{}", cookie: adminToken,
			headers: map[string]string{"Origin": "http://evil.example"}, wantStatus: http.StatusForbidden, wantCode: restapi.CodeForbidden,
		},
		{name: "non-admin identify", method: http.MethodPost, target: "/api/v1/items/x/identify", body: "{}", cookie: userToken, wantStatus: http.StatusForbidden},
		{name: "non-admin refresh", method: http.MethodPost, target: "/api/v1/items/x/refresh", cookie: userToken, wantStatus: http.StatusForbidden},
		{name: "non-admin match", method: http.MethodDelete, target: "/api/v1/items/x/match", cookie: userToken, wantStatus: http.StatusForbidden},
		{name: "non-admin metadata settings", method: http.MethodGet, target: "/api/v1/settings/metadata", cookie: userToken, wantStatus: http.StatusForbidden},
		{name: "non-admin users", method: http.MethodGet, target: "/api/v1/users", cookie: userToken, wantStatus: http.StatusForbidden},
		{name: "admin users", method: http.MethodGet, target: "/api/v1/users", cookie: adminToken, wantStatus: http.StatusOK, wantViewer: adminAccountID},
		{name: "non-admin devices", method: http.MethodGet, target: "/api/v1/devices", cookie: userToken, wantStatus: http.StatusForbidden},
		{name: "admin devices", method: http.MethodDelete, target: "/api/v1/devices/x", cookie: adminToken, wantStatus: http.StatusOK, wantViewer: adminAccountID},
		{name: "own profiles", method: http.MethodGet, target: ownProfiles, cookie: userToken, wantStatus: http.StatusOK, wantViewer: userAccountID},
		{name: "own profiles from a remote peer", method: http.MethodPost, target: ownProfiles, body: "{}", bearer: userToken, remoteAddr: publicPeer, allowPublic: true, wantStatus: http.StatusOK, wantViewer: userAccountID},
		{name: "other account's profiles", method: http.MethodGet, target: "/api/v1/users/" + otherAccountID + "/profiles", cookie: userToken, wantStatus: http.StatusForbidden},
		{name: "own user record", method: http.MethodGet, target: "/api/v1/users/" + userAccountID, cookie: userToken, wantStatus: http.StatusForbidden},
		{name: "change a profile", method: http.MethodPatch, target: "/api/v1/profiles/x", body: "{}", cookie: userToken, wantStatus: http.StatusOK, wantViewer: userAccountID},
		{name: "delete a profile", method: http.MethodDelete, target: "/api/v1/profiles/x", cookie: userToken, wantStatus: http.StatusOK, wantViewer: userAccountID},
		{name: "library reads stay open", method: http.MethodGet, target: "/api/v1/libraries", cookie: userToken, wantStatus: http.StatusOK, wantViewer: userAccountID},
	})
}

func TestAuthSetup(t *testing.T) {
	t.Parallel()

	pending := fakeSetup{required: true}

	runAuthCases(t, []authCase{
		{name: "lan peer during setup", method: http.MethodPost, target: "/api/v1/setup", body: "{}", setup: pending, wantStatus: http.StatusOK},
		{name: "loopback peer during setup", method: http.MethodPost, target: "/api/v1/setup", body: "{}", remoteAddr: "127.0.0.1:1", setup: pending, wantStatus: http.StatusOK},
		{
			name: "remote peer during setup", method: http.MethodPost, target: "/api/v1/setup", body: "{}", setup: pending,
			remoteAddr: publicPeer, allowPublic: true, wantStatus: http.StatusForbidden, wantCode: restapi.CodeForbidden,
		},
		{
			name: "cross-site during setup", method: http.MethodPost, target: "/api/v1/setup", body: "{}", setup: pending,
			headers: map[string]string{"Origin": "http://evil.example"}, wantStatus: http.StatusForbidden, wantCode: restapi.CodeForbidden,
		},
		{
			name: "form during setup", method: http.MethodPost, target: "/api/v1/setup", body: "a=b", setup: pending,
			headers: map[string]string{"Content-Type": "text/plain"}, wantStatus: http.StatusForbidden, wantCode: restapi.CodeForbidden,
		},
		{name: "setup done", method: http.MethodPost, target: "/api/v1/setup", body: `{"name":"x"}`, wantStatus: http.StatusConflict, wantCode: restapi.CodeConflict},
		{name: "empty probe after setup", method: http.MethodPost, target: "/api/v1/setup", wantStatus: http.StatusConflict, wantCode: restapi.CodeConflict},
		{
			name: "remote probe after setup", method: http.MethodPost, target: "/api/v1/setup", remoteAddr: publicPeer, allowPublic: true,
			wantStatus: http.StatusForbidden, wantCode: restapi.CodeForbidden,
		},
		{name: "setup state read", method: http.MethodGet, target: "/api/v1/setup", remoteAddr: publicPeer, allowPublic: true, wantStatus: http.StatusOK},
	})
}

// TestAuthEncodedPathsCannotSkipTheGuard: only plain paths match the open and profile-free
// routes; every encoded, unclean or slash-suffixed spelling falls back to the strictest rule.
func TestAuthEncodedPathsCannotSkipTheGuard(t *testing.T) {
	t.Parallel()

	unauthenticated := func(name, method, target string) authCase {
		return authCase{name: name, method: method, target: target, wantStatus: http.StatusUnauthorized, wantCode: restapi.CodeUnauthenticated}
	}
	needsProfile := func(name, target string) authCase {
		return authCase{
			name: name, method: http.MethodGet, target: target, cookie: noProfileToken,
			wantStatus: http.StatusForbidden, wantCode: restapi.CodeProfileRequired,
		}
	}

	runAuthCases(t, []authCase{
		unauthenticated("encoded letter in health", http.MethodGet, "/api/v1/heal%74h"),
		unauthenticated("encoded slashes in health", http.MethodGet, "/api%2Fv1%2Fhealth"),
		unauthenticated("trailing slash on health", http.MethodGet, "/api/v1/health/"),
		unauthenticated("dot segment to health", http.MethodGet, "/api/v1/users/../health"),
		unauthenticated("encoded sign in", http.MethodPost, "/api/v1/auth%2Fsession"),
		unauthenticated("encoded api prefix", http.MethodGet, "/api%2Fv1%2Flibraries"),
		unauthenticated("double slash", http.MethodGet, "//api/v1/libraries"),
		unauthenticated("encoded dot segments", http.MethodGet, "/%2e%2e/api/v1/libraries"),
		needsProfile("dot segment out of me", "/api/v1/me/../libraries"),
		needsProfile("encoded dot segment out of me", "/api/v1/me%2F..%2Flibraries"),
		needsProfile("encoded me", "/api/v1/m%65"),
		{
			name: "encoded own profiles", method: http.MethodGet, target: "/api/v1/users/" + userAccountID + "%2Fprofiles",
			cookie: userToken, wantStatus: http.StatusForbidden, wantCode: restapi.CodeForbidden,
		},
		{
			name: "encoded users path", method: http.MethodGet, target: "/api/v1/%75sers", cookie: userToken,
			wantStatus: http.StatusForbidden, wantCode: restapi.CodeForbidden,
		},
	})
}

func TestAuthServerFailuresAnswer500(t *testing.T) {
	t.Parallel()

	runAuthCases(t, []authCase{
		{name: "resolver fails", method: http.MethodGet, target: itemsPath, cookie: failingToken, wantStatus: http.StatusInternalServerError, wantCode: restapi.CodeInternal},
		{
			name: "setup state fails on a refusal", method: http.MethodGet, target: itemsPath,
			setup: fakeSetup{err: errors.New("disk")}, wantStatus: http.StatusInternalServerError, wantCode: restapi.CodeInternal,
		},
		{
			name: "setup state fails on setup", method: http.MethodPost, target: "/api/v1/setup", body: "{}",
			setup: fakeSetup{err: errors.New("disk")}, wantStatus: http.StatusInternalServerError, wantCode: restapi.CodeInternal,
		},
	})
}

func TestAuthPassesTheClassifiedPeer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		remoteAddr string
		want       auth.Peer
	}{
		{remoteAddr: lanPeer, want: auth.Peer{Address: netip.MustParseAddr("192.168.1.20"), Class: auth.AddressLAN}},
		{remoteAddr: "[::ffff:192.168.1.21]:1", want: auth.Peer{Address: netip.MustParseAddr("192.168.1.21"), Class: auth.AddressLAN}},
		{remoteAddr: publicPeer, want: auth.Peer{Address: netip.MustParseAddr("203.0.113.7"), Class: auth.AddressRemote}},
		{remoteAddr: "garbage", want: auth.Peer{Class: auth.AddressRemote}},
	}

	for _, test := range tests {
		resolver := &fakeResolver{}
		request := newAuthRequest(t, authCase{method: http.MethodGet, target: itemsPath, remoteAddr: test.remoteAddr, bearer: userToken})
		serveChain(t, authOptions(authCase{allowPublic: true}, resolver), request)

		if got, calls := resolver.peer(); calls != 1 || got != test.want {
			t.Errorf("%s: peer = %+v after %d calls, want %+v", test.remoteAddr, got, calls, test.want)
		}
	}
}

// TestAuthCrossSiteCookieWriteCostsNoLookup: the same-origin rule runs before the resolver.
func TestAuthCrossSiteCookieWriteCostsNoLookup(t *testing.T) {
	t.Parallel()

	resolver := &fakeResolver{}
	test := authCase{
		method: http.MethodPost, target: "/api/v1/playback/sessions", body: "{}", cookie: userToken,
		headers: map[string]string{"Sec-Fetch-Site": "cross-site"},
	}

	recorder := serveChain(t, authOptions(test, resolver), newAuthRequest(t, test))
	if _, calls := resolver.peer(); recorder.Code != http.StatusForbidden || calls != 0 {
		t.Errorf("cross-site cookie write = %d after %d lookups, want 403 after none", recorder.Code, calls)
	}
}

func TestAuthIsOffWithoutWiring(t *testing.T) {
	t.Parallel()

	logger, _ := newDebugLogger()
	options := Options{Logger: logger, APIHandler: viewerEcho()}

	if recorder := serveChain(t, options, newRequest(t, http.MethodGet, itemsPath)); recorder.Code != http.StatusOK {
		t.Errorf("GET items without authentication wired = %d, want 200", recorder.Code)
	}

	request := newGuardedRequest(t, guardedRequest{method: http.MethodPost, target: "/api/v1/setup"})
	if recorder := serveChain(t, options, request); recorder.Code != http.StatusOK {
		t.Errorf("POST setup without authentication wired = %d, want 200 (no setup gate)", recorder.Code)
	}
}

func TestNewRefusesHalfWiredAuthentication(t *testing.T) {
	t.Parallel()

	logger, _ := newDebugLogger()

	for _, authentication := range []Authentication{{Sessions: &fakeResolver{}}, {Setup: fakeSetup{}}} {
		_, err := New(Options{Address: ":0", Logger: logger, APIHandler: viewerEcho(), Authentication: authentication})
		if !errors.Is(err, domain.ErrInvalid) || !strings.Contains(err.Error(), "authentication") {
			t.Errorf("New(%+v) = %v, want an invalid-options error naming authentication", authentication, err)
		}
	}

	if _, err := New(Options{
		Address: ":0", Logger: logger, APIHandler: viewerEcho(),
		Authentication: Authentication{Sessions: &fakeResolver{}, Setup: fakeSetup{}},
	}); err != nil {
		t.Errorf("New with full authentication = %v, want nil", err)
	}
}
