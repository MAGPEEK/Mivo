package restapi_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/app/auth"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/account"
)

const signInBody = `{"name":"Anna","password":"` + authPassword + `","client":"web"}`

// TestAuthEndpointsAnswerNotImplementedWithoutService: an API wired without the auth service
// keeps the 501 answers the contract allows.
func TestAuthEndpointsAnswerNotImplementedWithoutService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		method string
		target string
		body   string
	}{
		{method: http.MethodGet, target: setupPath},
		{method: http.MethodPost, target: setupPath, body: `{"name":"Anna"}`},
		{method: http.MethodPost, target: authSessionPath, body: signInBody},
		{method: http.MethodDelete, target: authSessionPath},
		{method: http.MethodGet, target: mePath},
		{method: http.MethodPut, target: meProfilePath, body: `{"profileId":"` + authPrimaryID + `"}`},
		{method: http.MethodDelete, target: meProfilePath},
		{method: http.MethodPut, target: mePasswordPath, body: `{}`},
		{method: http.MethodGet, target: meDevicesPath},
		{method: http.MethodDelete, target: meDevicesPath + "/" + authOtherDevice},
	}

	for _, test := range tests {
		t.Run(test.method+" "+test.target, func(t *testing.T) {
			t.Parallel()

			got, _ := serveAuth(t, nil, authRequest{method: test.method, target: test.target, body: test.body})
			assertProblem(t, got, http.StatusNotImplemented, restapi.CodeNotImplemented)
		})
	}
}

func TestAuthSetupState(t *testing.T) {
	t.Parallel()

	for _, required := range []bool{true, false} {
		got, _ := serveAuth(t, &fakeAuth{setupRequired: required}, authRequest{method: http.MethodGet, target: setupPath, signedOut: true})
		if state := decode[apigen.SetupState](t, got.body); got.status != http.StatusOK || state.Required != required {
			t.Errorf("setup state = %d %+v, want 200 required %t", got.status, state, required)
		}
	}

	got, logs := serveAuth(t, &fakeAuth{setupErr: errors.New("database locked")}, authRequest{method: http.MethodGet, target: setupPath})
	assertProblem(t, got, http.StatusInternalServerError, restapi.CodeInternal)
	assertLogged(t, logs.String(), "database locked")
}

// TestAuthSignInSetsTheCookieForBrowsers: client "web" gets the cookie and no token.
func TestAuthSignInSetsTheCookieForBrowsers(t *testing.T) {
	t.Parallel()

	for _, secure := range []bool{false, true} {
		service := &fakeAuth{}
		got, _ := serveAuth(t, service, authRequest{
			method: http.MethodPost, target: authSessionPath, body: signInBody, signedOut: true, tls: secure,
			userAgent: firefoxOnWindows, peer: lanPeer(),
		})

		if got.status != http.StatusOK {
			t.Fatalf("status = %d %s, want 200", got.status, got.body)
		}

		assertSessionCookie(t, got, secure)

		session := decode[apigen.Session](t, got.body)
		if session.Token != nil || strings.Contains(string(got.body), authToken) {
			t.Errorf("body %s carries the token of a web client", got.body)
		}

		if session.Me.CurrentProfile == nil || session.Me.CurrentProfile.ID.String() != authPrimaryID ||
			!session.Me.CurrentProfile.IsAdmin || len(session.Me.Profiles) != 1 {
			t.Errorf("me = %+v, want the selected primary profile and the picker list", session.Me)
		}

		if got.header.Get("Cache-Control") != "no-store" {
			t.Errorf("Cache-Control = %q, want no-store", got.header.Get("Cache-Control"))
		}

		want := auth.SignInRequest{
			Name: "Anna", Password: authPassword, Peer: *lanPeer(),
			Device: auth.Device{Name: "Firefox on Windows", Client: auth.ClientWeb, Platform: "Windows"},
		}
		if len(service.signIns) != 1 || service.signIns[0] != want {
			t.Errorf("sign-in request = %+v, want %+v", service.signIns, want)
		}
	}
}

// assertSessionCookie checks the attributes of ADR 0032 §3: HttpOnly, SameSite=Lax, Path=/,
// Secure on TLS and the absolute session lifetime as Max-Age.
func assertSessionCookie(t *testing.T, got response, secure bool) {
	t.Helper()

	cookie := sessionCookieOf(t, got)
	if cookie == nil {
		t.Fatalf("no session cookie in %v", got.header)
	}

	wantMaxAge := int(auth.AbsoluteLifetime / time.Second)
	if cookie.Value != authToken || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode ||
		cookie.Path != "/" || cookie.Secure != secure || cookie.MaxAge != wantMaxAge || cookie.Domain != "" {
		t.Errorf("cookie = %+v, want HttpOnly, SameSite=Lax, Path=/, Secure %t, Max-Age %d", cookie, secure, wantMaxAge)
	}
}

