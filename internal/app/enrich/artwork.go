package enrich

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// chooseArtwork downloads the best provider picture of every unlocked kind and selects per
// kind among the item's other candidates and the new provider ones (ADR 0027 §8: local,
// embedded, then provider; a locked kind keeps its selection). A picture that fails to
// download is skipped (the next one of its kind is tried); only cancellation fails the item.
func (enricher *Enricher) chooseArtwork(ctx context.Context, current item, refs []ArtworkRef) (
	map[artwork.Kind]artwork.Candidate, []artwork.Candidate, error,
) {
	fetched, err := enricher.fetchArtwork(ctx, current, refs)
	if err != nil {
		return nil, nil, err
	}

	providerArtwork := withEarlierProviderArtwork(current.snapshot, fetched)

	return selectArtwork(current.snapshot, providerArtwork), providerArtwork, nil
}

// withEarlierProviderArtwork keeps the item's earlier provider pictures of every kind that
// got no new one (locked, failed or no longer offered), so a failed download or a locked
// provider selection never loses a picture the store would otherwise replace.
func withEarlierProviderArtwork(snapshot ItemSnapshot, fetched []artwork.Candidate) []artwork.Candidate {
	renewed := make(map[artwork.Kind]bool, len(fetched))
	for _, candidate := range fetched {
		renewed[candidate.Kind] = true
	}

	providerArtwork := fetched

	for _, candidate := range snapshot.Artwork.Candidates {
		if candidate.Source == artwork.SourceProvider && !renewed[candidate.Kind] {
			providerArtwork = append(providerArtwork, candidate)
		}
	}

	return providerArtwork
}

func (enricher *Enricher) fetchArtwork(ctx context.Context, current item, refs []ArtworkRef) ([]artwork.Candidate, error) {
	if enricher.deps.Artwork == nil {
		return nil, nil
	}

	fetched := make(map[artwork.Kind]bool)

	var candidates []artwork.Candidate

	for priority, ref := range refs {
		if fetched[ref.Kind] || artworkLocked(current.snapshot.Locks, ref.Kind) {
			continue
		}

		stored, err := enricher.deps.Artwork.Fetch(ctx, ref)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("fetch artwork: %w", ctxErr)
		}

		if err != nil {
			enricher.deps.Logger.DebugContext(ctx, "provider artwork skipped",
				"item", current.id, "kind", ref.Kind, "error", err)

			continue
		}

		fetched[ref.Kind] = true
		candidates = append(candidates, artwork.Candidate{
			Kind:       ref.Kind,
			Source:     artwork.SourceProvider,
			Priority:   priority,
			Location:   artwork.DataLocation(stored.DataPath),
			Size:       stored.Size,
			ModifiedAt: stored.ModifiedAt,
		})
	}

	return candidates, nil
}

// selectArtwork selects per kind among the item's non-provider candidates and providerArtwork,
// which already holds the earlier provider pictures that stay.
func selectArtwork(snapshot ItemSnapshot, providerArtwork []artwork.Candidate) map[artwork.Kind]artwork.Candidate {
	byKind := make(map[artwork.Kind][]artwork.Candidate)

	for _, candidate := range snapshot.Artwork.Candidates {
		if candidate.Source != artwork.SourceProvider {
			byKind[candidate.Kind] = append(byKind[candidate.Kind], candidate)
		}
	}

	for _, candidate := range providerArtwork {
		byKind[candidate.Kind] = append(byKind[candidate.Kind], candidate)
	}

	for kind := range snapshot.Artwork.Current {
		if _, known := byKind[kind]; !known {
			byKind[kind] = nil
		}
	}

	selection := make(map[artwork.Kind]artwork.Candidate, len(byKind))

	for kind, candidates := range byKind {
		var current *artwork.Candidate
		if stored, ok := snapshot.Artwork.Current[kind]; ok {
			current = &stored
		}

		if chosen, ok := artwork.Select(candidates, artworkLocked(snapshot.Locks, kind), current); ok {
			selection[kind] = chosen
		}
	}

	return selection
}

// artworkLocked reports whether the user locked the kind; a kind without a lockable field
// (music's square) is never locked.
func artworkLocked(locks metadata.LockSet, kind artwork.Kind) bool {
	field, err := metadata.ArtworkField(kind)

	return err == nil && locks.Has(field)
}
