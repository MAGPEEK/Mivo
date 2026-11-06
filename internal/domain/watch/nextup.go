package watch

import (
	"cmp"
	"slices"
)

// specialsSeason is the season number of specials, which Up next never suggests or follows.
const specialsSeason = 0

// EpisodeRef is one episode item of a show as the catalog numbers it. Absolute-numbered
// episodes have season 1. A multi-episode file is one item with EpisodeEnd set to its last
// episode number (0 for a single episode).
type EpisodeRef struct {
	ItemID     string
	Season     int
	Episode    int
	EpisodeEnd int
}

// NextUp returns the first unwatched episode after the last watched one of a show in
// (season, episode) order, with specials (season 0) skipped. found is false when no regular
// episode is watched or all episodes after the last watched one are watched. states holds the
// profile's states by item ID; a missing entry is "not started".
//
// An in-progress episode counts as unwatched; excluding shows with an in-progress episode
// from Up next is the row's rule (ADR 0032 §8), not this function's.
func NextUp(episodes []EpisodeRef, states map[string]State) (next EpisodeRef, found bool) {
	ordered := regularEpisodesInOrder(episodes)

	lastWatched := -1
	for index, episode := range ordered {
		if states[episode.ItemID].Watched {
			lastWatched = index
		}
	}

	if lastWatched < 0 {
		return EpisodeRef{}, false
	}

	for _, episode := range ordered[lastWatched+1:] {
		if !states[episode.ItemID].Watched {
			return episode, true
		}
	}

	return EpisodeRef{}, false
}

// regularEpisodesInOrder returns a sorted copy of the episodes without specials; ties of
// equal numbers (versions, overlapping files) fall back to the item ID for a stable result.
func regularEpisodesInOrder(episodes []EpisodeRef) []EpisodeRef {
	ordered := make([]EpisodeRef, 0, len(episodes))
	for _, episode := range episodes {
		if episode.Season != specialsSeason {
			ordered = append(ordered, episode)
		}
	}

	slices.SortFunc(ordered, func(left, right EpisodeRef) int {
		return cmp.Or(
			cmp.Compare(left.Season, right.Season),
			cmp.Compare(left.Episode, right.Episode),
			cmp.Compare(left.EpisodeEnd, right.EpisodeEnd),
			cmp.Compare(left.ItemID, right.ItemID),
		)
	})

	return ordered
}
