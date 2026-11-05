package identify_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/app/identify"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// matchedMovieWithNFO is a matched movie whose NFO names a TMDB ID and a title; the summary
// and genres came from TMDB, the tagline is locked.
func matchedMovieWithNFO() enrich.ItemSnapshot {
	nfo := metadata.Record{Title: "Sintel (NFO)", ProviderIDs: []metadata.ProviderID{tmdbID("45745")}}

	return enrich.ItemSnapshot{
		MatchState: match.MatchStateMatched,
		Record: metadata.Record{
			Title:       "Sintel",
			Summary:     "From TMDB",
			Tagline:     "My tagline",
			Genres:      []string{"Animation"},
			ProviderIDs: []metadata.ProviderID{tmdbID("45745"), {Provider: metadata.ProviderIMDb, Value: "tt1727587"}},
		},
		Locks:     metadata.NewLockSet([]metadata.Field{metadata.FieldTagline}),
		Local:     enrich.LocalRecords{NFO: nfo, Filename: metadata.Record{Title: "sintel", Year: 2010}},
		LinkedRef: match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie, ID: "45745"},
	}
}

func TestUnmatchDropsOnlineDataAndKeepsLocalData(t *testing.T) {
	h := newHarness()
	h.settings.settings.Enabled = false // removing a match works with online metadata off
	h.addItem("movie", library.ItemKindMovie, matchedMovieWithNFO())
	h.store.artwork["movie"] = []string{"artwork/a.jpg", "artwork/b.jpg"}

	if err := h.service().Unmatch(context.Background(), "movie"); err != nil {
		t.Fatalf("Unmatch: %v", err)
	}

	got := h.store.items["movie"]
	want := metadata.Record{
		Title: "Sintel (NFO)", Year: 2010, Tagline: "My tagline",
		ProviderIDs: []metadata.ProviderID{tmdbID("45745")},
	}

	switch {
	case got.MatchState != match.MatchStateDisabled || !got.LinkedRef.IsZero():
		t.Fatalf("state = %s, ref = %+v; want disabled without reference", got.MatchState, got.LinkedRef)
	case !recordsEqual(got.Record, want):
		t.Fatalf("record = %+v, want %+v (local data, locked tagline, NFO provider IDs)", got.Record, want)
	case !slices.Equal(got.Locks.Fields(), []metadata.Field{metadata.FieldTagline}):
		t.Fatalf("locks = %v, want unchanged", got.Locks.Fields())
	case !slices.Equal(h.files.removed, []string{"artwork/a.jpg", "artwork/b.jpg"}) || len(h.store.artwork) != 0:
		t.Fatalf("removed files %v, rows left %v", h.files.removed, h.store.artwork)
	}

	if h.movies.calls() != 0 {
		t.Fatal("Unmatch asked a provider")
	}
}

func recordsEqual(left, right metadata.Record) bool {
	return fmt.Sprintf("%+v", left) == fmt.Sprintf("%+v", right)
}

// TestIdentifyThenUnmatchReleasesTheIdentifyLock: Apply locks provider_ids for the user's
// choice; removing that match releases the lock, so the NFO ID is back after the next scan
// and the item stays disabled (ADR 0031 §9).
func TestIdentifyThenUnmatchReleasesTheIdentifyLock(t *testing.T) {
	h := newHarness()
	snapshot := matchedMovieWithNFO()
	h.addItem("movie", library.ItemKindMovie, snapshot)

	if err := h.service().Apply(context.Background(), "movie", match.RefTMDB, "603"); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if err := h.service().Unmatch(context.Background(), "movie"); err != nil {
		t.Fatalf("Unmatch: %v", err)
	}

	h.store.rescan("movie", snapshot.Local)

	got := h.store.items["movie"]

	switch {
	case got.MatchState != match.MatchStateDisabled:
		t.Fatalf("state = %s, want disabled", got.MatchState)
	case got.Locks.Has(metadata.FieldProviderIDs) || !got.Locks.Has(metadata.FieldTagline):
		t.Fatalf("locks = %v, want the tagline lock only", got.Locks.Fields())
	case !slices.Equal(got.Record.ProviderIDs, []metadata.ProviderID{tmdbID("45745")}):
		t.Fatalf("provider IDs = %v, want the NFO ID instead of the identified 603", got.Record.ProviderIDs)
	}
}

