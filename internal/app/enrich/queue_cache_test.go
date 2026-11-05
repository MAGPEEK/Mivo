package enrich_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

var errInvalidKey = fmt.Errorf("api key refused: %w", domain.ErrUnavailable)

type cacheHarness struct {
	clock  *fakeClock
	cache  *fakeCache
	health *enrich.Health
	movies *fakeMovies
	shows  *fakeShows
	music  *fakeMusic
}

func newCacheHarness() *cacheHarness {
	return &cacheHarness{
		clock:  newFakeClock(),
		cache:  newFakeCache(),
		health: enrich.NewHealth(enrich.HealthOptions{IsInvalidKey: func(err error) bool { return errors.Is(err, errInvalidKey) }}),
		movies: &fakeMovies{fakeTitles: fakeTitles{
			candidates: []match.Candidate{matrixCandidate()},
			details:    map[string]enrich.Details{matrixTMDBID: richDetails()},
		}},
		shows: &fakeShows{seasons: map[int]enrich.SeasonDetails{1: richSeason()}},
		music: &fakeMusic{fakeTitles: fakeTitles{candidates: []match.Candidate{matrixCandidate()}}},
	}
}

func (h *cacheHarness) deps() enrich.CacheDeps {
	return enrich.CacheDeps{Cache: h.cache, Health: h.health, Clock: h.clock.Now}
}

func (h *cacheHarness) cachedMovies() *enrich.CachedMovies {
	return enrich.NewCachedMovies(h.movies, h.deps())
}

func matrixRequest() enrich.DetailsRequest {
	return enrich.DetailsRequest{Ref: matrixCandidate().Ref, Language: "en"}
}

func richDetails() enrich.Details {
	released, _ := metadata.ParseDate("1999-03-31")

	return enrich.Details{
		Record: metadata.Record{
			Title: "The Matrix", Year: 1999, ReleaseDate: released, Genres: []string{"Action"},
			People:      []metadata.Credit{{Name: "Keanu Reeves", Role: metadata.RoleActor, Character: "Neo"}},
			ProviderIDs: []metadata.ProviderID{{Provider: metadata.ProviderTMDB, Value: matrixTMDBID}},
		},
		Artwork: []enrich.ArtworkRef{{Provider: enrich.ArtworkProviderTMDB, Kind: artwork.KindPoster, Path: "/poster.jpg"}},
	}
}

func richSeason() enrich.SeasonDetails {
	return enrich.SeasonDetails{
		Details:  enrich.Details{Record: metadata.Record{Title: "Season 1"}},
		Episodes: []enrich.EpisodeDetails{{Number: 1, Details: richDetails()}, {Number: 2}},
	}
}

func TestCachedDetailsReadsThroughAndServesFreshEntries(t *testing.T) {
	t.Parallel()

	h := newCacheHarness()
	movies := h.cachedMovies()

	first, err := movies.Details(context.Background(), matrixRequest())
	if err != nil {
		t.Fatalf("Details: %v", err)
	}

	_, entry, ok := h.cache.only()
	if !ok || entry.NotFound || !entry.ExpiresAt.Equal(queueStart().Add(enrich.CacheTTL)) {
		t.Fatalf("cached entry = %+v, want one answer valid for the cache TTL", entry)
	}

	h.clock.Advance(enrich.CacheTTL - time.Second)

	second, err := movies.Details(context.Background(), matrixRequest())
	if err != nil {
		t.Fatalf("Details from cache: %v", err)
	}

	if len(h.movies.detailsRequests) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(h.movies.detailsRequests))
	}

	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(second, richDetails()) {
		t.Fatalf("cached details = %+v, want %+v", second, richDetails())
	}
}

func TestCachedDetailsRefetchesExpiredEntries(t *testing.T) {
	t.Parallel()

	h := newCacheHarness()
	movies := h.cachedMovies()
	mustDetails(t, movies)

	h.clock.Advance(enrich.CacheTTL)
	mustDetails(t, movies)

	if len(h.movies.detailsRequests) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(h.movies.detailsRequests))
	}
}

