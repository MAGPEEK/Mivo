package identify_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/app/identify"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

const testMBID = "c0b1a2d3-e4f5-4678-9abc-def012345678"

func TestSearchRanksMovieCandidatesBestFirst(t *testing.T) {
	h := newHarness()
	h.addItem("movie", library.ItemKindMovie, enrich.ItemSnapshot{Language: "de"})
	h.movies.candidates = []match.Candidate{
		{Ref: match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie, ID: "1"}, Title: "Big Buck Bunny Returns", Year: 2015},
		{Ref: match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie, ID: "2"}, Title: "Big Buck Bunny", Year: 2008},
	}

	got, err := h.service().Search(context.Background(), "movie", identify.Query{Text: " Big Buck Bunny ", Year: 2008})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(got) != 2 || got[0].Candidate.Ref.ID != "2" || got[0].Confidence <= got[1].Confidence {
		t.Fatalf("candidates = %+v, want ID 2 first with the higher confidence", got)
	}

	want := enrich.SearchQuery{Title: "Big Buck Bunny", Year: 2008, Language: "de"}
	if len(h.movies.searches) != 1 || h.movies.searches[0] != want {
		t.Fatalf("movie searches = %+v, want %+v", h.movies.searches, want)
	}
}

func TestSearchShowChildrenSearchShowsAtShowLevel(t *testing.T) {
	for _, kind := range []library.ItemKind{library.ItemKindShow, library.ItemKindSeason, library.ItemKindEpisode} {
		t.Run(string(kind), func(t *testing.T) {
			h := newHarness()
			h.addItem("item", kind, enrich.ItemSnapshot{})
			h.shows.candidates = []match.Candidate{{Ref: match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindShow, ID: "7"}, Title: "Sintel"}}

			got, err := h.service().Search(context.Background(), "item", identify.Query{Text: "Sintel"})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}

			if len(got) != 1 || len(h.shows.searches) != 1 || h.movies.calls() != 0 {
				t.Fatalf("got %+v, show searches %d, movie calls %d", got, len(h.shows.searches), h.movies.calls())
			}

			if h.shows.searches[0].Language != "en" {
				t.Fatalf("language = %q, want the global default", h.shows.searches[0].Language)
			}
		})
	}
}

func TestSearchMusicUsesKindAndArtist(t *testing.T) {
	tests := []struct {
		kind library.ItemKind
		want enrich.MusicQuery
	}{
		{library.ItemKindAlbum, enrich.MusicQuery{Kind: match.RefKindRelease, Title: "Abbey Road", Artist: "The Beatles", Year: 1969}},
		{library.ItemKindArtist, enrich.MusicQuery{Kind: match.RefKindArtist, Title: "Abbey Road", Year: 1969}},
	}

	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			h := newHarness()
			h.addItem("music", test.kind, enrich.ItemSnapshot{ArtistName: "The Beatles"})

			if _, err := h.service().Search(context.Background(), "music", identify.Query{Text: "Abbey Road", Year: 1969}); err != nil {
				t.Fatalf("Search: %v", err)
			}

			if len(h.music.musicSearches) != 1 || h.music.musicSearches[0] != test.want {
				t.Fatalf("music searches = %+v, want %+v", h.music.musicSearches, test.want)
			}
		})
	}
}

func TestSearchCapsCandidates(t *testing.T) {
	h := newHarness()
	h.addItem("movie", library.ItemKindMovie, enrich.ItemSnapshot{})

	for index := range identify.MaxCandidates + 5 {
		h.movies.candidates = append(h.movies.candidates, match.Candidate{
			Ref: match.ProviderRef{Provider: match.RefTMDB, ID: strconv.Itoa(index + 1)}, Title: "Title",
		})
	}

	got, err := h.service().Search(context.Background(), "movie", identify.Query{Text: "Title"})
	if err != nil || len(got) != identify.MaxCandidates {
		t.Fatalf("Search = %d candidates, %v; want %d", len(got), err, identify.MaxCandidates)
	}
}

