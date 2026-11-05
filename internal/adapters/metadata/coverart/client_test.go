package coverart_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/metadata/coverart"
	"github.com/magpeek/mivo/internal/adapters/netaccess/outbound"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/platform/brand"
)

const (
	fixtureDir   = "../../../../testdata/providers/coverart"
	groupID      = "1b022e01-4da6-387b-8658-8678046e4cef"
	releaseID    = "76df3287-6cda-33eb-8e9a-044b5e15ffdd"
	groupPath    = "/release-group/" + groupID
	releasePath  = "/release/" + releaseID
	frontOnlyURL = "https://coverartarchive.org/release/76df3287-6cda-33eb-8e9a-044b5e15ffdd/829521842-1200.jpg"
)

// route is one fake answer: a fixture with a status, or a redirect when location is set.
type route struct {
	path     string
	status   int
	fixture  string
	location string
}

// fakeArchive serves fixtures and records every request.
type fakeArchive struct {
	t      *testing.T
	routes []route
	server *httptest.Server

	mu       sync.Mutex
	requests []*http.Request
}

func newFakeArchive(t *testing.T, routes ...route) *fakeArchive {
	t.Helper()

	fake := &fakeArchive{t: t, routes: routes}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(fake.server.Close)

	return fake
}

func (fake *fakeArchive) serve(writer http.ResponseWriter, request *http.Request) {
	fake.mu.Lock()
	fake.requests = append(fake.requests, request.Clone(request.Context()))
	fake.mu.Unlock()

	index := slices.IndexFunc(fake.routes, func(candidate route) bool { return candidate.path == request.URL.Path })
	if index < 0 {
		fake.t.Errorf("unexpected request %s", request.URL)
		http.NotFound(writer, request)

		return
	}

	answer := fake.routes[index]
	if answer.location != "" {
		http.Redirect(writer, request, answer.location, http.StatusTemporaryRedirect)

		return
	}

	var body []byte

	if answer.fixture != "" {
		var err error

		body, err = os.ReadFile(filepath.Join(fixtureDir, answer.fixture))
		if err != nil {
			fake.t.Errorf("read fixture %s: %v", answer.fixture, err)
		}
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(max(answer.status, http.StatusOK))
	_, _ = writer.Write(body)
}

func (fake *fakeArchive) paths() []string {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	paths := make([]string, 0, len(fake.requests))
	for _, request := range fake.requests {
		paths = append(paths, request.URL.Path)
	}

	return paths
}

func (fake *fakeArchive) client() *coverart.Client {
	return coverart.New(coverart.Options{
		HTTP: outbound.New(outbound.Options{
			AllowInsecureLoopback: true,
			MaxAttempts:           2,
			Sleep:                 func(context.Context, time.Duration) error { return nil },
		}),
		BaseURL: fake.server.URL + "/",
	})
}

func poster(address string) enrich.ArtworkRef {
	return enrich.ArtworkRef{Provider: enrich.ArtworkProviderCoverArt, Kind: artwork.KindPoster, Path: address}
}

func TestCoversListsFrontCovers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		routes    []route
		request   coverart.CoverRequest
		want      []enrich.ArtworkRef
		wantPaths []string
	}{
		{
			name:      "front only, http rewritten to https",
			routes:    []route{{path: groupPath, fixture: "front_only.json"}},
			request:   coverart.CoverRequest{ReleaseGroupID: groupID, ReleaseID: releaseID},
			want:      []enrich.ArtworkRef{poster(frontOnlyURL)},
			wantPaths: []string{groupPath},
		},
		{
			name:    "front and back keeps the front with its 500 thumbnail",
			routes:  []route{{path: releasePath, fixture: "front_back.json"}},
			request: coverart.CoverRequest{ReleaseID: strings.ToUpper(releaseID)},
			want: []enrich.ArtworkRef{poster(
				"https://coverartarchive.org/release/b84ee12a-09ef-421b-82de-0441a926375b/5769316809-500.jpg")},
			wantPaths: []string{releasePath},
		},
		{
			name: "release group missing falls back to the release",
			routes: []route{
				{path: groupPath, status: http.StatusNotFound},
				{path: releasePath, fixture: "front_only.json"},
			},
			request:   coverart.CoverRequest{ReleaseGroupID: groupID, ReleaseID: releaseID},
			want:      []enrich.ArtworkRef{poster(frontOnlyURL)},
			wantPaths: []string{groupPath, releasePath},
		},
		{
			name: "redirected listing is followed",
			routes: []route{
				{path: groupPath, location: releasePath},
				{path: releasePath, fixture: "front_only.json"},
			},
			request:   coverart.CoverRequest{ReleaseGroupID: groupID},
			want:      []enrich.ArtworkRef{poster(frontOnlyURL)},
			wantPaths: []string{groupPath, releasePath},
		},
		{
			name:    "untrusted urls and unapproved images are dropped",
			routes:  []route{{path: groupPath, fixture: "untrusted_urls.json"}},
			request: coverart.CoverRequest{ReleaseGroupID: groupID},
			want: []enrich.ArtworkRef{poster(
				"https://coverartarchive.org/release/0a1b2c3d-0000-4000-8000-000000000001/4.jpg")},
			wantPaths: []string{groupPath},
		},
		{
			name: "no covers anywhere is not an error",
			routes: []route{
				{path: groupPath, status: http.StatusNotFound},
				{path: releasePath, status: http.StatusNotFound},
			},
			request:   coverart.CoverRequest{ReleaseGroupID: groupID, ReleaseID: releaseID},
			wantPaths: []string{groupPath, releasePath},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fake := newFakeArchive(t, test.routes...)

			got, err := fake.client().Covers(t.Context(), test.request)
			if err != nil {
				t.Fatalf("Covers() error = %v", err)
			}

			if !slices.Equal(got, test.want) {
				t.Errorf("Covers() = %v, want %v", got, test.want)
			}

			if paths := fake.paths(); !slices.Equal(paths, test.wantPaths) {
				t.Errorf("requested %v, want %v", paths, test.wantPaths)
			}
		})
	}
}

func TestCoversErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		routes  []route
		request coverart.CoverRequest
		want    error
	}{
		{"429", []route{{path: groupPath, status: http.StatusTooManyRequests}}, coverart.CoverRequest{ReleaseGroupID: groupID}, domain.ErrRateLimited},
		{"503", []route{{path: groupPath, status: http.StatusServiceUnavailable}}, coverart.CoverRequest{ReleaseGroupID: groupID}, domain.ErrRateLimited},
		{"500", []route{{path: groupPath, status: http.StatusInternalServerError}}, coverart.CoverRequest{ReleaseGroupID: groupID}, domain.ErrUnavailable},
		{"broken json", []route{{path: groupPath, fixture: "broken.json"}}, coverart.CoverRequest{ReleaseGroupID: groupID}, domain.ErrUnavailable},
		{"no id", nil, coverart.CoverRequest{}, domain.ErrInvalid},
		{"path in id", nil, coverart.CoverRequest{ReleaseGroupID: "../../../etc/passwd/aaaaaaaaaaaaaaaaa"}, domain.ErrInvalid},
		{"bad release id", nil, coverart.CoverRequest{ReleaseGroupID: groupID, ReleaseID: "abc"}, domain.ErrInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fake := newFakeArchive(t, test.routes...)

			got, err := fake.client().Covers(t.Context(), test.request)
			if !errors.Is(err, test.want) {
				t.Fatalf("Covers() error = %v, want %v", err, test.want)
			}

			if got != nil {
				t.Errorf("Covers() = %v, want nil", got)
			}

			if test.routes == nil && len(fake.paths()) != 0 {
				t.Errorf("invalid request reached the archive: %v", fake.paths())
			}
		})
	}
}

func TestCoversCancelledContext(t *testing.T) {
	t.Parallel()

	fake := newFakeArchive(t, route{path: groupPath, fixture: "front_only.json"})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := fake.client().Covers(ctx, coverart.CoverRequest{ReleaseGroupID: groupID}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Covers() error = %v, want context.Canceled", err)
	}
}

func TestCoversSendsOnlyBrandUserAgentAndID(t *testing.T) {
	t.Parallel()

	fake := newFakeArchive(t, route{path: groupPath, fixture: "front_only.json"})

	if _, err := fake.client().Covers(t.Context(), coverart.CoverRequest{ReleaseGroupID: groupID}); err != nil {
		t.Fatalf("Covers() error = %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()

	request := fake.requests[0]
	if agent := request.UserAgent(); !strings.HasPrefix(agent, brand.DisplayName+"/") || strings.Contains(agent, "@") {
		t.Errorf("User-Agent = %q, want the brand without an email address", agent)
	}

	if request.URL.RawQuery != "" || request.Header.Get("Cookie") != "" {
		t.Errorf("request carried query %q or cookies", request.URL.RawQuery)
	}
}

func TestHosts(t *testing.T) {
	t.Parallel()

	if got := coverart.AllowedHosts(); !slices.Equal(got, []string{"coverartarchive.org"}) {
		t.Errorf("AllowedHosts() = %v", got)
	}

	if got := coverart.RedirectHosts(); !slices.Equal(got, []string{"coverartarchive.org", ".archive.org"}) {
		t.Errorf("RedirectHosts() = %v", got)
	}

	if coverart.New(coverart.Options{}) == nil {
		t.Error("New() with defaults returned nil")
	}
}
