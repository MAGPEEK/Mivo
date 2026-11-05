package enrich_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

const breakingBadTMDBID = "1396"

func breakingBadRef() match.ProviderRef {
	return match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindShow, ID: breakingBadTMDBID}
}

func showHarness(kind library.ItemKind) *harness {
	h := newHarness()
	h.setSnapshot(enrich.ItemSnapshot{
		Kind:              kind,
		OnlineEnabled:     true,
		MatchState:        match.MatchStateUnmatched,
		Local:             enrich.LocalRecords{Filename: metadata.Record{Title: "Breaking Bad", Year: 2008}},
		SeasonNumber:      2,
		EpisodeNumber:     3,
		ParentProviderIDs: []metadata.ProviderID{{Provider: metadata.ProviderTMDB, Value: breakingBadTMDBID}},
	})
	h.shows.candidates = []match.Candidate{{Ref: breakingBadRef(), Title: "Breaking Bad", Year: 2008}}
	h.shows.details = map[string]enrich.Details{breakingBadTMDBID: {Record: metadata.Record{Title: "Breaking Bad", Summary: "A teacher turns."}}}
	h.shows.seasons[2] = enrich.SeasonDetails{
		Details: enrich.Details{Record: metadata.Record{Title: "Season 2", Summary: "Season summary."}},
		Episodes: []enrich.EpisodeDetails{
			{Number: 1, Details: enrich.Details{Record: metadata.Record{Title: "Seven Thirty-Seven"}}},
			{Number: 3, Details: enrich.Details{Record: metadata.Record{Title: "Bit by a Dead Bee"}}},
		},
	}

	return h
}

func TestEnrichItemShowSearchesAndMatches(t *testing.T) {
	h := showHarness(library.ItemKindShow)

	h.mustEnrich(t, enrich.OutcomeMatched)

	if want := []enrich.SearchQuery{{Title: "Breaking Bad", Year: 2008, Language: "en"}}; !slices.Equal(h.shows.queries, want) {
		t.Errorf("queries = %+v, want %+v", h.shows.queries, want)
	}

	if want := []enrich.DetailsRequest{{Ref: breakingBadRef(), Language: "en"}}; !slices.Equal(h.shows.detailsRequests, want) {
		t.Errorf("details = %+v, want %+v", h.shows.detailsRequests, want)
	}

	if saved := h.onlyEnrichment(t); saved.Record.Summary != "A teacher turns." {
		t.Errorf("summary = %q, want the show's", saved.Record.Summary)
	}
}

func TestEnrichItemSeasonAndEpisodeUseTheShowAndPlacementNumbers(t *testing.T) {
	tests := []struct {
		kind      library.ItemKind
		wantTitle string
	}{
		{library.ItemKindSeason, "Season 2"},
		{library.ItemKindEpisode, "Bit by a Dead Bee"},
	}

	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			h := showHarness(test.kind)

			h.mustEnrich(t, enrich.OutcomeMatched)

			if h.shows.fakeTitles.calls() != 0 {
				t.Errorf("show search/details calls = %d, want 0", h.shows.fakeTitles.calls())
			}

			want := []enrich.SeasonRequest{{Show: breakingBadRef(), Season: 2, Language: "en"}}
			if !slices.Equal(h.shows.seasonRequests, want) {
				t.Errorf("season requests = %+v, want %+v", h.shows.seasonRequests, want)
			}

			assertChildEnrichment(t, h.onlyEnrichment(t), test.wantTitle)
		})
	}
}

func assertChildEnrichment(t *testing.T, saved enrich.Enrichment, wantTitle string) {
	t.Helper()

	if saved.Record.Title != wantTitle || saved.Match.State != match.MatchStateMatched {
		t.Errorf("saved = %q/%s, want %q matched", saved.Record.Title, saved.Match.State, wantTitle)
	}

	if saved.Match.Ref != breakingBadRef() {
		t.Errorf("match ref = %+v, want the show's", saved.Match.Ref)
	}

	if len(saved.Record.ProviderIDs) != 0 {
		t.Errorf("provider ids = %+v, want none: the show's ID is not the child's", saved.Record.ProviderIDs)
	}
}

