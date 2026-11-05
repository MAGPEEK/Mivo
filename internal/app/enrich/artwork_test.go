package enrich_test

import (
	"context"
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

func tmdbPicture(kind artwork.Kind, path string) enrich.ArtworkRef {
	return enrich.ArtworkRef{Provider: enrich.ArtworkProviderTMDB, Kind: kind, Path: path}
}

func localPoster() artwork.Candidate {
	return artwork.Candidate{Kind: artwork.KindPoster, Source: artwork.SourceLocal, Location: artwork.LocalLocation("Matrix/poster.jpg")}
}

func earlierProviderBackdrop() artwork.Candidate {
	return artwork.Candidate{Kind: artwork.KindBackdrop, Source: artwork.SourceProvider, Location: artwork.DataLocation("artwork/old-backdrop.jpg")}
}

// artworkHarness offers two posters and a backdrop at the provider; the item has a local
// poster and an earlier provider backdrop.
func artworkHarness() *harness {
	h := newHarness()
	details := matrixDetails()
	details.Artwork = []enrich.ArtworkRef{
		tmdbPicture(artwork.KindPoster, "/poster-1.jpg"),
		tmdbPicture(artwork.KindPoster, "/poster-2.jpg"),
		tmdbPicture(artwork.KindBackdrop, "/backdrop-1.jpg"),
	}
	h.movies.details = map[string]enrich.Details{matrixTMDBID: details}

	snapshot := movieSnapshot()
	snapshot.Artwork = enrich.ArtworkState{
		Candidates: []artwork.Candidate{localPoster(), earlierProviderBackdrop()},
		Current:    map[artwork.Kind]artwork.Candidate{artwork.KindPoster: localPoster(), artwork.KindBackdrop: earlierProviderBackdrop()},
	}
	h.setSnapshot(snapshot)

	return h
}

func dataPaths(candidates []artwork.Candidate) map[artwork.Kind]string {
	paths := make(map[artwork.Kind]string, len(candidates))
	for _, candidate := range candidates {
		paths[candidate.Kind] = candidate.Location.DataPath
	}

	return paths
}

func TestEnrichItemRegistersProviderArtworkAndKeepsLocalFirst(t *testing.T) {
	h := artworkHarness()

	h.mustEnrich(t, enrich.OutcomeMatched)

	saved := h.onlyEnrichment(t)
	want := map[artwork.Kind]string{artwork.KindPoster: "artwork/poster-1.jpg", artwork.KindBackdrop: "artwork/backdrop-1.jpg"}

	if got := dataPaths(saved.ProviderArtwork); len(got) != len(want) || got[artwork.KindPoster] != want[artwork.KindPoster] || got[artwork.KindBackdrop] != want[artwork.KindBackdrop] {
		t.Errorf("provider artwork = %v, want %v", got, want)
	}

	if len(h.downloader.fetched) != 2 {
		t.Errorf("fetched = %+v, want only the best picture per kind", h.downloader.fetched)
	}

	if poster := saved.Selection[artwork.KindPoster]; poster.Source != artwork.SourceLocal {
		t.Errorf("poster = %+v, want the local one (ADR 0027 §8)", poster)
	}

	if backdrop := saved.Selection[artwork.KindBackdrop]; backdrop.Location.DataPath != "artwork/backdrop-1.jpg" {
		t.Errorf("backdrop = %+v, want the new provider one", backdrop)
	}

	for _, candidate := range saved.ProviderArtwork {
		if candidate.Source != artwork.SourceProvider {
			t.Errorf("provider artwork source = %s, want provider", candidate.Source)
		}
	}
}

func TestEnrichItemFailedDownloadDoesNotFailTheItem(t *testing.T) {
	h := artworkHarness()
	h.downloader.failures = map[string]bool{"/poster-1.jpg": true, "/backdrop-1.jpg": true}

	h.mustEnrich(t, enrich.OutcomeMatched)

	saved := h.onlyEnrichment(t)
	got := dataPaths(saved.ProviderArtwork)

	if got[artwork.KindPoster] != "artwork/poster-2.jpg" {
		t.Errorf("poster = %q, want the next poster after the failed one", got[artwork.KindPoster])
	}

	if got[artwork.KindBackdrop] != "artwork/old-backdrop.jpg" {
		t.Errorf("backdrop = %q, want the earlier provider picture kept", got[artwork.KindBackdrop])
	}

	if backdrop := saved.Selection[artwork.KindBackdrop]; backdrop.Location.DataPath != "artwork/old-backdrop.jpg" {
		t.Errorf("selected backdrop = %+v, want the earlier one", backdrop)
	}
}

func TestEnrichItemRespectsArtworkLocks(t *testing.T) {
	for _, kind := range artwork.Kinds() {
		t.Run(string(kind), func(t *testing.T) {
			field, err := metadata.ArtworkField(kind)
			if err != nil {
				t.Fatalf("artwork field: %v", err)
			}

			current := artwork.Candidate{Kind: kind, Source: artwork.SourceUpload, Location: artwork.DataLocation("uploads/mine.jpg")}
			h := newHarness()
			details := matrixDetails()
			details.Artwork = []enrich.ArtworkRef{tmdbPicture(kind, "/new.jpg")}
			h.movies.details = map[string]enrich.Details{matrixTMDBID: details}

			snapshot := movieSnapshot()
			snapshot.Locks = metadata.NewLockSet([]metadata.Field{field})
			snapshot.Artwork.Current = map[artwork.Kind]artwork.Candidate{kind: current}
			h.setSnapshot(snapshot)

			h.mustEnrich(t, enrich.OutcomeMatched)

			if len(h.downloader.fetched) != 0 {
				t.Errorf("fetched = %+v, want nothing for a locked kind", h.downloader.fetched)
			}

			if selected := h.onlyEnrichment(t).Selection[kind]; selected != current {
				t.Errorf("selection = %+v, want the locked %+v", selected, current)
			}
		})
	}
}

func TestEnrichItemCancelledDuringDownloadFails(t *testing.T) {
	h := artworkHarness()
	ctx, cancel := context.WithCancel(t.Context())
	h.downloader.cancel = cancel

	_, err := h.enricher().EnrichItem(ctx, itemID)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want canceled", err)
	}

	if h.store.writes() != 0 {
		t.Errorf("writes = %d, want 0", h.store.writes())
	}
}

func TestEnrichItemWithoutDownloaderKeepsEarlierProviderArtwork(t *testing.T) {
	h := artworkHarness()
	enricher := enrich.NewEnricher(enrich.Deps{Store: h.store, Settings: h.settings, Movies: h.movies})

	if _, err := enricher.EnrichItem(t.Context(), itemID); err != nil {
		t.Fatalf("EnrichItem: %v", err)
	}

	saved := h.onlyEnrichment(t)
	if got := dataPaths(saved.ProviderArtwork); len(got) != 1 || got[artwork.KindBackdrop] != "artwork/old-backdrop.jpg" {
		t.Errorf("provider artwork = %v, want only the earlier backdrop", got)
	}
}