func TestSearchByPastedReference(t *testing.T) {
	matrix := enrich.Details{Record: metadata.Record{Title: "The Matrix", OriginalTitle: "The Matrix", Year: 1999, ProviderIDs: []metadata.ProviderID{tmdbID("603")}}}

	tests := []struct {
		name        string
		kind        library.ItemKind
		pasted      string
		wantRequest match.ProviderRef
		wantRef     match.ProviderRef
	}{
		{
			name: "tmdb url", kind: library.ItemKindMovie, pasted: "https://www.themoviedb.org/movie/603-the-matrix",
			wantRequest: match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie, ID: "603"},
			wantRef:     match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie, ID: "603"},
		},
		{
			name: "bare tmdb id completes the kind", kind: library.ItemKindMovie, pasted: "603",
			wantRequest: match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie, ID: "603"},
			wantRef:     match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie, ID: "603"},
		},
		{
			name: "imdb id becomes the tmdb id of the entry", kind: library.ItemKindMovie, pasted: "tt0133093",
			wantRequest: match.ProviderRef{Provider: match.RefIMDb, ID: "tt0133093"},
			wantRef:     match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie, ID: "603"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness()
			h.addItem("movie", test.kind, enrich.ItemSnapshot{})
			h.movies.details[test.wantRequest.ID] = matrix

			got, err := h.service().Search(context.Background(), "movie", identify.Query{Text: "ignored", ProviderRef: test.pasted})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}

			if len(h.movies.detailsRequests) != 1 || h.movies.detailsRequests[0].Ref != test.wantRequest || len(h.movies.searches) != 0 {
				t.Fatalf("details requests = %+v, searches = %d", h.movies.detailsRequests, len(h.movies.searches))
			}

			want := match.Scored{
				Candidate:  match.Candidate{Ref: test.wantRef, Title: "The Matrix", OriginalTitle: "The Matrix", Year: 1999},
				Confidence: 1, ExactID: true,
			}
			if len(got) != 1 || got[0] != want {
				t.Fatalf("candidates = %+v, want %+v", got, want)
			}
		})
	}
}

func TestSearchPastedImdbWithoutTmdbIDKeepsImdb(t *testing.T) {
	h := newHarness()
	h.addItem("show", library.ItemKindShow, enrich.ItemSnapshot{})
	h.shows.details["tt0903747"] = enrich.Details{Record: metadata.Record{Title: "Breaking Bad"}}

	got, err := h.service().Search(context.Background(), "show", identify.Query{ProviderRef: "https://www.imdb.com/title/tt0903747/"})
	if err != nil || len(got) != 1 || got[0].Candidate.Ref != (match.ProviderRef{Provider: match.RefIMDb, ID: "tt0903747"}) {
		t.Fatalf("Search = %+v, %v", got, err)
	}
}

func TestSearchPastedMusicBrainzRelease(t *testing.T) {
	h := newHarness()
	h.addItem("album", library.ItemKindAlbum, enrich.ItemSnapshot{})
	h.music.details[testMBID] = enrich.Details{Record: metadata.Record{Title: "Abbey Road"}}

	got, err := h.service().Search(context.Background(), "album", identify.Query{ProviderRef: "https://musicbrainz.org/release/" + testMBID})
	want := match.ProviderRef{Provider: match.RefMusicBrainz, Kind: match.RefKindRelease, ID: testMBID}

	if err != nil || len(got) != 1 || got[0].Candidate.Ref != want {
		t.Fatalf("Search = %+v, %v; want ref %+v", got, err, want)
	}
}

func TestSearchUnknownPastedReferenceReturnsNoCandidate(t *testing.T) {
	h := newHarness()
	h.addItem("movie", library.ItemKindMovie, enrich.ItemSnapshot{})

	got, err := h.service().Search(context.Background(), "movie", identify.Query{ProviderRef: "999999"})
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("Search = %#v, %v; want an empty list", got, err)
	}
}

func TestSearchWithoutResultsReturnsEmptyList(t *testing.T) {
	h := newHarness()
	h.addItem("movie", library.ItemKindMovie, enrich.ItemSnapshot{})

	got, err := h.service().Search(context.Background(), "movie", identify.Query{Text: "Nothing"})
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("Search = %#v, %v; want an empty, non-nil list", got, err)
	}
}