func TestCachedDetailsCachesNotFound(t *testing.T) {
	t.Parallel()

	h := newCacheHarness()
	movies := h.cachedMovies()
	request := enrich.DetailsRequest{Ref: match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie, ID: "999"}}

	for range 2 {
		if _, err := movies.Details(context.Background(), request); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}

		h.clock.Advance(enrich.CacheTTL)
	}

	_, entry, _ := h.cache.only()
	if !entry.NotFound || !entry.ExpiresAt.Equal(queueStart().Add(enrich.NotFoundTTL)) {
		t.Fatalf("entry = %+v, want not found for the not-found TTL", entry)
	}

	if len(h.movies.detailsRequests) != 1 {
		t.Fatalf("provider calls = %d, want 1 within the not-found TTL", len(h.movies.detailsRequests))
	}

	if h.health.State(enrich.ProviderTMDB) != enrich.ProviderStateOK {
		t.Fatal("a not-found answer made the provider unhealthy")
	}
}

func TestCachedDetailsServesStaleWhenProviderFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want enrich.ProviderState
	}{
		{name: "offline", err: errUnavailable(), want: enrich.ProviderStateOffline},
		{name: "rate limited", err: fmt.Errorf("status 429: %w", domain.ErrRateLimited), want: enrich.ProviderStateRateLimited},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newCacheHarness()
			movies := h.cachedMovies()
			mustDetails(t, movies)

			h.clock.Advance(enrich.CacheTTL + time.Hour)
			h.movies.detailsErr = test.err

			got := mustDetails(t, movies)
			if !reflect.DeepEqual(got, richDetails()) {
				t.Fatalf("stale details = %+v, want the cached answer", got)
			}

			if state := h.health.State(enrich.ProviderTMDB); state != test.want {
				t.Fatalf("state = %s, want %s", state, test.want)
			}
		})
	}
}

func TestCachedDetailsFailsWithoutCacheWhenOffline(t *testing.T) {
	t.Parallel()

	h := newCacheHarness()
	h.movies.detailsErr = errUnavailable()

	if _, err := h.cachedMovies().Details(context.Background(), matrixRequest()); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}

	if h.cache.writes != 0 {
		t.Fatal("a failure was cached")
	}
}

func TestCachedLookupStopsCallingAfterInvalidKey(t *testing.T) {
	t.Parallel()

	h := newCacheHarness()
	movies := h.cachedMovies()
	mustDetails(t, movies)
	h.clock.Advance(enrich.CacheTTL)
	h.movies.searchErr = errInvalidKey

	if _, err := movies.Search(context.Background(), enrich.SearchQuery{Title: "Heat"}); !errors.Is(err, errInvalidKey) {
		t.Fatalf("err = %v, want the invalid key", err)
	}

	calls := h.movies.calls()

	// Disabled: no further call, a stale entry is still served, a missing one fails.
	if got := mustDetails(t, movies); !reflect.DeepEqual(got, richDetails()) {
		t.Fatalf("stale details = %+v, want the cached answer", got)
	}

	if _, err := movies.Search(context.Background(), enrich.SearchQuery{Title: "Heat"}); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}

	if h.movies.calls() != calls || h.health.State(enrich.ProviderTMDB) != enrich.ProviderStateDisabled {
		t.Fatalf("calls %d -> %d, state %s; want no call while disabled", calls, h.movies.calls(), h.health.State(enrich.ProviderTMDB))
	}

	h.health.SettingsChanged()
	h.movies.searchErr = nil

	if _, err := movies.Search(context.Background(), enrich.SearchQuery{Title: "Heat"}); err != nil {
		t.Fatalf("Search after a settings change: %v", err)
	}
}

// Parts containing the separator must not let two different queries share a key.
func TestCachedSearchKeysDoNotCollide(t *testing.T) {
	t.Parallel()

	h := newCacheHarness()
	movies := h.cachedMovies()

	for _, query := range []enrich.SearchQuery{{Title: "t", Language: "en|0"}, {Title: "0|t", Language: "en"}} {
		if _, err := movies.Search(context.Background(), query); err != nil {
			t.Fatalf("Search(%+v): %v", query, err)
		}
	}

	if h.movies.searchCalls != 2 || len(h.cache.entries) != 2 {
		t.Fatalf("provider calls = %d, entries = %d; want two separate answers", h.movies.searchCalls, len(h.cache.entries))
	}
}

func TestCachedLookupTreatsBrokenCacheAsMiss(t *testing.T) {
	t.Parallel()

	h := newCacheHarness()
	h.cache.readErr = errors.New("disk I/O error")
	h.cache.writeErr = errors.New("disk full")

	mustDetails(t, h.cachedMovies())

	h = newCacheHarness()
	movies := h.cachedMovies()
	mustDetails(t, movies)

	key, entry, _ := h.cache.only()
	entry.Payload = []byte(`{"Record":{"ReleaseDate":"not a date"}}`)
	h.cache.entries[key] = entry

	if got := mustDetails(t, movies); !reflect.DeepEqual(got, richDetails()) {
		t.Fatalf("details = %+v, want them fetched again", got)
	}

	if len(h.movies.detailsRequests) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(h.movies.detailsRequests))
	}
}

