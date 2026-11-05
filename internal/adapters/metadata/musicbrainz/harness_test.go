package musicbrainz_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/metadata/musicbrainz"
	"github.com/magpeek/mivo/internal/adapters/netaccess/outbound"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

const (
	fixtureDir = "../../../../testdata/providers/musicbrainz"
	apiPrefix  = "/ws/2"

	okComputerGroup   = "b1392450-e666-3926-a536-22c65f834433"
	okComputerRelease = "0b6b4ba0-d36f-47bd-b4ea-6a5b91842d29"
	radioheadArtist   = "a74b1b7f-71a5-4011-9441-d0b5e4122711"
)

// allowedParams is everything a request may carry (ADR 0031 §5): the search query built
// from tag-derived names and the year, the answer format, a result limit and the includes.
var allowedParams = []string{"query", "fmt", "limit", "inc"} //nolint:gochecknoglobals // read-only test table

// route is one fixture answer, selected by path and, when set, a fragment of the query.
type route struct {
	path       string
	query      string
	status     int
	retryAfter string
	fixture    string
}

// recorded is a request the fake MusicBrainz received.
type recorded struct {
	path   string
	query  url.Values
	header http.Header
}

// fakeMusicBrainz serves fixtures and records every request.
type fakeMusicBrainz struct {
	t      *testing.T
	routes []route
	server *httptest.Server

	mu       sync.Mutex
	requests []recorded
}

func newFake(t *testing.T, routes ...route) *fakeMusicBrainz {
	t.Helper()

	fake := &fakeMusicBrainz{t: t, routes: routes}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(fake.server.Close)

	return fake
}

func (fake *fakeMusicBrainz) serve(writer http.ResponseWriter, request *http.Request) {
	fake.mu.Lock()
	fake.requests = append(fake.requests, recorded{path: request.URL.Path, query: request.URL.Query(), header: request.Header.Clone()})
	fake.mu.Unlock()

	for _, candidate := range fake.routes {
		if apiPrefix+candidate.path != request.URL.Path || !strings.Contains(request.URL.Query().Get("query"), candidate.query) {
			continue
		}

		body, err := os.ReadFile(filepath.Join(fixtureDir, candidate.fixture))
		if err != nil {
			fake.t.Errorf("read fixture %s: %v", candidate.fixture, err)
		}

		writer.Header().Set("Content-Type", "application/json")

		if candidate.retryAfter != "" {
			writer.Header().Set("Retry-After", candidate.retryAfter)
		}

		writer.WriteHeader(max(candidate.status, http.StatusOK))
		_, _ = writer.Write(body)

		return
	}

	fake.t.Errorf("unexpected request %s?%s", request.URL.Path, request.URL.RawQuery)
	http.NotFound(writer, request)
}

func (fake *fakeMusicBrainz) recorded() []recorded {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	return slices.Clone(fake.requests)
}

// assertPrivacy fails when a request carried a parameter outside allowedParams or asked for
// anything but JSON.
func (fake *fakeMusicBrainz) assertPrivacy() {
	fake.t.Helper()

	for _, request := range fake.recorded() {
		for name := range request.query {
			if !slices.Contains(allowedParams, name) {
				fake.t.Errorf("request %s carried parameter %q", request.path, name)
			}
		}

		if got := request.query.Get("fmt"); got != "json" {
			fake.t.Errorf("request %s fmt = %q, want json", request.path, got)
		}
	}
}

// fakeClock is the injected clock of the request pacer: Sleep advances Now and records the
// pause instead of waiting.
type fakeClock struct {
	mu    sync.Mutex
	now   time.Time
	slept []time.Duration
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, time.November, 5, 10, 0, 0, 0, time.UTC)}
}

func (clock *fakeClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()

	return clock.now
}

func (clock *fakeClock) Sleep(ctx context.Context, pause time.Duration) error {
	clock.mu.Lock()
	defer clock.mu.Unlock()

	clock.slept = append(clock.slept, pause)
	clock.now = clock.now.Add(pause)

	return ctx.Err()
}

func (clock *fakeClock) advance(pause time.Duration) {
	clock.mu.Lock()
	defer clock.mu.Unlock()

	clock.now = clock.now.Add(pause)
}

func (clock *fakeClock) pauses() []time.Duration {
	clock.mu.Lock()
	defer clock.mu.Unlock()

	return slices.Clone(clock.slept)
}

// newClient builds an adapter against the fake through a real outbound client.
func newClient(fake *fakeMusicBrainz, clock *fakeClock) *musicbrainz.Client {
	return musicbrainz.New(musicbrainz.Options{
		HTTP:    newOutbound(),
		BaseURL: fake.server.URL + apiPrefix,
		Now:     clock.Now,
		Sleep:   clock.Sleep,
	})
}

func newOutbound() *outbound.Client {
	return outbound.New(outbound.Options{
		AllowInsecureLoopback: true,
		MaxAttempts:           2,
		Sleep:                 func(context.Context, time.Duration) error { return nil },
	})
}

func mbRef(kind match.RefKind, id string) match.ProviderRef {
	return match.ProviderRef{Provider: match.RefMusicBrainz, Kind: kind, ID: id}
}