func TestSearchRejectsUnusableInput(t *testing.T) {
	tests := []struct {
		name  string
		kind  library.ItemKind
		query identify.Query
	}{
		{"empty query", library.ItemKindMovie, identify.Query{Text: "  "}},
		{"garbage reference", library.ItemKindMovie, identify.Query{ProviderRef: "https://example.com/movie/1"}},
		{"show url for a movie", library.ItemKindMovie, identify.Query{ProviderRef: "https://www.themoviedb.org/tv/1396"}},
		{"musicbrainz id for a movie", library.ItemKindMovie, identify.Query{ProviderRef: testMBID}},
		{"tmdb id for an album", library.ItemKindAlbum, identify.Query{ProviderRef: "603"}},
		{"artist url for an album", library.ItemKindAlbum, identify.Query{ProviderRef: "https://musicbrainz.org/artist/" + testMBID}},
		{"kind without provider", library.ItemKindTrack, identify.Query{Text: "Song"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness()
			h.addItem("item", test.kind, enrich.ItemSnapshot{})

			_, err := h.service().Search(context.Background(), "item", test.query)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("Search error = %v, want ErrInvalid", err)
			}

			if h.movies.calls()+h.shows.calls()+h.music.calls() != 0 {
				t.Fatal("a provider was asked for unusable input")
			}
		})
	}
}

func TestSearchOfflineReturnsUnavailable(t *testing.T) {
	offline := fmt.Errorf("tmdb: %w", domain.ErrUnavailable)

	for _, query := range []identify.Query{{Text: "Sintel"}, {ProviderRef: "45745"}} {
		h := newHarness()
		h.addItem("movie", library.ItemKindMovie, enrich.ItemSnapshot{})
		h.movies.searchErr, h.movies.detailsErr = offline, offline

		if _, err := h.service().Search(context.Background(), "movie", query); !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("Search(%+v) error = %v, want ErrUnavailable", query, err)
		}
	}
}

func TestSearchRefusesWhenOnlineMetadataIsOff(t *testing.T) {
	tests := []struct {
		name   string
		change func(h *harness)
	}{
		{"global switch", func(h *harness) { h.settings.settings.Enabled = false }},
		{"provider switch", func(h *harness) { h.settings.settings.TMDB = false }},
		{"library switch", func(h *harness) {
			snapshot := h.store.items["movie"]
			snapshot.OnlineEnabled = false
			h.store.items["movie"] = snapshot
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness()
			h.addItem("movie", library.ItemKindMovie, enrich.ItemSnapshot{})
			test.change(h)

			if _, err := h.service().Search(context.Background(), "movie", identify.Query{Text: "Sintel"}); !errors.Is(err, domain.ErrConflict) {
				t.Fatalf("Search error = %v, want ErrConflict", err)
			}

			if h.movies.calls() != 0 {
				t.Fatal("a switched-off provider was asked")
			}
		})
	}
}

func TestSearchRefusesUnwiredProviders(t *testing.T) {
	tests := []struct {
		kind  library.ItemKind
		unset func(deps *identify.Deps)
	}{
		{library.ItemKindMovie, func(deps *identify.Deps) { deps.Movies = nil }},
		{library.ItemKindShow, func(deps *identify.Deps) { deps.Shows = nil }},
		{library.ItemKindAlbum, func(deps *identify.Deps) { deps.Music = nil }},
	}

	for _, test := range tests {
		h := newHarness()
		h.addItem("item", test.kind, enrich.ItemSnapshot{})
		deps := h.deps()
		test.unset(&deps)

		_, err := identify.NewService(deps).Search(context.Background(), "item", identify.Query{Text: "x"})
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("%s: Search error = %v, want ErrConflict", test.kind, err)
		}
	}
}

func TestSearchLoadErrors(t *testing.T) {
	h := newHarness()

	if _, err := h.service().Search(context.Background(), "missing", identify.Query{Text: "x"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown item error = %v, want ErrNotFound", err)
	}

	h.addItem("movie", library.ItemKindMovie, enrich.ItemSnapshot{})
	h.settings.err = errBoom

	if _, err := h.service().Search(context.Background(), "movie", identify.Query{Text: "x"}); !errors.Is(err, errBoom) {
		t.Fatalf("settings error = %v, want errBoom", err)
	}
}