func TestUnmatchWithoutNFOClearsProviderIDs(t *testing.T) {
	h := newHarness()
	snapshot := matchedMovieWithNFO()
	snapshot.Local.NFO = metadata.Record{}
	snapshot.Locks = metadata.NewLockSet([]metadata.Field{metadata.FieldProviderIDs})
	h.addItem("movie", library.ItemKindMovie, snapshot)

	if err := h.service().Unmatch(context.Background(), "movie"); err != nil {
		t.Fatalf("Unmatch: %v", err)
	}

	if got := h.store.items["movie"]; got.Record.ProviderIDs != nil || got.Locks.Has(metadata.FieldProviderIDs) {
		t.Fatalf("provider IDs = %v, locks = %v; want none", got.Record.ProviderIDs, got.Locks.Fields())
	}
}

// TestUnmatchThenRescanStaysDisabled documents the NFO rule: the next scan merges the NFO
// again, so its provider IDs return to the record, but a scan never touches the match
// state. The item stays disabled and the enrichment worker skips it without asking a
// provider, although a trusted NFO ID would otherwise skip the search and link it.
func TestUnmatchThenRescanStaysDisabled(t *testing.T) {
	h := newHarness()
	snapshot := matchedMovieWithNFO()
	h.addItem("movie", library.ItemKindMovie, snapshot)

	if err := h.service().Unmatch(context.Background(), "movie"); err != nil {
		t.Fatalf("Unmatch: %v", err)
	}

	h.store.rescan("movie", snapshot.Local)

	rescanned := h.store.items["movie"]
	if rescanned.MatchState != match.MatchStateDisabled || !slices.Equal(rescanned.Record.ProviderIDs, []metadata.ProviderID{tmdbID("45745")}) {
		t.Fatalf("after rescan: state %s, provider IDs %v; want disabled with the NFO ID back", rescanned.MatchState, rescanned.Record.ProviderIDs)
	}

	outcome, err := h.enricher().EnrichItem(context.Background(), "movie")
	if err != nil || outcome != enrich.OutcomeSkipped || h.movies.calls() != 0 {
		t.Fatalf("EnrichItem = %s, %v, provider calls %d; want skipped without calls", outcome, err, h.movies.calls())
	}

	if state := h.store.items["movie"].MatchState; state != match.MatchStateDisabled {
		t.Fatalf("state after enrichment = %s, want disabled", state)
	}
}

func TestUnmatchFileErrorsAreLoggedNotFailed(t *testing.T) {
	h := newHarness()
	h.addItem("movie", library.ItemKindMovie, matchedMovieWithNFO())
	h.store.artwork["movie"] = []string{"artwork/gone.jpg", "artwork/locked.jpg", "artwork/ok.jpg"}
	h.files.errs["artwork/gone.jpg"] = fmt.Errorf("gone: %w", domain.ErrNotFound)
	h.files.errs["artwork/locked.jpg"] = errBoom

	var logs bytes.Buffer

	deps := h.deps()
	deps.Logger = slog.New(slog.NewTextHandler(&logs, nil))

	if err := identify.NewService(deps).Unmatch(context.Background(), "movie"); err != nil {
		t.Fatalf("Unmatch: %v", err)
	}

	if !slices.Equal(h.files.removed, []string{"artwork/ok.jpg"}) {
		t.Fatalf("removed = %v", h.files.removed)
	}

	if strings.Count(logs.String(), "could not delete provider artwork file") != 1 {
		t.Fatalf("logs = %q, want one warning for the failed file only", logs.String())
	}
}

func TestUnmatchErrors(t *testing.T) {
	tests := []struct {
		name   string
		change func(h *harness)
		want   error
	}{
		{"unknown item", func(h *harness) { delete(h.store.items, "movie") }, domain.ErrNotFound},
		{"save fails", func(h *harness) { h.store.saveErr = errBoom }, errBoom},
		{"artwork rows fail", func(h *harness) { h.store.artworkErr = errBoom }, errBoom},
		{"kind without provider", func(h *harness) {
			snapshot := h.store.items["movie"]
			snapshot.Kind = library.ItemKindTrack
			h.store.items["movie"] = snapshot
		}, domain.ErrInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness()
			h.addItem("movie", library.ItemKindMovie, matchedMovieWithNFO())
			test.change(h)

			if err := h.service().Unmatch(context.Background(), "movie"); !errors.Is(err, test.want) {
				t.Fatalf("Unmatch error = %v, want %v", err, test.want)
			}
		})
	}
}