func TestCachedShowsAndMusicRoundTrip(t *testing.T) {
	t.Parallel()

	h := newCacheHarness()
	h.shows.details = map[string]enrich.Details{matrixTMDBID: richDetails()}
	h.music.details = map[string]enrich.Details{matrixTMDBID: richDetails()}
	shows := enrich.NewCachedShows(h.shows, h.deps())
	music := enrich.NewCachedMusic(h.music, h.deps())
	ctx := context.Background()

	for range 2 {
		season, err := shows.SeasonDetails(ctx, enrich.SeasonRequest{Show: matrixCandidate().Ref, Season: 1})
		if err != nil || !reflect.DeepEqual(season, richSeason()) {
			t.Fatalf("SeasonDetails = %+v, %v; want %+v", season, err, richSeason())
		}

		if _, err := shows.Details(ctx, matrixRequest()); err != nil {
			t.Fatalf("show Details: %v", err)
		}

		if _, err := shows.Search(ctx, enrich.SearchQuery{Title: "Matrix"}); err != nil {
			t.Fatalf("show Search: %v", err)
		}

		candidates, err := music.Search(ctx, enrich.MusicQuery{Kind: match.RefKindRelease, Title: "Matrix"})
		if err != nil || !reflect.DeepEqual(candidates, []match.Candidate{matrixCandidate()}) {
			t.Fatalf("music Search = %+v, %v", candidates, err)
		}

		if _, err := music.Details(ctx, matrixRequest()); err != nil {
			t.Fatalf("music Details: %v", err)
		}
	}

	if h.shows.calls() != 3 || h.music.calls() != 2 {
		t.Fatalf("show calls = %d, music calls = %d; want each answer fetched once", h.shows.calls(), h.music.calls())
	}
}

func TestCachedSeasonRejectsBrokenEpisode(t *testing.T) {
	t.Parallel()

	h := newCacheHarness()
	shows := enrich.NewCachedShows(h.shows, h.deps())
	request := enrich.SeasonRequest{Show: matrixCandidate().Ref, Season: 1}

	if _, err := shows.SeasonDetails(context.Background(), request); err != nil {
		t.Fatalf("SeasonDetails: %v", err)
	}

	key, entry, _ := h.cache.only()
	entry.Payload = []byte(`{"Season":{"Record":{}},"Episodes":[{"Number":1,"Details":{"Record":{"ReleaseDate":"x"}}}]}`)
	h.cache.entries[key] = entry

	if _, err := shows.SeasonDetails(context.Background(), request); err != nil {
		t.Fatalf("SeasonDetails: %v", err)
	}

	if len(h.shows.seasonRequests) != 2 {
		t.Fatalf("season calls = %d, want the broken entry fetched again", len(h.shows.seasonRequests))
	}
}

func TestObservedArtwork(t *testing.T) {
	t.Parallel()

	health := enrich.NewHealth(enrich.HealthOptions{IsInvalidKey: func(err error) bool { return errors.Is(err, errInvalidKey) }})
	downloader := &fakeDownloader{failures: map[string]bool{"/down.jpg": true}}
	observed := enrich.NewObservedArtwork(downloader, health)
	ref := enrich.ArtworkRef{Provider: enrich.ArtworkProviderCoverArt, Kind: artwork.KindPoster, Path: "/down.jpg"}

	if _, err := observed.Fetch(context.Background(), ref); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}

	if health.State(enrich.ProviderCoverArt) != enrich.ProviderStateOffline {
		t.Fatalf("state = %s, want offline", health.State(enrich.ProviderCoverArt))
	}

	health.Observe(enrich.ProviderCoverArt, errInvalidKey)
	ref.Path = "/up.jpg"

	if _, err := observed.Fetch(context.Background(), ref); !errors.Is(err, domain.ErrUnavailable) || len(downloader.fetched) != 1 {
		t.Fatalf("disabled provider: err = %v, downloads = %d; want refused without a call", err, len(downloader.fetched))
	}
}

func mustDetails(t *testing.T, movies *enrich.CachedMovies) enrich.Details {
	t.Helper()

	details, err := movies.Details(context.Background(), matrixRequest())
	if err != nil {
		t.Fatalf("Details: %v", err)
	}

	return details
}
