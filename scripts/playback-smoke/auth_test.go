package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

const (
	testUser     = "smoke"
	testPassword = "correct horse battery"
)

// runAgainst plays the passing corpus against server with the given account and returns the
// output and the error of the run.
func runAgainst(t *testing.T, server *httptest.Server, account *credentials) (string, error) {
	t.Helper()

	opts := testOptions(t, server, passingManifest(t))
	opts.account = account

	var out bytes.Buffer

	err := smoke(t.Context(), opts, testDependencies(t, passingSummaries()), &out)

	return out.String(), err
}

// lockedAuth returns the account state of fake after the run.
func lockedAuth(fake *fakeServer) fakeAuth {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	return *fake.auth
}

func TestSmokeRunsTheFirstRunSetupWithAGeneratedAdministrator(t *testing.T) {
	t.Parallel()

	fake, server := newFakeServer(t, passingFiles(t)...)
	fake.auth = &fakeAuth{}

	text, err := runAgainst(t, server, nil)
	if err != nil {
		t.Fatalf("smoke = %v, want nil; output:\n%s", err, text)
	}

	auth := lockedAuth(fake)
	request := auth.setupRequest

	if auth.setups != 1 || request.Name != generatedAdminName || request.Client == nil || *request.Client != apigen.ClientKindCli {
		t.Errorf("setups = %d with %+v, want one as %q with client cli", auth.setups, request, generatedAdminName)
	}

	if request.Password == nil || len(*request.Password) < 8 || strings.Contains(text, *request.Password) {
		t.Errorf("generated password = %v, want at least 8 characters that the output never shows", request.Password)
	}

	assertSignedInRun(t, auth, text, `signed in as "playback-smoke", created by the first-run setup`)
}

func TestSmokeRunsTheFirstRunSetupWithTheGivenAccount(t *testing.T) {
	t.Parallel()

	fake, server := newFakeServer(t, passingFiles(t)...)
	fake.auth = &fakeAuth{}

	text, err := runAgainst(t, server, &credentials{name: testUser, password: testPassword})
	if err != nil {
		t.Fatalf("smoke = %v, want nil; output:\n%s", err, text)
	}

	auth := lockedAuth(fake)
	if auth.accounts[testUser] != testPassword {
		t.Errorf("accounts = %v, want %q created by the setup", auth.accounts, testUser)
	}

	assertSignedInRun(t, auth, text, `signed in as "smoke", created by the first-run setup`)
}

func TestSmokeSignsInWithTheGivenAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		managed  bool
		wantText string
	}{
		{name: "profile selected by the server", wantText: `signed in as "smoke"`},
		// With managed profiles the server selects none, so the smoke selects the primary one.
		{name: "managed profiles", managed: true, wantText: `signed in as "smoke"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fake, server := newFakeServer(t, passingFiles(t)...)
			fake.auth = &fakeAuth{accounts: map[string]string{testUser: testPassword}, managedProfiles: test.managed}

			text, err := runAgainst(t, server, &credentials{name: testUser, password: testPassword})
			if err != nil {
				t.Fatalf("smoke = %v, want nil; output:\n%s", err, text)
			}

			auth := lockedAuth(fake)
			if auth.signIns != 1 || auth.setups != 0 || auth.signInRequest.Client != apigen.ClientKindCli {
				t.Errorf("sign-ins = %d, setups = %d, client %q; want 1, 0 and cli", auth.signIns, auth.setups, auth.signInRequest.Client)
			}

			assertSignedInRun(t, auth, text, test.wantText)
		})
	}
}

// assertSignedInRun checks that every request after the sign-in carried the token, the run
// signed out and the output names the sign-in.
func assertSignedInRun(t *testing.T, auth fakeAuth, text, want string) {
	t.Helper()

	if len(auth.rejected) != 0 {
		t.Errorf("requests without token or profile: %v", auth.rejected)
	}

	if auth.signOuts != 1 {
		t.Errorf("sign-outs = %d, want 1", auth.signOuts)
	}

	for _, line := range []string{want, "summary: 5 passed, 0 failed"} {
		if !strings.Contains(text, line) {
			t.Errorf("output lacks %q:\n%s", line, text)
		}
	}
}

// Servers from before sign-in (404) and with the M7-01 stubs (501) are used anonymously.
func TestSmokeRunsWithoutSignInOnAServerWithoutAccounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		auth *fakeAuth
	}{
		{name: "pre-M7 404"},
		{name: "M7-01 stub 501", auth: &fakeAuth{stubStatus: http.StatusNotImplemented}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fake, server := newFakeServer(t, passingFiles(t)...)
			fake.auth = test.auth

			text, err := runAgainst(t, server, &credentials{name: testUser, password: testPassword})
			if err != nil {
				t.Fatalf("smoke = %v, want nil; output:\n%s", err, text)
			}

			if !strings.Contains(text, "anonymously: the server offers no sign-in") {
				t.Errorf("output lacks the anonymous run:\n%s", text)
			}
		})
	}
}

func TestSmokeFailsWhenItCannotSignIn(t *testing.T) {
	t.Parallel()

	known := map[string]string{testUser: testPassword}
	given := &credentials{name: testUser, password: testPassword}

	tests := []struct {
		name    string
		auth    fakeAuth
		account *credentials
		wantErr error
		want    string
		// signOuts is 1 when the server handed out a token: that session must end too.
		signOuts int
	}{
		{name: "accounts but no -user", auth: fakeAuth{accounts: known}, wantErr: errCredentialsNeeded},
		{
			name: "wrong password", auth: fakeAuth{accounts: known},
			account: &credentials{name: testUser, password: "wrong password"}, want: "invalid_credentials",
		},
		{name: "no token", auth: fakeAuth{accounts: known, noToken: true}, account: given, wantErr: errNoToken},
		{
			name: "primary profile with a PIN", auth: fakeAuth{accounts: known, primaryHasPIN: true}, account: given,
			wantErr: errProfileNeedsPIN, signOuts: 1,
		},
		{name: "no primary profile", auth: fakeAuth{accounts: known, noPrimary: true}, account: given, wantErr: errNoPrimaryProfile, signOuts: 1},
		{
			name: "profile selection fails", auth: fakeAuth{accounts: known, managedProfiles: true, profileStatus: http.StatusInternalServerError},
			account: given, want: "select the primary profile", signOuts: 1,
		},
		{name: "setup state fails", auth: fakeAuth{stubStatus: http.StatusInternalServerError}, account: given, want: "read the setup state"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fake, server := newFakeServer(t, passingFiles(t)...)
			fake.auth = &test.auth

			text, err := runAgainst(t, server, test.account)

			if err == nil || !strings.Contains(err.Error(), "sign in at") ||
				(test.wantErr != nil && !errors.Is(err, test.wantErr)) || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("smoke = %v, want a sign-in error %v %q; output:\n%s", err, test.wantErr, test.want, text)
			}

			if strings.Contains(err.Error(), testPassword) || strings.Contains(text, testPassword) {
				t.Errorf("the password leaked into %q or the output", err)
			}

			if got := lockedAuth(fake).signOuts; got != test.signOuts {
				t.Errorf("sign-outs = %d, want %d", got, test.signOuts)
			}
		})
	}
}

// A run whose cases fail still ends its session.
func TestSmokeSignsOutAfterFailingCases(t *testing.T) {
	t.Parallel()

	files := passingFiles(t)
	files[0].streamStatus = http.StatusInternalServerError
	fake, server := newFakeServer(t, files...)
	fake.auth = &fakeAuth{accounts: map[string]string{testUser: testPassword}}

	text, err := runAgainst(t, server, &credentials{name: testUser, password: testPassword})
	if !errors.Is(err, errCasesFailed) {
		t.Fatalf("smoke = %v, want %v; output:\n%s", err, errCasesFailed, text)
	}

	if got := lockedAuth(fake).signOuts; got != 1 {
		t.Errorf("sign-outs = %d, want 1", got)
	}
}

// A redirect to another origin is refused: the other host sees neither the token nor the
// password a 307 would re-send. Redirects within the origin still work.
func TestClientRefusesRedirectsToAnotherOrigin(t *testing.T) {
	t.Parallel()

	foreignRequests := make(chan string, 1)
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		foreignRequests <- r.Header.Get("Authorization") + " " + string(body)

		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(foreign.Close)

	own := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/loop" {
			http.Redirect(w, r, "/loop", http.StatusFound)

			return
		}

		if r.URL.Path == "/moved" {
			http.Redirect(w, r, "/api/v1/setup", http.StatusTemporaryRedirect)

			return
		}

		if r.URL.Path == "/api/v1/setup" {
			w.WriteHeader(http.StatusNoContent)

			return
		}

		http.Redirect(w, r, foreign.URL+"/steal", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(own.Close)

	client := newAPIClient(mustParseURL(t, own.URL), own.Client())
	client.token = fakeToken

	_, err := createSignInSession(t.Context(), client, credentials{name: testUser, password: testPassword})
	if !errors.Is(err, errCrossOriginRedirect) {
		t.Errorf("sign-in through a cross-origin redirect = %v, want %v", err, errCrossOriginRedirect)
	}

	select {
	case seen := <-foreignRequests:
		t.Errorf("the other origin got a request: %q", seen)
	default:
	}

	err = client.call(t.Context(), apiCall{method: http.MethodGet, reference: "/moved", want: http.StatusNoContent})
	if err != nil {
		t.Errorf("same-origin redirect = %v, want it followed", err)
	}

	err = client.call(t.Context(), apiCall{method: http.MethodGet, reference: "/loop", want: http.StatusOK})
	if !errors.Is(err, errTooManyRedirects) {
		t.Errorf("redirect loop = %v, want %v", err, errTooManyRedirects)
	}
}

// The token goes only to the server's origin, never to another host a URL points at.
func TestSendAddsTheTokenOnlyForTheServerOrigin(t *testing.T) {
	t.Parallel()

	seen := make(chan string, 2)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Get("Authorization")

		w.WriteHeader(http.StatusNoContent)
	})

	own, other := httptest.NewServer(handler), httptest.NewServer(handler)
	t.Cleanup(own.Close)
	t.Cleanup(other.Close)

	client := newAPIClient(mustParseURL(t, own.URL), own.Client())
	client.token = fakeToken

	for _, target := range []string{own.URL + "/a", other.URL + "/b"} {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
		if err != nil {
			t.Fatalf("build the request: %v", err)
		}

		response, err := client.send(request)
		if err != nil {
			t.Fatalf("send %s: %v", target, err)
		}

		closeBody(response)
	}

	if got, want := <-seen, "Bearer "+fakeToken; got != want {
		t.Errorf("own origin Authorization = %q, want %q", got, want)
	}

	if got := <-seen; got != "" {
		t.Errorf("other origin Authorization = %q, want none", got)
	}
}
