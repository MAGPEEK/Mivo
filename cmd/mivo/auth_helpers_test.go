package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

// Account of the tests: the server asks for sign-in, so every test server that is meant to
// serve data starts with the first admin created and signed in.
const (
	testAccountName = "Tester"
	testPassword    = "a long enough test password"
	setupPath       = "/api/v1/setup"
	signInPath      = "/api/v1/auth/session"
)

// TestMain signs the requests of http.DefaultClient in: the tests of the corpus and of the
// wiring call the API as the administrator that startTestServer created, without each helper
// carrying a token. Requests that must stay anonymous use http.DefaultTransport (fetchStatus)
// or newAnonymousClient.
func TestMain(m *testing.M) {
	http.DefaultClient.Transport = &signInTransport{base: http.DefaultTransport}

	os.Exit(m.Run())
}

// signInTransport adds the bearer token registered for the server a request goes to.
type signInTransport struct {
	base   http.RoundTripper
	tokens sync.Map // host:port -> bearer token
}

func (transport *signInTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	value, signedIn := transport.tokens.Load(request.URL.Host)
	token, _ := value.(string)

	if !signedIn || token == "" || request.Header.Get("Authorization") != "" {
		return transport.base.RoundTrip(request) //nolint:wrapcheck // transparent wrapper
	}

	signed := request.Clone(request.Context())
	signed.Header.Set("Authorization", "Bearer "+token)

	return transport.base.RoundTrip(signed) //nolint:wrapcheck // transparent wrapper
}

// registerToken makes http.DefaultClient send the token to the server at host.
func registerToken(t *testing.T, host, token string) {
	t.Helper()

	transport, ok := http.DefaultClient.Transport.(*signInTransport)
	if !ok {
		t.Fatal("http.DefaultClient has no signInTransport; TestMain did not run")
	}

	transport.tokens.Store(host, token)
}

// newAnonymousClient is a client without credentials, with a cookie jar for the web client's
// session cookie.
func newAnonymousClient(t *testing.T) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}

	return &http.Client{Jar: jar, Transport: http.DefaultTransport}
}

// accountCall is one setup or sign-in request of the test account.
type accountCall struct {
	client  *http.Client
	baseURL string
	// clientKind is "cli" (token in the answer) or "web" (session cookie).
	clientKind string
	headers    map[string]string
}

// anonymousCall is an accountCall of a "cli" client without cookies.
func anonymousCall(t *testing.T, baseURL string) accountCall {
	t.Helper()

	return accountCall{client: newAnonymousClient(t), baseURL: baseURL, clientKind: "cli"}
}

// postSetup creates the first admin through the API from the loopback peer the tests run on.
// A "cli" client gets its token in the answer. It returns the status and the decoded session
// (zero unless the status is 201).
func postSetup(t *testing.T, call accountCall) (int, apigen.Session) {
	t.Helper()

	return postAccount(t, call, setupPath, http.StatusCreated)
}

// postSessions signs the test account in.
func postSessions(t *testing.T, call accountCall) (int, apigen.Session) {
	t.Helper()

	return postAccount(t, call, signInPath, http.StatusOK)
}

func postAccount(t *testing.T, call accountCall, path string, successStatus int) (int, apigen.Session) {
	t.Helper()

	body, err := json.Marshal(map[string]string{"name": testAccountName, "password": testPassword, "client": call.clientKind})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, call.baseURL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	request.Header.Set("Content-Type", "application/json")

	for name, value := range call.headers {
		request.Header.Set(name, value)
	}

	response, err := call.client.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}

	defer func() { _ = response.Body.Close() }()

	var session apigen.Session
	if response.StatusCode == successStatus {
		if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
			t.Fatalf("decode answer of %s: %v", path, err)
		}
	}

	return response.StatusCode, session
}

// signInTestServer runs the first-run setup of the server at baseURL as a "cli" client and
// registers the token for http.DefaultClient. A server that was set up already (a restart on
// the same data directory) is signed in to with the same account instead.
func signInTestServer(t *testing.T, baseURL string) {
	t.Helper()

	status, session := postSetup(t, anonymousCall(t, baseURL))
	if status == http.StatusConflict {
		status, session = postSessions(t, anonymousCall(t, baseURL))
	}

	if (status != http.StatusCreated && status != http.StatusOK) || session.Token == nil {
		t.Fatalf("sign in to %s = %d (token present: %v), want a token", baseURL, status, session.Token != nil)
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse %s: %v", baseURL, err)
	}

	registerToken(t, parsed.Host, *session.Token)
}
