package enrich_test

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

var errBroken = errors.New("broken")

func TestEnrichItemSearchesAndMatchesAutomatically(t *testing.T) {
	h := newHarness()

	h.mustEnrich(t, enrich.OutcomeMatched)

	if want := []enrich.SearchQuery{{Title: "The Matrix", Year: 1999, Language: "en"}}; !slices.Equal(h.movies.queries, want) {
		t.Errorf("queries = %+v, want %+v", h.movies.queries, want)
	}

	saved := h.onlyEnrichment(t)
	if saved.Match.State != match.MatchStateMatched || saved.Match.Provider != match.RefTMDB ||
		saved.Match.Ref != matrixCandidate().Ref || saved.Match.Confidence < match.AutoMatchThreshold {
		t.Errorf("match = %+v, want matched at TMDB", saved.Match)
	}

	if saved.Record.Summary != "A hacker learns the truth." || len(saved.Record.Genres) != 2 {
		t.Errorf("record = %+v, want provider summary and genres", saved.Record)
	}

	if id, _ := saved.Record.ProviderID(metadata.ProviderTMDB); id != matrixTMDBID {
		t.Errorf("tmdb id = %q, want %s added for the next run", id, matrixTMDBID)
	}
}

func TestEnrichItemUsesKnownProviderIDWithoutSearching(t *testing.T) {
	tests := []struct {
		name    string
		ids     []metadata.ProviderID
		linked  match.ProviderRef
		wantRef match.ProviderRef
	}{
		{
			name:    "tmdb id wins over imdb",
			ids:     []metadata.ProviderID{{Provider: metadata.ProviderIMDb, Value: "tt0133093"}, {Provider: metadata.ProviderTMDB, Value: matrixTMDBID}},
			wantRef: match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie, ID: matrixTMDBID},
		},
		{
			name:    "imdb id alone",
			ids:     []metadata.ProviderID{{Provider: metadata.ProviderIMDb, Value: "tt0133093"}},
			wantRef: match.ProviderRef{Provider: match.RefIMDb, ID: "tt0133093"},
		},
		{
			name:    "corrected record id wins over the linked reference",
			ids:     []metadata.ProviderID{{Provider: metadata.ProviderTMDB, Value: matrixTMDBID}},
			linked:  match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie, ID: "604"},
			wantRef: match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie, ID: matrixTMDBID},
		},
		{
			name:    "linked reference as fallback",
			linked:  match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie, ID: "604"},
			wantRef: match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie, ID: "604"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness()
			snapshot := movieSnapshot()
			snapshot.Record.ProviderIDs = test.ids
			snapshot.LinkedRef = test.linked
			h.setSnapshot(snapshot)
			h.movies.details = map[string]enrich.Details{test.wantRef.ID: matrixDetails()}

			h.mustEnrich(t, enrich.OutcomeMatched)

			if h.movies.searchCalls != 0 {
				t.Errorf("search calls = %d, want 0", h.movies.searchCalls)
			}

			want := []enrich.DetailsRequest{{Ref: test.wantRef, Language: "en"}}
			if !slices.Equal(h.movies.detailsRequests, want) {
				t.Errorf("details requests = %+v, want %+v", h.movies.detailsRequests, want)
			}

			if confidence := h.onlyEnrichment(t).Match.Confidence; confidence != 1 {
				t.Errorf("confidence = %v, want 1", confidence)
			}
		})
	}
}

func TestEnrichItemAmbiguousMatchStaysUnmatched(t *testing.T) {
	remake := matrixCandidate()
	remake.Ref.ID = "999"

	h := newHarness()
	h.movies.candidates = []match.Candidate{matrixCandidate(), remake}

	h.mustEnrich(t, enrich.OutcomeUnmatched)

	if len(h.movies.detailsRequests) != 0 || len(h.store.enrichments) != 0 {
		t.Errorf("details requests = %d, enrichments = %d, want none", len(h.movies.detailsRequests), len(h.store.enrichments))
	}

	// The state is already unmatched, so nothing needs writing.
	if len(h.store.matchStates) != 0 {
		t.Errorf("match states = %+v, want none", h.store.matchStates)
	}
}

func TestEnrichItemNoConfidentMatchWritesUnmatched(t *testing.T) {
	h := newHarness()
	snapshot := movieSnapshot()
	snapshot.MatchState = ""
	h.setSnapshot(snapshot)
	h.movies.candidates = []match.Candidate{{Ref: matrixCandidate().Ref, Title: "Something Else", Year: 1970}}

	h.mustEnrich(t, enrich.OutcomeUnmatched)

	if want := []enrich.MatchResult{{State: match.MatchStateUnmatched}}; !slices.Equal(h.store.matchStates, want) {
		t.Errorf("match states = %+v, want %+v", h.store.matchStates, want)
	}
}

