package enrich_test

import (
	"context"
	"testing"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

const (
	itemID       = "item-1"
	matrixTMDBID = "603"
)

type harness struct {
	store      *fakeStore
	settings   fakeSettings
	movies     *fakeMovies
	shows      *fakeShows
	music      *fakeMusic
	downloader *fakeDownloader
}

// newHarness returns ports with every switch on and an unmatched movie "The Matrix (1999)"
// that TMDB finds exactly once.
func newHarness() *harness {
	return &harness{
		store:    &fakeStore{snapshots: map[string]enrich.ItemSnapshot{itemID: movieSnapshot()}},
		settings: fakeSettings{settings: enrich.OnlineSettings{Enabled: true, TMDB: true, MusicBrainz: true, Language: "en"}},
		movies: &fakeMovies{fakeTitles: fakeTitles{
			candidates: []match.Candidate{matrixCandidate()},
			details:    map[string]enrich.Details{matrixTMDBID: matrixDetails()},
		}},
		shows:      &fakeShows{seasons: map[int]enrich.SeasonDetails{}},
		music:      &fakeMusic{},
		downloader: &fakeDownloader{},
	}
}

func (h *harness) enricher() *enrich.Enricher {
	return enrich.NewEnricher(enrich.Deps{
		Store:    h.store,
		Settings: h.settings,
		Movies:   h.movies,
		Shows:    h.shows,
		Music:    h.music,
		Artwork:  h.downloader,
	})
}

func (h *harness) setSnapshot(snapshot enrich.ItemSnapshot) {
	h.store.snapshots[itemID] = snapshot
}

func (h *harness) providerCalls() int {
	return h.movies.calls() + h.shows.calls() + h.music.calls() + len(h.downloader.fetched)
}

func (h *harness) enrich(t *testing.T) (enrich.Outcome, error) {
	t.Helper()

	return h.enricher().EnrichItem(context.Background(), itemID)
}

// mustEnrich runs EnrichItem and fails the test on an error.
func (h *harness) mustEnrich(t *testing.T, want enrich.Outcome) {
	t.Helper()

	outcome, err := h.enrich(t)
	if err != nil {
		t.Fatalf("EnrichItem: %v", err)
	}

	if outcome != want {
		t.Fatalf("outcome = %s, want %s", outcome, want)
	}
}

// onlyEnrichment returns the single saved enrichment.
func (h *harness) onlyEnrichment(t *testing.T) enrich.Enrichment {
	t.Helper()

	if len(h.store.enrichments) != 1 {
		t.Fatalf("saved enrichments = %d, want 1", len(h.store.enrichments))
	}

	return h.store.enrichments[0]
}

func movieSnapshot() enrich.ItemSnapshot {
	filename := metadata.Record{Title: "The Matrix", Year: 1999}

	return enrich.ItemSnapshot{
		Kind:          library.ItemKindMovie,
		OnlineEnabled: true,
		MatchState:    match.MatchStateUnmatched,
		Record:        filename,
		Local:         enrich.LocalRecords{Filename: filename},
	}
}

func matrixCandidate() match.Candidate {
	return match.Candidate{
		Ref:   match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie, ID: matrixTMDBID},
		Title: "The Matrix",
		Year:  1999,
	}
}

func matrixDetails() enrich.Details {
	return enrich.Details{Record: metadata.Record{
		Title:   "The Matrix",
		Year:    1999,
		Summary: "A hacker learns the truth.",
		Genres:  []string{"Action", "Science Fiction"},
	}}
}
