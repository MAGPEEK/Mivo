package scan

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
)

// refreshArtwork collects the item's local and embedded candidates, selects one per kind
// under the kind's lock and saves the result when the selection changed.
func (run *refreshRun) refreshArtwork(ctx context.Context, item *refreshItem, state MetadataState) error {
	candidates := run.artworkCandidates(item, state)
	selection := make(map[artwork.Kind]artwork.Candidate)

	for _, kind := range artwork.Kinds() {
		current, hasCurrent := state.CurrentArtwork[kind]

		var currentPointer *artwork.Candidate
		if hasCurrent {
			currentPointer = &current
		}

		if chosen, found := artwork.Select(ofKind(candidates, kind), state.LockedArtwork[kind], currentPointer); found {
			selection[kind] = chosen
		}
	}

	if sameSelection(state.CurrentArtwork, selection) {
		return nil
	}

	err := run.refresher.deps.Catalog.SaveArtwork(ctx, item.id, candidates, selection)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("save artwork: %w", err)
	}

	return nil
}

// artworkCandidates returns every local and embedded picture of the item, best first per kind.
func (run *refreshRun) artworkCandidates(item *refreshItem, state MetadataState) []artwork.Candidate {
	var candidates []artwork.Candidate

	switch item.level.Kind {
	case library.ItemKindMovie:
		candidates = run.movieArtwork(item)
	case library.ItemKindShow:
		candidates = run.showArtwork(item)
	case library.ItemKindSeason:
		candidates = run.seasonArtwork(item)
	case library.ItemKindEpisode:
		candidates = run.episodeArtwork(item)
	case library.ItemKindExtra, library.ItemKindArtist, library.ItemKindAlbum, library.ItemKindTrack:
		return nil
	default:
		return nil
	}

	candidates = append(candidates, embeddedArtwork(item.own, state)...)

	return dedupe(candidates)
}

func (run *refreshRun) movieArtwork(item *refreshItem) []artwork.Candidate {
	var candidates []artwork.Candidate

	for _, file := range item.own {
		directory := keyOf(file.State).directory()
		localContext := artwork.LocalContext{Kind: artwork.ContextLooseMovie, BaseName: baseName(file.State.RelativePath)}

		if run.index.isFolderMovie(directory) {
			localContext.Kind = artwork.ContextFolderMovie
		}

		candidates = append(candidates, run.localCandidates(directory, localContext, nil)...)
	}

	return candidates
}

func (run *refreshRun) episodeArtwork(item *refreshItem) []artwork.Candidate {
	var candidates []artwork.Candidate

	for _, file := range item.own {
		localContext := artwork.LocalContext{Kind: artwork.ContextEpisode, BaseName: baseName(file.State.RelativePath)}
		candidates = append(candidates, run.localCandidates(keyOf(file.State).directory(), localContext, nil)...)
	}

	return candidates
}

func (run *refreshRun) showArtwork(item *refreshItem) []artwork.Candidate {
	var candidates []artwork.Candidate

	for _, rootID := range item.rootIDs() {
		directory := item.showDirectory(rootID)
		if directory == "" {
			continue
		}

		candidates = append(candidates, run.localCandidates(fileKey{rootID: rootID, path: directory},
			artwork.LocalContext{Kind: artwork.ContextShowFolder},
			func(match artwork.LocalMatch) bool { return !match.ForSeason })...)
	}

	return candidates
}

// seasonArtwork takes the poster inside the season folder and "seasonNN-poster" files of the
// show folder, in every library folder that holds the season.
func (run *refreshRun) seasonArtwork(item *refreshItem) []artwork.Candidate {
	number, hasNumber := item.level.Number.Get()
	if !hasNumber {
		return nil
	}

	var candidates []artwork.Candidate

	for _, rootID := range item.rootIDs() {
		if directory := item.seasonDirectory(rootID); directory != "" {
			localContext := artwork.LocalContext{Kind: artwork.ContextSeasonFolder, Season: number}
			candidates = append(candidates, run.localCandidates(fileKey{rootID: rootID, path: directory}, localContext, nil)...)
		}

		if directory := item.showDirectory(rootID); directory != "" {
			onlyThisSeason := func(match artwork.LocalMatch) bool { return match.ForSeason && match.Season == number }
			candidates = append(candidates, run.localCandidates(fileKey{rootID: rootID, path: directory},
				artwork.LocalContext{Kind: artwork.ContextShowFolder}, onlyThisSeason)...)
		}
	}

	return candidates
}

