package posterproxy_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/posterproxy"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

const imageFixtures = "../../../../testdata/providers/images"

var posterURLPattern = regexp.MustCompile(`^/api/v1/metadata/posters/[0-9a-f]{32}$`)

// fakeSource serves one body or error and counts the previews.
type fakeSource struct {
	mutex sync.Mutex
	body  []byte
	err   error
	calls int
}

func (source *fakeSource) Preview(context.Context, enrich.ArtworkRef) ([]byte, error) {
	source.mutex.Lock()
	defer source.mutex.Unlock()

	source.calls++

	return source.body, source.err
}

func (source *fakeSource) callCount() int {
	source.mutex.Lock()
	defer source.mutex.Unlock()

	return source.calls
}

// fakeClock is a settable clock.
type fakeClock struct {
	mutex sync.Mutex
	now   time.Time
}

func (clock *fakeClock) Now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	return clock.now
}

func (clock *fakeClock) advance(duration time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	clock.now = clock.now.Add(duration)
}

func readImage(t *testing.T, name string) []byte {
	t.Helper()

	body, err := fs.ReadFile(os.DirFS(imageFixtures), name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	return body
}

func poster(path string) enrich.ArtworkRef {
	return enrich.ArtworkRef{Provider: enrich.ArtworkProviderTMDB, Kind: artwork.KindPoster, Path: path}
}

func newProxy(t *testing.T, options posterproxy.Options) (*posterproxy.Proxy, http.Handler, *fakeClock) {
	t.Helper()

	clock := &fakeClock{now: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)}
	options.Clock = clock.Now
	options.Logger = slog.New(slog.DiscardHandler)

	if options.Settings == nil {
		options.Settings = newSwitches(true, true)
	}

	proxy, err := posterproxy.New(options)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mux := http.NewServeMux()
	for _, mount := range proxy.Mounts() {
		mux.Handle(mount.Pattern, mount.Handler)
	}

	return proxy, mux, clock
}

func get(t *testing.T, handler http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))

	return recorder
}

func assertProblem(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) apigen.Problem {
	t.Helper()

	var problem apigen.Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil || recorder.Code != status || problem.Code != code {
		t.Fatalf("answer = %d %s, want %d %s", recorder.Code, recorder.Body, status, code)
	}

	return problem
}

// switches is a settable enrich.SettingsSource.
type switches struct {
	mutex    sync.Mutex
	settings enrich.OnlineSettings
	err      error
}

func newSwitches(online, tmdb bool) *switches {
	return &switches{settings: enrich.OnlineSettings{Enabled: online, TMDB: tmdb, MusicBrainz: true}}
}

func (source *switches) OnlineSettings(context.Context) (enrich.OnlineSettings, error) {
	source.mutex.Lock()
	defer source.mutex.Unlock()

	return source.settings, source.err
}

func (source *switches) set(online, tmdb bool) {
	source.mutex.Lock()
	defer source.mutex.Unlock()

	source.settings.Enabled, source.settings.TMDB = online, tmdb
}

func TestNewRequiresSourceSettingsAndLogger(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	for _, options := range []posterproxy.Options{
		{Logger: logger, Settings: newSwitches(true, true)},
		{Source: &fakeSource{}, Logger: logger},
		{Source: &fakeSource{}, Settings: newSwitches(true, true)},
	} {
		if _, err := posterproxy.New(options); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("New(%+v) error = %v, want ErrInvalid", options, err)
		}
	}
}

func TestPosterURLIsAMivoURLPerReference(t *testing.T) {
	t.Parallel()

	proxy, _, _ := newProxy(t, posterproxy.Options{Source: &fakeSource{}})

	first := proxy.PosterURL(poster("/a.jpg"))
	again := proxy.PosterURL(poster("/a.jpg"))
	other := proxy.PosterURL(poster("/b.jpg"))

	if !posterURLPattern.MatchString(first) || first != again || first == other || !posterURLPattern.MatchString(other) {
		t.Errorf("urls = %q, %q, %q", first, again, other)
	}

	cover := enrich.ArtworkRef{Provider: enrich.ArtworkProviderCoverArt, Kind: artwork.KindPoster, Path: "https://coverartarchive.org/x.jpg"}
	if got := proxy.PosterURL(cover); got != "" {
		t.Errorf("cover art url = %q, want none", got)
	}

	if got := proxy.PosterURL(poster("")); got != "" {
		t.Errorf("empty path url = %q, want none", got)
	}
}

func TestServePosterFetchesOnceAndCaches(t *testing.T) {
	t.Parallel()

	source := &fakeSource{body: readImage(t, "poster.jpg")}
	proxy, handler, _ := newProxy(t, posterproxy.Options{Source: source})
	target := proxy.PosterURL(poster("/a.jpg"))

	for range 3 {
		recorder := get(t, handler, target)
		if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "image/jpeg" ||
			recorder.Header().Get("Cache-Control") != "private, max-age=3600" || recorder.Body.Len() != len(source.body) {
			t.Fatalf("answer = %d %v (%d bytes)", recorder.Code, recorder.Header(), recorder.Body.Len())
		}
	}

	if source.callCount() != 1 {
		t.Errorf("previews = %d, want 1", source.callCount())
	}
}