func TestEnrichItemKeepsAnEarlierMatchWithoutWinner(t *testing.T) {
	h := newHarness()
	snapshot := movieSnapshot()
	snapshot.MatchState = match.MatchStateMatched
	h.setSnapshot(snapshot)
	h.movies.candidates = nil

	h.mustEnrich(t, enrich.OutcomeMatched)

	if h.store.writes() != 0 {
		t.Errorf("writes = %d, want 0", h.store.writes())
	}
}

func TestEnrichItemSkipsWithoutAnyCall(t *testing.T) {
	tests := []struct {
		name   string
		modify func(h *harness, snapshot *enrich.ItemSnapshot)
	}{
		{"disabled item", func(_ *harness, snapshot *enrich.ItemSnapshot) { snapshot.MatchState = match.MatchStateDisabled }},
		{"library switched off", func(_ *harness, snapshot *enrich.ItemSnapshot) { snapshot.OnlineEnabled = false }},
		{"online switched off", func(h *harness, _ *enrich.ItemSnapshot) { h.settings.settings.Enabled = false }},
		{"tmdb switched off", func(h *harness, _ *enrich.ItemSnapshot) { h.settings.settings.TMDB = false }},
		{"musicbrainz switched off", func(h *harness, snapshot *enrich.ItemSnapshot) {
			h.settings.settings.MusicBrainz = false
			snapshot.Kind = library.ItemKindAlbum
		}},
		{"kind without provider", func(_ *harness, snapshot *enrich.ItemSnapshot) { snapshot.Kind = library.ItemKindTrack }},
		{"episode of a show without id", func(_ *harness, snapshot *enrich.ItemSnapshot) { snapshot.Kind = library.ItemKindEpisode }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness()
			snapshot := movieSnapshot()
			test.modify(h, &snapshot)
			h.setSnapshot(snapshot)

			h.mustEnrich(t, enrich.OutcomeSkipped)

			if h.providerCalls() != 0 || h.store.writes() != 0 {
				t.Errorf("provider calls = %d, writes = %d, want none", h.providerCalls(), h.store.writes())
			}
		})
	}
}

func TestEnrichItemWithoutLookupPortSkips(t *testing.T) {
	h := newHarness()
	enricher := enrich.NewEnricher(enrich.Deps{Store: h.store, Settings: h.settings})

	outcome, err := enricher.EnrichItem(t.Context(), itemID)
	if err != nil || outcome != enrich.OutcomeSkipped {
		t.Fatalf("EnrichItem = %s, %v, want skipped", outcome, err)
	}
}

func TestEnrichItemReturnsTypedLookupErrors(t *testing.T) {
	tests := []struct {
		name   string
		modify func(h *harness)
		want   error
	}{
		{"offline search", func(h *harness) { h.movies.searchErr = domain.ErrUnavailable }, domain.ErrUnavailable},
		{"rate limited search", func(h *harness) { h.movies.searchErr = domain.ErrRateLimited }, domain.ErrRateLimited},
		{"offline details", func(h *harness) { h.movies.detailsErr = domain.ErrUnavailable }, domain.ErrUnavailable},
		{"unknown id", func(h *harness) { h.movies.details = nil }, domain.ErrNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness()
			test.modify(h)

			_, err := h.enrich(t)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}

			if h.store.writes() != 0 {
				t.Errorf("writes = %d, want 0: the item keeps its local data", h.store.writes())
			}
		})
	}
}