// TestAuthSignInReturnsTheTokenToOtherClients: "tv" and "cli" get the token in the body and
// no cookie.
func TestAuthSignInReturnsTheTokenToOtherClients(t *testing.T) {
	t.Parallel()

	for _, client := range []auth.Client{auth.ClientTV, auth.ClientCLI} {
		service := &fakeAuth{managed: true}
		body := fmt.Sprintf(`{"name":"Anna","password":%q,"client":%q,"deviceName":"  Living room TV "}`, authPassword, client)

		got, _ := serveAuth(t, service, authRequest{
			method: http.MethodPost, target: authSessionPath, body: body, signedOut: true, userAgent: firefoxOnWindows,
		})

		session := decode[apigen.Session](t, got.body)
		if got.status != http.StatusOK || session.Token == nil || *session.Token != authToken {
			t.Errorf("%s: answer = %d %s, want 200 with the token", client, got.status, got.body)
		}

		if got.header.Get("Set-Cookie") != "" {
			t.Errorf("%s: Set-Cookie = %q, want none", client, got.header.Get("Set-Cookie"))
		}

		if session.Me.CurrentProfile != nil || len(session.Me.Profiles) != 2 {
			t.Errorf("%s: me = %+v, want no selected profile and two picker entries", client, session.Me)
		}

		device := service.signIns[0].Device
		if device.Name != "Living room TV" || device.Client != client {
			t.Errorf("%s: device = %+v, want the given name", client, device)
		}

		// Without the middleware's peer the request counts as remote.
		if service.signIns[0].Peer != (auth.Peer{Class: auth.AddressRemote}) {
			t.Errorf("%s: peer = %+v, want remote", client, service.signIns[0].Peer)
		}
	}
}

func TestAuthSignInErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantRetry  string
	}{
		{name: "wrong password", err: fmt.Errorf("sign in: %w", auth.ErrInvalidCredentials), wantStatus: 401, wantCode: restapi.CodeInvalidCredentials},
		{name: "no account yet", err: auth.ErrSetupRequired, wantStatus: 401, wantCode: restapi.CodeSetupRequired},
		{name: "throttled", err: &auth.ThrottledError{Wait: 90 * time.Second}, wantStatus: 429, wantCode: restapi.CodeTooManyAttempts, wantRetry: "90"},
		{name: "unknown client", err: fmt.Errorf("device client: %w", domain.ErrInvalid), wantStatus: 400, wantCode: restapi.CodeInvalid},
		{name: "store failure", err: errors.New("database locked"), wantStatus: 500, wantCode: restapi.CodeInternal},
		{name: "unpublished sentinel", err: domain.ErrNotFound, wantStatus: 500, wantCode: restapi.CodeInternal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, _ := serveAuth(t, &fakeAuth{signInErr: test.err}, authRequest{
				method: http.MethodPost, target: authSessionPath, body: signInBody, signedOut: true,
			})

			problem := assertProblem(t, got, test.wantStatus, test.wantCode)
			if got.header.Get("Retry-After") != test.wantRetry || (test.wantRetry != "") != (problem.RetryAfterSeconds != nil) {
				t.Errorf("Retry-After = %q, retryAfterSeconds = %v, want %q", got.header.Get("Retry-After"), problem.RetryAfterSeconds, test.wantRetry)
			}

			if got.header.Get("Set-Cookie") != "" || got.header.Get("WWW-Authenticate") != "" {
				t.Errorf("headers = %v, want no cookie and no challenge", got.header)
			}
		})
	}
}

// TestAuthSignInFailsWhenTheNewSessionCannotBeDescribed: whatever Me answers after a
// successful sign-in is a server fault, never a 401 that reads like a wrong password.
func TestAuthSignInFailsWhenTheNewSessionCannotBeDescribed(t *testing.T) {
	t.Parallel()

	got, _ := serveAuth(t, &fakeAuth{meErr: domain.ErrUnauthenticated}, authRequest{
		method: http.MethodPost, target: authSessionPath, body: signInBody, signedOut: true,
	})

	assertProblem(t, got, http.StatusInternalServerError, restapi.CodeInternal)

	if got.header.Get("Set-Cookie") != "" {
		t.Errorf("Set-Cookie = %q, want none", got.header.Get("Set-Cookie"))
	}
}