func TestServePosterBeyondTheByteBudgetFetchesAgain(t *testing.T) {
	t.Parallel()

	source := &fakeSource{body: readImage(t, "poster.png")}
	proxy, handler, _ := newProxy(t, posterproxy.Options{Source: source, MaxCachedBytes: 1})
	target := proxy.PosterURL(poster("/a.png"))

	for range 2 {
		if recorder := get(t, handler, target); recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "image/png" {
			t.Fatalf("answer = %d %v", recorder.Code, recorder.Header())
		}
	}

	if source.callCount() != 2 {
		t.Errorf("previews = %d, want 2 (nothing kept)", source.callCount())
	}
}

func TestServePosterUnknownExpiredAndMalformedIDs(t *testing.T) {
	t.Parallel()

	source := &fakeSource{body: readImage(t, "poster.jpg")}
	proxy, handler, clock := newProxy(t, posterproxy.Options{Source: source, Lifetime: time.Minute})
	target := proxy.PosterURL(poster("/a.jpg"))

	assertProblem(t, get(t, handler, "/api/v1/metadata/posters/"+strings.Repeat("0", 32)), http.StatusNotFound, restapi.CodeNotFound)
	assertProblem(t, get(t, handler, "/api/v1/metadata/posters/NOT-AN-ID"), http.StatusBadRequest, restapi.CodeInvalid)
	assertProblem(t, get(t, handler, "/api/v1/metadata/posters/"+strings.Repeat("A", 32)), http.StatusBadRequest, restapi.CodeInvalid)

	clock.advance(time.Minute)
	assertProblem(t, get(t, handler, target), http.StatusNotFound, restapi.CodeNotFound)

	if source.callCount() != 0 {
		t.Errorf("previews = %d, want none", source.callCount())
	}

	if renewed := proxy.PosterURL(poster("/a.jpg")); renewed == target {
		t.Error("an expired reference kept its ID")
	}
}

func TestPosterURLDropsTheSoonestExpiringEntry(t *testing.T) {
	t.Parallel()

	source := &fakeSource{body: readImage(t, "poster.jpg")}
	proxy, handler, clock := newProxy(t, posterproxy.Options{Source: source, MaxEntries: 2})

	first := proxy.PosterURL(poster("/1.jpg"))
	clock.advance(time.Second)
	second := proxy.PosterURL(poster("/2.jpg"))
	clock.advance(time.Second)
	third := proxy.PosterURL(poster("/3.jpg"))

	assertProblem(t, get(t, handler, first), http.StatusNotFound, restapi.CodeNotFound)

	for _, target := range []string{second, third} {
		if recorder := get(t, handler, target); recorder.Code != http.StatusOK {
			t.Errorf("%s = %d, want 200", target, recorder.Code)
		}
	}
}

func TestServePosterMapsSourceFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		source         *fakeSource
		wantStatus     int
		wantCode       string
		wantRetryAfter string
	}{
		{"missing", &fakeSource{err: domain.ErrNotFound}, http.StatusNotFound, restapi.CodeNotFound, ""},
		{"not an image", &fakeSource{err: domain.ErrInvalid}, http.StatusNotFound, restapi.CodeNotFound, ""},
		{"foreign host", &fakeSource{err: domain.ErrForbidden}, http.StatusNotFound, restapi.CodeNotFound, ""},
		{"offline", &fakeSource{err: domain.ErrUnavailable}, http.StatusServiceUnavailable, restapi.CodeProviderUnavailable, "30"},
		{"rate limited", &fakeSource{err: domain.ErrRateLimited}, http.StatusServiceUnavailable, restapi.CodeProviderUnavailable, "30"},
		{"other", &fakeSource{err: errors.New("boom")}, http.StatusInternalServerError, restapi.CodeInternal, ""},
		{"undeclared format", &fakeSource{body: []byte("GIF89a not declared")}, http.StatusNotFound, restapi.CodeNotFound, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			proxy, handler, _ := newProxy(t, posterproxy.Options{Source: test.source})
			recorder := get(t, handler, proxy.PosterURL(poster("/a.jpg")))

			problem := assertProblem(t, recorder, test.wantStatus, test.wantCode)
			if got := recorder.Header().Get("Retry-After"); got != test.wantRetryAfter {
				t.Errorf("Retry-After = %q, want %q", got, test.wantRetryAfter)
			}

			if test.wantRetryAfter != "" && (problem.RetryAfterSeconds == nil || *problem.RetryAfterSeconds != 30) {
				t.Errorf("retryAfterSeconds = %v, want 30", problem.RetryAfterSeconds)
			}
		})
	}
}

func TestServePosterIgnoresACancelledRequest(t *testing.T) {
	t.Parallel()

	proxy, handler, _ := newProxy(t, posterproxy.Options{Source: &fakeSource{err: context.Canceled}})
	target := proxy.PosterURL(poster("/a.jpg"))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequestWithContext(ctx, http.MethodGet, target, nil))

	if body, _ := io.ReadAll(recorder.Body); len(body) != 0 {
		t.Errorf("cancelled request got a body: %s", body)
	}
}

func TestQuietPatternsNameTheRoute(t *testing.T) {
	t.Parallel()

	proxy, _, _ := newProxy(t, posterproxy.Options{Source: &fakeSource{}})
	if patterns := posterproxy.QuietPatterns(); len(patterns) != 1 || patterns[0] != proxy.Mounts()[0].Pattern {
		t.Errorf("QuietPatterns = %v, want the mount pattern", patterns)
	}
}
