package tmdb_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/metadata/tmdb"
	"github.com/magpeek/mivo/internal/adapters/netaccess/outbound"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

func TestHTTPStatusMapping(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		fixture  string
		want     error
		attempts int
	}{
		{"401 invalid key", http.StatusUnauthorized, "error_401.json", tmdb.ErrInvalidKey, 1},
		{"404 not found", http.StatusNotFound, "error_404.json", domain.ErrNotFound, 1},
		{"429 after retries", http.StatusTooManyRequests, "error_429.json", domain.ErrRateLimited, 2},
		{"500 unavailable", http.StatusInternalServerError, "error_404.json", domain.ErrUnavailable, 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := newFakeTMDB(t, route{path: "/3/movie/603", status: test.status, fixture: test.fixture})

			_, err := newClient(fake, testKey).Movies().Details(t.Context(), enrich.DetailsRequest{Ref: tmdbRef(match.RefKindMovie, "603")})
			if !errors.Is(err, test.want) {
				t.Fatalf("err = %v, want %v", err, test.want)
			}

			assertChainWithoutKey(t, err)

			if got := len(fake.recorded()); got != test.attempts {
				t.Errorf("attempts = %d, want %d", got, test.attempts)
			}
		})
	}
}

func TestInvalidKeyIsUnavailable(t *testing.T) {
	if !errors.Is(tmdb.ErrInvalidKey, domain.ErrUnavailable) {
		t.Error("ErrInvalidKey must wrap domain.ErrUnavailable")
	}
}

func TestUnreadableAnswerIsUnavailable(t *testing.T) {
	fake := newFakeTMDB(t, route{path: "/3/search/movie", fixture: "broken.json"})

	_, err := newClient(fake, testKey).Movies().Search(t.Context(), enrich.SearchQuery{Title: "x"})
	if !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
}

func TestReadAccessTokenGoesIntoTheHeader(t *testing.T) {
	fake := newFakeTMDB(t, route{path: "/3/search/movie", fixture: "search_empty.json"})

	if _, err := newClient(fake, testToken).Movies().Search(t.Context(), enrich.SearchQuery{Title: "x"}); err != nil {
		t.Fatalf("Search: %v", err)
	}

	request := fake.recorded()[0]
	if request.query.Has("api_key") || request.header.Get("Authorization") != "Bearer "+testToken {
		t.Errorf("query %v, authorization %q", request.query, request.header.Get("Authorization"))
	}
}

func TestMissingOrMalformedKeySendsNothing(t *testing.T) {
	tests := []struct {
		name string
		keys tmdb.KeyProvider
		want error
	}{
		{"no provider", nil, tmdb.ErrInvalidKey},
		{"empty key", staticKey{}, tmdb.ErrInvalidKey},
		{"key with a line break", staticKey{key: "abc\r\nX-Evil: 1"}, tmdb.ErrInvalidKey},
		{"key with a space", staticKey{key: "abc def"}, tmdb.ErrInvalidKey},
		{"provider error", staticKey{err: domain.ErrUnavailable}, domain.ErrUnavailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := newFakeTMDB(t)
			client := tmdb.New(tmdb.Options{HTTP: newOutbound(), Keys: test.keys, BaseURL: fake.server.URL + "/3"})

			_, err := client.Movies().Search(t.Context(), enrich.SearchQuery{Title: "x"})
			if !errors.Is(err, test.want) || len(fake.recorded()) != 0 {
				t.Errorf("err = %v after %d requests, want %v without a request", err, len(fake.recorded()), test.want)
			}

			if strings.Contains(err.Error(), "Evil") {
				t.Errorf("error %q echoes the key", err)
			}
		})
	}
}

// leakyGetter fails with an error that contains the whole URL, key included.
type leakyGetter struct{}

func (leakyGetter) Get(_ context.Context, rawURL string, _ http.Header) (*outbound.Response, error) {
	return nil, errors.New("dial " + rawURL + ": " + domain.ErrUnavailable.Error())
}

type wrappingGetter struct{}

func (wrappingGetter) Get(_ context.Context, rawURL string, _ http.Header) (*outbound.Response, error) {
	return nil, &urlError{url: rawURL}
}

type urlError struct{ url string }

func (e *urlError) Error() string { return "GET " + e.url + ": unavailable" }
func (e *urlError) Unwrap() error { return domain.ErrUnavailable }