func TestAuthCompleteSetup(t *testing.T) {
	t.Parallel()

	service := &fakeAuth{setupRequired: true}
	got, _ := serveAuth(t, service, authRequest{
		method: http.MethodPost, target: setupPath, body: `{"name":"Anna","password":"` + authPassword + `"}`,
		signedOut: true, userAgent: firefoxOnWindows, peer: lanPeer(),
	})

	if got.status != http.StatusCreated {
		t.Fatalf("status = %d %s, want 201", got.status, got.body)
	}

	if cookie := sessionCookieOf(t, got); cookie == nil || cookie.Value != authToken || !cookie.HttpOnly {
		t.Errorf("cookie = %+v, want the session cookie", cookie)
	}

	if session := decode[apigen.Session](t, got.body); session.Token != nil || session.Me.Account.Name != "Anna" {
		t.Errorf("session = %s, want the account without a token", got.body)
	}

	// client defaults to "web" when the setup request omits it.
	want := auth.SignInRequest{
		Name: "Anna", Password: authPassword, Peer: *lanPeer(),
		Device: auth.Device{Name: "Firefox on Windows", Client: auth.ClientWeb, Platform: "Windows"},
	}
	if len(service.setups) != 1 || service.setups[0] != want {
		t.Errorf("setup request = %+v, want %+v", service.setups, want)
	}

	// The session is described as itself: Me runs with the viewer of the new session.
	if len(service.meViewers) != 1 || service.meViewers[0].AuthSessionID != authSessionID ||
		service.meViewers[0].AccountID != authAccountID || service.meViewers[0].Admin {
		t.Errorf("me viewers = %+v, want the new session without powers", service.meViewers)
	}
}

func TestAuthCompleteSetupForCLIReturnsTheToken(t *testing.T) {
	t.Parallel()

	got, _ := serveAuth(t, &fakeAuth{setupRequired: true}, authRequest{
		method: http.MethodPost, target: setupPath, signedOut: true,
		body: `{"name":"Anna","password":"` + authPassword + `","client":"cli"}`,
	})

	session := decode[apigen.Session](t, got.body)
	if got.status != http.StatusCreated || session.Token == nil || *session.Token != authToken || got.header.Get("Set-Cookie") != "" {
		t.Errorf("answer = %d %s cookie %q, want 201 with the token and no cookie", got.status, got.body, got.header.Get("Set-Cookie"))
	}
}

// TestAuthCompleteSetupAnswerOrder: the conflict comes before the body is validated
// (BACKLOG m7-30), the field errors after it.
func TestAuthCompleteSetupAnswerOrder(t *testing.T) {
	t.Parallel()

	nameError := fmt.Errorf("setup: %w", &account.InvalidFieldError{Field: account.FieldName, Reason: "must have 1 to 40 characters"})
	passwordError := fmt.Errorf("setup: %w", &account.InvalidFieldError{Field: account.FieldPassword, Reason: "too short"})

	tests := []struct {
		name       string
		service    *fakeAuth
		body       string
		wantStatus int
		wantCode   string
		wantField  string
		wantSetups int
	}{
		{name: "empty probe after setup", service: &fakeAuth{}, body: `{}`, wantStatus: 409, wantCode: restapi.CodeConflict},
		{name: "valid body after setup", service: &fakeAuth{}, body: `{"name":"Anna","password":"long enough"}`, wantStatus: 409, wantCode: restapi.CodeConflict},
		{
			name: "invalid name", service: &fakeAuth{setupRequired: true, signInErr: nameError}, body: `{}`,
			wantStatus: 400, wantCode: restapi.CodeInvalid, wantField: "name", wantSetups: 1,
		},
		{
			name: "invalid password", service: &fakeAuth{setupRequired: true, signInErr: passwordError}, body: `{"name":"Anna"}`,
			wantStatus: 400, wantCode: restapi.CodeInvalid, wantField: "password", wantSetups: 1,
		},
		{
			name: "lost the race", service: &fakeAuth{setupRequired: true, signInErr: auth.ErrSetupComplete}, body: `{"name":"Anna"}`,
			wantStatus: 409, wantCode: restapi.CodeConflict, wantSetups: 1,
		},
		{
			name: "setup state unreadable", service: &fakeAuth{setupErr: errors.New("database locked")}, body: `{}`,
			wantStatus: 500, wantCode: restapi.CodeInternal,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, _ := serveAuth(t, test.service, authRequest{method: http.MethodPost, target: setupPath, body: test.body, signedOut: true})

			problem := assertProblem(t, got, test.wantStatus, test.wantCode)
			assertProblemParts(t, problem, test.wantField, false)

			if len(test.service.setups) != test.wantSetups {
				t.Errorf("setup calls = %d, want %d", len(test.service.setups), test.wantSetups)
			}

			if got.header.Get("Set-Cookie") != "" {
				t.Errorf("Set-Cookie = %q, want none", got.header.Get("Set-Cookie"))
			}
		})
	}
}