// localCandidates classifies the images of directory; accept, when set, filters the matches.
// A library with local artwork switched off has none.
func (run *refreshRun) localCandidates(directory fileKey, localContext artwork.LocalContext,
	accept func(artwork.LocalMatch) bool,
) []artwork.Candidate {
	if !run.request.Library.Options.LocalArtwork {
		return nil
	}

	var candidates []artwork.Candidate

	for _, image := range run.sidecars.images(directory) {
		match, found := artwork.ClassifyLocal(localContext, path.Base(image.RelativePath))
		if !found || (accept != nil && !accept(match)) {
			continue
		}

		location := artwork.LocalLocation(image.RelativePath)
		location.RootID = image.RootID

		candidates = append(candidates, artwork.Candidate{
			Kind: match.Kind, Source: artwork.SourceLocal, Priority: match.Priority,
			Location: location, Size: image.Size, ModifiedAt: image.ModifiedAt,
		})
	}

	return candidates
}

// embeddedArtwork returns the attached pictures of the item's files as poster candidates. For
// a file this scan did not probe the pictures are unknown, so the stored embedded selection of
// that file is kept instead of being dropped.
func embeddedArtwork(files []RefreshFile, state MetadataState) []artwork.Candidate {
	var candidates []artwork.Candidate

	for _, file := range files {
		if !file.PicturesKnown {
			candidates = append(candidates, keptEmbedded(file, state)...)

			continue
		}

		for position, streamIndex := range file.EmbeddedPictures {
			location := artwork.EmbeddedLocation(file.State.RelativePath, streamIndex)
			location.RootID = file.State.RootID

			candidates = append(candidates, artwork.Candidate{
				Kind: artwork.KindPoster, Source: artwork.SourceEmbedded, Priority: position,
				Location: location,
				Size:     file.State.Size, ModifiedAt: file.State.ModifiedAt,
			})
		}
	}

	return candidates
}

func keptEmbedded(file RefreshFile, state MetadataState) []artwork.Candidate {
	var kept []artwork.Candidate

	for _, current := range state.CurrentArtwork {
		sameFolder := current.Location.RootID == "" || current.Location.RootID == file.State.RootID
		if current.Source == artwork.SourceEmbedded && sameFolder && current.Location.RelativePath == file.State.RelativePath {
			kept = append(kept, current)
		}
	}

	return kept
}

// dedupe keeps the best candidate per kind and location and orders the rest stably.
func dedupe(candidates []artwork.Candidate) []artwork.Candidate {
	slices.SortFunc(candidates, func(first, second artwork.Candidate) int {
		return cmp.Or(
			cmp.Compare(first.Kind, second.Kind),
			strings.Compare(first.Location.RootID, second.Location.RootID),
			strings.Compare(first.Location.RelativePath, second.Location.RelativePath),
			cmp.Compare(first.Location.StreamIndex, second.Location.StreamIndex),
			cmp.Compare(first.Priority, second.Priority),
		)
	})

	return slices.CompactFunc(candidates, func(first, second artwork.Candidate) bool {
		return first.Kind == second.Kind && first.Location == second.Location
	})
}

func ofKind(candidates []artwork.Candidate, kind artwork.Kind) []artwork.Candidate {
	return slices.DeleteFunc(slices.Clone(candidates), func(candidate artwork.Candidate) bool {
		return candidate.Kind != kind
	})
}

// sameSelection compares the selections by kind and cache tag, which covers source, location,
// size and modification time.
func sameSelection(stored, selected map[artwork.Kind]artwork.Candidate) bool {
	if len(stored) != len(selected) {
		return false
	}

	for kind, candidate := range selected {
		current, found := stored[kind]
		if !found || current.Tag() != candidate.Tag() {
			return false
		}
	}

	return true
}