func TestEnrichItemReportsStoreAndSettingsErrors(t *testing.T) {
	tests := []struct {
		name   string
		modify func(h *harness)
		want   error
	}{
		{"settings", func(h *harness) { h.settings.err = errBroken }, errBroken},
		{"snapshot", func(h *harness) { h.store.snapshotErr = errBroken }, errBroken},
		{"save enrichment", func(h *harness) { h.store.saveErr = errBroken }, errBroken},
		{"save match state", func(h *harness) {
			h.store.saveErr = errBroken
			h.movies.candidates = nil

			snapshot := movieSnapshot()
			snapshot.MatchState = ""
			h.setSnapshot(snapshot)
		}, errBroken},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness()
			test.modify(h)

			if _, err := h.enrich(t); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestEnrichItemSeparatesAGoneItemFromAProviderNotFound(t *testing.T) {
	tests := []struct {
		name     string
		modify   func(h *harness)
		wantGone bool
	}{
		{"unknown item", func(h *harness) { delete(h.store.snapshots, itemID) }, true},
		{"item deleted before saving", func(h *harness) { h.store.saveErr = fmt.Errorf("save: %w", domain.ErrNotFound) }, true},
		{"stale provider id", func(h *harness) { h.movies.details = nil }, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness()
			test.modify(h)

			_, err := h.enrich(t)
			if !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("error = %v, want not found", err)
			}

			if gone := errors.Is(err, enrich.ErrItemGone); gone != test.wantGone {
				t.Errorf("item gone = %v, want %v (error %v)", gone, test.wantGone, err)
			}
		})
	}
}

func TestEnrichItemSearchesWithLockedTitleAndYear(t *testing.T) {
	h := newHarness()
	snapshot := movieSnapshot()
	snapshot.Record = metadata.Record{Title: "The Matrix", Year: 1999}
	snapshot.Locks = metadata.NewLockSet([]metadata.Field{metadata.FieldTitle, metadata.FieldYear})
	snapshot.Local.NFO = metadata.Record{Title: "Matrix (NFO)", Year: 2000}
	h.setSnapshot(snapshot)

	h.mustEnrich(t, enrich.OutcomeMatched)

	if query := h.movies.queries[0]; query.Title != "The Matrix" || query.Year != 1999 {
		t.Errorf("query = %+v, want the locked title and year", query)
	}
}

func TestEnrichItemAddsTheMatchedIDBesideNFOProviderIDs(t *testing.T) {
	h := newHarness()
	snapshot := movieSnapshot()
	tvdb := metadata.ProviderID{Provider: metadata.ProviderTVDB, Value: "123"}
	snapshot.Local.NFO = metadata.Record{ProviderIDs: []metadata.ProviderID{tvdb}}
	h.setSnapshot(snapshot)

	h.mustEnrich(t, enrich.OutcomeMatched)

	saved := h.onlyEnrichment(t).Record
	if id, _ := saved.ProviderID(metadata.ProviderTMDB); id != matrixTMDBID || !slices.Contains(saved.ProviderIDs, tvdb) {
		t.Errorf("provider ids = %+v, want the NFO's TVDB ID and the matched TMDB ID", saved.ProviderIDs)
	}
}

func TestEnrichItemUsesItemLanguage(t *testing.T) {
	h := newHarness()
	snapshot := movieSnapshot()
	snapshot.Language = "de"
	h.setSnapshot(snapshot)

	h.mustEnrich(t, enrich.OutcomeMatched)

	if h.movies.queries[0].Language != "de" || h.movies.detailsRequests[0].Language != "de" {
		t.Errorf("languages = %q / %q, want de", h.movies.queries[0].Language, h.movies.detailsRequests[0].Language)
	}
}

func TestEnrichItemSearchesWithNFOTitle(t *testing.T) {
	h := newHarness()
	snapshot := movieSnapshot()
	snapshot.Local.Filename = metadata.Record{Title: "Matrix"}
	snapshot.Local.NFO = metadata.Record{Title: "The Matrix", Year: 1999}
	h.setSnapshot(snapshot)

	h.mustEnrich(t, enrich.OutcomeMatched)

	if query := h.movies.queries[0]; query.Title != "The Matrix" || query.Year != 1999 {
		t.Errorf("query = %+v, want the NFO title and year", query)
	}
}

// TestLookupRequestsCarryNoLocalData guards ADR 0031 §5: what goes to a provider has no
// path, file name, library or item reference. ArtworkRef is exempt: its Path is the
// provider's own address that the provider returned (a TMDB file path or a Cover Art Archive
// URL), not local data, and it goes only to the artwork downloader.
func TestLookupRequestsCarryNoLocalData(t *testing.T) {
	forbidden := []string{"path", "file", "library", "item", "root", "id"}
	requests := []any{enrich.SearchQuery{}, enrich.MusicQuery{}, enrich.DetailsRequest{}, enrich.SeasonRequest{}}

	for _, request := range requests {
		requestType := reflect.TypeOf(request)
		for index := range requestType.NumField() {
			name := strings.ToLower(requestType.Field(index).Name)
			for _, word := range forbidden {
				if strings.Contains(name, word) {
					t.Errorf("%s.%s looks like local data (%q)", requestType.Name(), requestType.Field(index).Name, word)
				}
			}
		}
	}
}