// TestAuthSignOutDeletesTheSessionAndClearsTheCookie also covers a session that is gone
// already.
func TestAuthSignOutDeletesTheSessionAndClearsTheCookie(t *testing.T) {
	t.Parallel()

	for _, revokeErr := range []error{nil, fmt.Errorf("revoke device: %w", domain.ErrNotFound)} {
		service := &fakeAuth{revokeErr: revokeErr}
		got, _ := serveAuth(t, service, authRequest{method: http.MethodDelete, target: authSessionPath, tls: true})

		if got.status != http.StatusNoContent || len(got.body) != 0 {
			t.Fatalf("answer = %d %s, want 204", got.status, got.body)
		}

		cookie := sessionCookieOf(t, got)
		if cookie == nil || cookie.Value != "" || cookie.MaxAge >= 0 || cookie.Path != "/" || !cookie.HttpOnly || !cookie.Secure {
			t.Errorf("cookie = %+v, want the expired session cookie", cookie)
		}

		if len(service.revoked) != 1 || service.revoked[0] != authSessionID {
			t.Errorf("revoked = %v, want the caller's session", service.revoked)
		}
	}
}

func TestAuthSignOutErrors(t *testing.T) {
	t.Parallel()

	got, _ := serveAuth(t, &fakeAuth{}, authRequest{method: http.MethodDelete, target: authSessionPath, signedOut: true})
	assertProblem(t, got, http.StatusUnauthorized, restapi.CodeUnauthenticated)

	got, _ = serveAuth(t, &fakeAuth{revokeErr: errors.New("database locked")}, authRequest{method: http.MethodDelete, target: authSessionPath})
	assertProblem(t, got, http.StatusInternalServerError, restapi.CodeInternal)

	if got.header.Get("Set-Cookie") != "" {
		t.Errorf("Set-Cookie = %q, want the cookie kept while the session still exists", got.header.Get("Set-Cookie"))
	}
}

// TestAuthRequestLogNeverHoldsSecrets: neither the answers nor the request log repeat a
// password, a PIN or the session token, whatever the outcome.
func TestAuthRequestLogNeverHoldsSecrets(t *testing.T) {
	t.Parallel()

	const (
		newPassword = "another long passphrase"
		pin         = "pin-0x9a7c"
	)

	tests := []struct {
		name    string
		service *fakeAuth
		request authRequest
	}{
		{name: "signed in", service: &fakeAuth{}, request: authRequest{method: http.MethodPost, target: authSessionPath, body: signInBody, signedOut: true}},
		{
			name: "wrong password", service: &fakeAuth{signInErr: auth.ErrInvalidCredentials},
			request: authRequest{method: http.MethodPost, target: authSessionPath, body: signInBody, signedOut: true},
		},
		{
			name: "sign-in fails on the server", service: &fakeAuth{signInErr: errors.New("database locked")},
			request: authRequest{method: http.MethodPost, target: authSessionPath, body: signInBody, signedOut: true},
		},
		{
			name: "malformed sign-in", service: &fakeAuth{},
			request: authRequest{method: http.MethodPost, target: authSessionPath, signedOut: true, body: `{"name":7,"password":"` + authPassword + `"`},
		},
		{
			name: "setup", service: &fakeAuth{setupRequired: true, signInErr: &account.InvalidFieldError{Field: account.FieldPassword, Reason: "too short"}},
			request: authRequest{method: http.MethodPost, target: setupPath, signedOut: true, body: `{"name":"Anna","password":"` + authPassword + `"}`},
		},
		{
			name: "password change refused", service: &fakeAuth{passwordErr: auth.ErrInvalidCredentials},
			request: authRequest{
				method: http.MethodPut, target: mePasswordPath,
				body: `{"currentPassword":"` + authPassword + `","newPassword":"` + newPassword + `"}`,
			},
		},
		{
			name: "wrong pin", service: &fakeAuth{selectErr: auth.ErrPINInvalid},
			request: authRequest{method: http.MethodPut, target: meProfilePath, body: `{"profileId":"` + authManagedID + `","pin":"` + pin + `"}`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, logs := serveAuth(t, test.service, test.request)

			for _, secret := range []string{authPassword, newPassword, pin} {
				if strings.Contains(string(got.body), secret) || strings.Contains(logs.String(), secret) {
					t.Errorf("secret %q is in the answer %s or the log %s", secret, got.body, logs)
				}
			}

			if strings.Contains(logs.String(), authToken) {
				t.Errorf("the session token is in the log %s", logs)
			}
		})
	}
}
