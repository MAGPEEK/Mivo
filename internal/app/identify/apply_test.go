package identify_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// wronglyMatchedMovie is a movie auto-matched with the wrong TMDB entry whose title and
// poster the user locked.
func wronglyMatchedMovie() enrich.ItemSnapshot {
	return enrich.ItemSnapshot{
		MatchState: match.MatchStateMatched,
		Record: metadata.Record{
			Title:       "My Title",
			Summary:     "Summary of the wrong movie",
			Genres:      []string{"Horror"},
			ProviderIDs: []metadata.ProviderID{tmdbID("1"), {Provider: metadata.ProviderIMDb, Value: "tt0000001"}},
		},
		Locks:     metadata.NewLockSet([]metadata.Field{metadata.FieldTitle, metadata.FieldArtworkPoster}),
		Local:     enrich.LocalRecords{Filename: metadata.Record{Title: "Sintel", Year: 2010}},
		LinkedRef: match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie, ID: "1"},
	}
}

func TestApplyReidentifyKeepsUserLocks(t *testing.T) {
	h := newHarness()
	h.addItem("movie", library.ItemKindMovie, wronglyMatchedMovie())
	h.movies.details["45745"] = enrich.Details{Record: metadata.Record{
		Title: "Sintel", Summary: "A girl and a dragon", Year: 2010, ProviderIDs: []metadata.ProviderID{tmdbID("45745")},
	}}

	if err := h.service().Apply(context.Background(), "movie", match.RefTMDB, "45745"); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	applied := h.store.items["movie"]
	wantLocks := []metadata.Field{metadata.FieldTitle, metadata.FieldProviderIDs, metadata.FieldArtworkPoster}

	switch {
	case !slices.Equal(applied.Locks.Fields(), wantLocks):
		t.Fatalf("locks = %v, want %v", applied.Locks.Fields(), wantLocks)
	case applied.MatchState != match.MatchStateMatched:
		t.Fatalf("state = %s, want matched", applied.MatchState)
	case !slices.Equal(applied.Record.ProviderIDs, []metadata.ProviderID{tmdbID("45745")}):
		t.Fatalf("provider IDs = %v, want only the chosen one", applied.Record.ProviderIDs)
	case applied.Record.Title != "My Title" || applied.Record.Summary != "" || applied.Record.Genres != nil:
		t.Fatalf("record = %+v, want the locked title and no data of the wrong match", applied.Record)
	case applied.LinkedRef != (match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie, ID: "45745"}):
		t.Fatalf("linked ref = %+v", applied.LinkedRef)
	}

	if h.store.changes[0].Match.Confidence != 1 || len(h.movies.detailsRequests) != 0 {
		t.Fatalf("match = %+v, details requests %d; want confidence 1 and no provider call", h.store.changes[0].Match, len(h.movies.detailsRequests))
	}

	if want := []queued{{itemID: "movie", reason: enrich.ReasonIdentify}}; !slices.Equal(h.queue.calls, want) {
		t.Fatalf("queued = %+v, want %+v", h.queue.calls, want)
	}

	assertEnrichedWithLocks(t, h)
}

// assertEnrichedWithLocks runs the queued enrichment and checks that it fetched the chosen
// entry without a search and kept the locked title.
func assertEnrichedWithLocks(t *testing.T, h *harness) {
	t.Helper()

	outcome, err := h.enricher().EnrichItem(context.Background(), "movie")
	if err != nil || outcome != enrich.OutcomeMatched {
		t.Fatalf("EnrichItem = %s, %v; want matched", outcome, err)
	}

	enriched := h.store.items["movie"]
	if enriched.Record.Title != "My Title" || enriched.Record.Summary != "A girl and a dragon" {
		t.Fatalf("enriched record = %+v, want the locked title and the new summary", enriched.Record)
	}

	if len(h.movies.searches) != 0 || h.movies.detailsRequests[0].Ref.ID != "45745" {
		t.Fatalf("searches %d, details %+v; want the chosen entry without a search", len(h.movies.searches), h.movies.detailsRequests)
	}
}

func TestApplyShowQueuesSeasonsAndEpisodes(t *testing.T) {
	h := newHarness()
	h.addItem("show", library.ItemKindShow, enrich.ItemSnapshot{})
	h.store.children["show"] = []string{"season-1", "episode-1", "episode-gone"}
	h.queue.missing["episode-gone"] = true

	if err := h.service().Apply(context.Background(), "show", match.RefTMDB, "https://www.themoviedb.org/tv/1396"); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if want := []string{"show", "season-1", "episode-1"}; !slices.Equal(h.queue.itemIDs(), want) {
		t.Fatalf("queued = %v, want %v", h.queue.itemIDs(), want)
	}

	for _, call := range h.queue.calls {
		if call.reason != enrich.ReasonIdentify {
			t.Fatalf("reason = %s, want identify", call.reason)
		}
	}

	if ref := h.store.items["show"].LinkedRef; ref != (match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindShow, ID: "1396"}) {
		t.Fatalf("linked ref = %+v", ref)
	}
}

