package rows

import (
	"cmp"
	"slices"
	"time"
)

// RecentEpisodeWindow bounds which episodes of a show count as recently added: those added
// at most this long before the show's newest episode. A season added in one scan becomes one
// card, while an episode added weeks later starts a new batch rather than reviving the badge
// count of the old one.
const RecentEpisodeWindow = 7 * 24 * time.Hour

// EpisodeGroup counts the recently added episodes of one season of a show (within
// RecentEpisodeWindow of the show's newest episode), as the catalog reports them.
type EpisodeGroup struct {
	ShowID       string
	SeasonID     string
	EpisodeCount int
	NewestAdded  time.Time
}

// EpisodeCard is one card of a "recentlyAdded" row of a shows library (HO-05): the season
// when all new episodes of the show belong to one season, else the show.
type EpisodeCard struct {
	// ItemID is the season (Season true) or the show.
	ItemID string
	ShowID string
	Season bool
	// NewEpisodeCount is the number of new episodes the card groups, at least 1.
	NewEpisodeCount int
	// NewestAdded orders the cards, newest first.
	NewestAdded time.Time
}

// GroupEpisodes turns the groups into at most limit cards, one per show, ordered by their
// newest episode (newest first, ties by show ID). Groups without episodes are ignored.
//
// GroupEpisodes does not apply RecentEpisodeWindow: the catalog reader counts only the
// episodes inside the window when it builds the groups (in SQL, where the per-show newest
// episode is known without reading every episode), so the groups arrive already windowed.
func GroupEpisodes(groups []EpisodeGroup, limit int) []EpisodeCard {
	if limit <= 0 {
		return nil
	}

	byShow := map[string]*EpisodeCard{}
	seasons := map[string]string{}

	var cards []*EpisodeCard

	for _, group := range groups {
		if group.EpisodeCount <= 0 {
			continue
		}

		card, found := byShow[group.ShowID]
		if !found {
			card = &EpisodeCard{ItemID: group.SeasonID, ShowID: group.ShowID, Season: true}
			byShow[group.ShowID], seasons[group.ShowID] = card, group.SeasonID
			cards = append(cards, card)
		}

		card.add(group, seasons[group.ShowID])
	}

	slices.SortFunc(cards, func(left, right *EpisodeCard) int {
		return cmp.Or(right.NewestAdded.Compare(left.NewestAdded), cmp.Compare(left.ShowID, right.ShowID))
	})

	result := make([]EpisodeCard, 0, min(limit, len(cards)))
	for _, card := range cards[:min(limit, len(cards))] {
		result = append(result, *card)
	}

	return result
}

// add counts the group into the card; a second season turns a season card into the show.
func (card *EpisodeCard) add(group EpisodeGroup, firstSeasonID string) {
	card.NewEpisodeCount += group.EpisodeCount

	if group.NewestAdded.After(card.NewestAdded) {
		card.NewestAdded = group.NewestAdded
	}

	if group.SeasonID != firstSeasonID {
		card.ItemID, card.Season = card.ShowID, false
	}
}
