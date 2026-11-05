package tmdb_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/metadata/tmdb"
	"github.com/magpeek/mivo/internal/adapters/netaccess/outbound"
)

const (
	fixtureDir = "../../../../testdata/providers/tmdb"
	testKey    = "0123456789abcdef0123456789abcdef"
	// testToken has the shape of a TMDB v4 read access token (a JWT).
	testToken = "eyJhbGciOiJIUzI1NiJ9.eyJhdWQiOiJ0ZXN0In0.c2lnbmF0dXJl" //nolint:gosec // fake token, not a credential
)

// allowedParams is everything a request may carry (ADR 0031 §5): query, year, language, IDs
// (in the path or as external_source) and fixed switches, plus the key.
var allowedParams = []string{ //nolint:gochecknoglobals // read-only test table
	"query", "year", "first_air_date_year", "language", "include_image_language",
	"append_to_response", "external_source", "include_adult", "api_key",
}

// route is one fixture answer, selected by path and, when set, language.
type route struct {
	path     string
	language string
	status   int
	fixture  string
}

// recorded is a request the fake TMDB received.
type recorded struct {
	path   string
	query  url.Values
	header http.Header
}

// fakeTMDB serves fixtures and records every request.
type fakeTMDB struct {
	t      *testing.T
	routes []route
	server *httptest.Server

	mu       sync.Mutex
	requests []recorded
}

func newFakeTMDB(t *testing.T, routes ...route) *fakeTMDB {
	t.Helper()

	fake := &fakeTMDB{t: t, routes: routes}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(fake.server.Close)

	return fake
}

func (fake *fakeTMDB) serve(writer http.ResponseWriter, request *http.Request) {
	fake.mu.Lock()
	fake.requests = append(fake.requests, recorded{path: request.URL.Path, query: request.URL.Query(), header: request.Header.Clone()})
	fake.mu.Unlock()

	for _, candidate := range fake.routes {
		if candidate.path != request.URL.Path || (candidate.language != "" && candidate.language != request.URL.Query().Get("language")) {
			continue
		}

		body, err := os.ReadFile(filepath.Join(fixtureDir, candidate.fixture))
		if err != nil {
			fake.t.Errorf("read fixture %s: %v", candidate.fixture, err)
		}

		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(max(candidate.status, http.StatusOK))
		_, _ = writer.Write(body)

		return
	}

	fake.t.Errorf("unexpected request %s?%s", request.URL.Path, request.URL.RawQuery)
	http.NotFound(writer, request)
}

func (fake *fakeTMDB) recorded() []recorded {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	return slices.Clone(fake.requests)
}

// assertPrivacy fails when a request carried a parameter outside allowedParams.
func (fake *fakeTMDB) assertPrivacy() {
	fake.t.Helper()

	for _, request := range fake.recorded() {
		for name := range request.query {
			if !slices.Contains(allowedParams, name) {
				fake.t.Errorf("request %s carried parameter %q", request.path, name)
			}
		}
	}
}

// staticKey is a KeyProvider with a fixed answer.
type staticKey struct {
	key string
	err error
}

func (key staticKey) APIKey(context.Context) (string, error) { return key.key, key.err }

// cancellingKey answers testKey and cancels the context once it was asked more than after
// times.
type cancellingKey struct {
	cancel context.CancelFunc
	after  int
	calls  int
}

func (key *cancellingKey) APIKey(context.Context) (string, error) {
	key.calls++
	if key.calls > key.after {
		key.cancel()
	}

	return testKey, nil
}

// newClient builds an adapter against the fake through a real outbound client.
func newClient(fake *fakeTMDB, key string) *tmdb.Client {
	return tmdb.New(tmdb.Options{
		HTTP:         newOutbound(),
		Keys:         staticKey{key: key},
		BaseURL:      fake.server.URL + "/3",
		ImageBaseURL: "https://image.example/t/p",
	})
}

func newOutbound() *outbound.Client {
	return outbound.New(outbound.Options{
		AllowInsecureLoopback: true,
		MaxAttempts:           2,
		Sleep:                 func(context.Context, time.Duration) error { return nil },
	})
}