func TestEnrichItemEpisodeTheProviderDoesNotList(t *testing.T) {
	tests := []struct {
		name        string
		state       match.MatchState
		wantOutcome enrich.Outcome
		wantWrites  []enrich.MatchResult
	}{
		{"new episode is written unmatched", "", enrich.OutcomeUnmatched, []enrich.MatchResult{{State: match.MatchStateUnmatched}}},
		{"matched episode keeps its match", match.MatchStateMatched, enrich.OutcomeMatched, nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := showHarness(library.ItemKindEpisode)
			snapshot := h.store.snapshots[itemID]
			snapshot.EpisodeNumber = 9
			snapshot.MatchState = test.state
			h.setSnapshot(snapshot)

			h.mustEnrich(t, test.wantOutcome)

			if !slices.Equal(h.store.matchStates, test.wantWrites) || len(h.store.enrichments) != 0 {
				t.Errorf("match states = %+v, enrichments = %d, want %+v and none", h.store.matchStates, len(h.store.enrichments), test.wantWrites)
			}
		})
	}
}

func TestEnrichItemSeasonLookupErrorIsTyped(t *testing.T) {
	h := showHarness(library.ItemKindSeason)
	h.shows.seasonErr = domain.ErrRateLimited

	if _, err := h.enrich(t); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("error = %v, want rate limited", err)
	}
}

func TestEnrichItemMusic(t *testing.T) {
	releaseRef := match.ProviderRef{Provider: match.RefMusicBrainz, Kind: match.RefKindRelease, ID: "b84ee12a-09ef-421b-82de-0441a926375b"}
	artistRef := match.ProviderRef{Provider: match.RefMusicBrainz, Kind: match.RefKindArtist, ID: "a74b1b7f-71a5-4011-9441-d0b5e4122711"}

	tests := []struct {
		name      string
		kind      library.ItemKind
		linked    match.ProviderRef
		wantQuery []enrich.MusicQuery
		wantRef   match.ProviderRef
	}{
		{
			name:      "album search with artist",
			kind:      library.ItemKindAlbum,
			wantQuery: []enrich.MusicQuery{{Kind: match.RefKindRelease, Title: "OK Computer", Artist: "Radiohead", Year: 1997}},
			wantRef:   releaseRef,
		},
		{
			name:      "artist search",
			kind:      library.ItemKindArtist,
			wantQuery: []enrich.MusicQuery{{Kind: match.RefKindArtist, Title: "OK Computer", Year: 1997}},
			wantRef:   artistRef,
		},
		{name: "linked album", kind: library.ItemKindAlbum, linked: releaseRef, wantRef: releaseRef},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness()
			h.setSnapshot(enrich.ItemSnapshot{
				Kind: test.kind, OnlineEnabled: true, MatchState: match.MatchStateUnmatched, ArtistName: "Radiohead",
				Local:     enrich.LocalRecords{Filename: metadata.Record{Title: "OK Computer", Year: 1997}},
				LinkedRef: test.linked,
				// A TMDB ID is no MusicBrainz reference and must not be used for music.
				Record: metadata.Record{ProviderIDs: []metadata.ProviderID{{Provider: metadata.ProviderTMDB, Value: "1"}}},
			})
			h.music.candidates = []match.Candidate{{Ref: test.wantRef, Title: "OK Computer", Year: 1997}}
			h.music.details = map[string]enrich.Details{test.wantRef.ID: {Record: metadata.Record{Title: "OK Computer", Genres: []string{"Rock"}}}}

			h.mustEnrich(t, enrich.OutcomeMatched)

			if !slices.Equal(h.music.queries, test.wantQuery) {
				t.Errorf("queries = %+v, want %+v", h.music.queries, test.wantQuery)
			}

			if h.music.detailsRequests[0].Ref != test.wantRef {
				t.Errorf("details ref = %+v, want %+v", h.music.detailsRequests[0].Ref, test.wantRef)
			}

			saved := h.onlyEnrichment(t)
			if saved.Match.Provider != match.RefMusicBrainz || len(saved.Record.Genres) != 1 {
				t.Errorf("saved = %+v, want MusicBrainz data", saved)
			}

			// The MBID cannot live in the record; the match carries it for LinkedRef.
			if saved.Match.Ref != test.wantRef {
				t.Errorf("match ref = %+v, want %+v", saved.Match.Ref, test.wantRef)
			}
		})
	}
}