func TestApplyImdbAndMusicBrainz(t *testing.T) {
	h := newHarness()
	h.addItem("movie", library.ItemKindMovie, enrich.ItemSnapshot{MatchState: match.MatchStateDisabled})
	h.addItem("album", library.ItemKindAlbum, enrich.ItemSnapshot{Record: metadata.Record{ProviderIDs: []metadata.ProviderID{tmdbID("9")}}})

	if err := h.service().Apply(context.Background(), "movie", match.RefIMDb, "tt0133093"); err != nil {
		t.Fatalf("Apply imdb: %v", err)
	}

	movie := h.store.items["movie"]
	if movie.MatchState != match.MatchStateMatched || !slices.Equal(movie.Record.ProviderIDs, []metadata.ProviderID{{Provider: metadata.ProviderIMDb, Value: "tt0133093"}}) {
		t.Fatalf("movie = %+v, want a disabled item matched by its IMDb ID", movie)
	}

	if err := h.service().Apply(context.Background(), "album", match.RefMusicBrainz, testMBID); err != nil {
		t.Fatalf("Apply musicbrainz: %v", err)
	}

	album := h.store.items["album"]
	if album.LinkedRef != (match.ProviderRef{Provider: match.RefMusicBrainz, Kind: match.RefKindRelease, ID: testMBID}) ||
		!slices.Equal(album.Record.ProviderIDs, []metadata.ProviderID{tmdbID("9")}) || !album.Locks.Has(metadata.FieldProviderIDs) {
		t.Fatalf("album = %+v, want the release in the match reference and the record IDs unchanged", album)
	}
}

func TestApplyRejectsInvalidTargets(t *testing.T) {
	tests := []struct {
		name       string
		kind       library.ItemKind
		provider   match.RefProvider
		externalID string
	}{
		{"episode", library.ItemKindEpisode, match.RefTMDB, "1396"},
		{"season", library.ItemKindSeason, match.RefTMDB, "1396"},
		{"provider differs from the ID", library.ItemKindMovie, match.RefMusicBrainz, "603"},
		{"musicbrainz for a movie", library.ItemKindMovie, match.RefMusicBrainz, testMBID},
		{"show url for a movie", library.ItemKindMovie, match.RefTMDB, "https://www.themoviedb.org/tv/1396"},
		{"malformed ID", library.ItemKindMovie, match.RefTMDB, "abc"},
		{"kind without provider", library.ItemKindPhoto, match.RefTMDB, "603"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness()
			h.addItem("item", test.kind, enrich.ItemSnapshot{})

			err := h.service().Apply(context.Background(), "item", test.provider, test.externalID)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("Apply error = %v, want ErrInvalid", err)
			}

			if len(h.store.changes) != 0 || len(h.queue.calls) != 0 {
				t.Fatal("a rejected Apply wrote or queued")
			}
		})
	}
}

func TestApplyErrors(t *testing.T) {
	tests := []struct {
		name   string
		change func(h *harness)
		want   error
	}{
		{"online off", func(h *harness) { h.settings.settings.Enabled = false }, domain.ErrConflict},
		{"unknown item", func(h *harness) { delete(h.store.items, "show") }, domain.ErrNotFound},
		{"save fails", func(h *harness) { h.store.saveErr = errBoom }, errBoom},
		{"queue fails", func(h *harness) { h.queue.err = errBoom }, errBoom},
		{"children fail", func(h *harness) { h.store.childrenErr = errBoom }, errBoom},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness()
			h.addItem("show", library.ItemKindShow, enrich.ItemSnapshot{})
			test.change(h)

			if err := h.service().Apply(context.Background(), "show", match.RefTMDB, "1396"); !errors.Is(err, test.want) {
				t.Fatalf("Apply error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestApplyEpisodeIsInvalidEvenWithOnlineOff(t *testing.T) {
	h := newHarness()
	h.settings.settings.Enabled = false
	h.addItem("episode", library.ItemKindEpisode, enrich.ItemSnapshot{})

	if err := h.service().Apply(context.Background(), "episode", match.RefTMDB, "1396"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("Apply error = %v, want ErrInvalid before the online switches", err)
	}
}