func TestTransportErrorsNeverCarryTheKey(t *testing.T) {
	for _, getter := range []tmdb.HTTPGetter{leakyGetter{}, wrappingGetter{}} {
		client := tmdb.New(tmdb.Options{HTTP: getter, Keys: staticKey{key: testKey}})

		_, err := client.Movies().Search(t.Context(), enrich.SearchQuery{Title: "x"})
		if err == nil {
			t.Fatal("Search succeeded, want an error")
		}

		assertChainWithoutKey(t, err)
	}

	client := tmdb.New(tmdb.Options{HTTP: wrappingGetter{}, Keys: staticKey{key: testKey}})
	if _, err := client.Movies().Search(t.Context(), enrich.SearchQuery{Title: "x"}); !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("err = %v, want the sentinel chain kept", err)
	}
}

// assertChainWithoutKey walks the whole unwrap tree: a caller unwrapping the error must not
// reach a message with the key either.
func assertChainWithoutKey(t *testing.T, err error) {
	t.Helper()

	pending := []error{err}
	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]

		if strings.Contains(current.Error(), testKey) {
			t.Errorf("error %q in the chain leaks the key", current)
		}

		switch unwrapper := current.(type) { //nolint:errorlint // walks the chain by hand on purpose
		case interface{ Unwrap() error }:
			if inner := unwrapper.Unwrap(); inner != nil {
				pending = append(pending, inner)
			}
		case interface{ Unwrap() []error }:
			pending = append(pending, unwrapper.Unwrap()...)
		}
	}
}

func TestImageURL(t *testing.T) {
	client := tmdb.New(tmdb.Options{})

	tests := []struct {
		name string
		ref  enrich.ArtworkRef
		want string
	}{
		{"poster", enrich.ArtworkRef{Provider: enrich.ArtworkProviderTMDB, Kind: artwork.KindPoster, Path: "/a.jpg"}, "https://image.tmdb.org/t/p/w780/a.jpg"},
		{"backdrop", enrich.ArtworkRef{Provider: enrich.ArtworkProviderTMDB, Kind: artwork.KindBackdrop, Path: "/b.jpg"}, "https://image.tmdb.org/t/p/w1280/b.jpg"},
		{"logo", enrich.ArtworkRef{Provider: enrich.ArtworkProviderTMDB, Kind: artwork.KindLogo, Path: "/c.png"}, "https://image.tmdb.org/t/p/original/c.png"},
		{"still", enrich.ArtworkRef{Provider: enrich.ArtworkProviderTMDB, Kind: artwork.KindThumb, Path: "/d.jpg"}, "https://image.tmdb.org/t/p/original/d.jpg"},
		{"unknown kind", enrich.ArtworkRef{Provider: enrich.ArtworkProviderTMDB, Kind: "square", Path: "/e.jpg"}, "https://image.tmdb.org/t/p/original/e.jpg"},
	}

	for _, test := range tests {
		got, err := client.ImageURL(test.ref)
		if err != nil || got != test.want {
			t.Errorf("%s: ImageURL = %q, %v; want %q", test.name, got, err, test.want)
		}
	}
}

func TestImageURLRejectsUnsafeRefs(t *testing.T) {
	client := tmdb.New(tmdb.Options{})

	for _, ref := range []enrich.ArtworkRef{
		{Provider: enrich.ArtworkProviderCoverArt, Kind: artwork.KindPoster, Path: "/a.jpg"},
		{Provider: enrich.ArtworkProviderTMDB, Kind: artwork.KindPoster, Path: "a.jpg"},
		{Provider: enrich.ArtworkProviderTMDB, Kind: artwork.KindPoster, Path: "/../a.jpg"},
		{Provider: enrich.ArtworkProviderTMDB, Kind: artwork.KindPoster, Path: "/..jpg"},
		{Provider: enrich.ArtworkProviderTMDB, Kind: artwork.KindPoster, Path: "/a.jpg?x=1"},
		{Provider: enrich.ArtworkProviderTMDB, Kind: artwork.KindPoster, Path: "/"},
		{Provider: enrich.ArtworkProviderTMDB, Kind: artwork.KindPoster, Path: "/" + strings.Repeat("a", 200)},
	} {
		if _, err := client.ImageURL(ref); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("ImageURL(%+v) err = %v, want ErrInvalid", ref, err)
		}
	}
}

func TestAllowedHosts(t *testing.T) {
	hosts := tmdb.AllowedHosts()
	if len(hosts) != 2 || hosts[0] != "api.themoviedb.org" || hosts[1] != "image.tmdb.org" {
		t.Errorf("AllowedHosts = %v", hosts)
	}
}
