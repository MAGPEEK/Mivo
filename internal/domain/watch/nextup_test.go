package watch_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/domain/watch"
)

func episode(itemID string, season, number int) watch.EpisodeRef {
	return watch.EpisodeRef{ItemID: itemID, Season: season, Episode: number}
}

func watchedStates(itemIDs ...string) map[string]watch.State {
	states := make(map[string]watch.State, len(itemIDs))
	for _, itemID := range itemIDs {
		states[itemID] = watch.State{Watched: true, PlayCount: 1}
	}

	return states
}

func TestNextUp(t *testing.T) {
	t.Parallel()

	season1 := []watch.EpisodeRef{episode("s1e1", 1, 1), episode("s1e2", 1, 2), episode("s1e3", 1, 3)}
	twoSeasons := append(append([]watch.EpisodeRef{}, season1...), episode("s2e1", 2, 1), episode("s2e2", 2, 2))

	tests := []struct {
		name     string
		episodes []watch.EpisodeRef
		states   map[string]watch.State
		want     string
	}{
		{name: "no episodes", episodes: nil, states: nil, want: ""},
		{name: "nothing watched", episodes: season1, states: nil, want: ""},
		{name: "next episode in the season", episodes: season1, states: watchedStates("s1e1"), want: "s1e2"},
		{name: "crosses into the next season", episodes: twoSeasons, states: watchedStates("s1e3"), want: "s2e1"},
		{name: "all watched", episodes: season1, states: watchedStates("s1e1", "s1e2", "s1e3"), want: ""},
		{
			name: "after the furthest watched, not the first gap", episodes: twoSeasons,
			states: watchedStates("s1e1", "s1e3"), want: "s2e1",
		},
		{
			name: "input order does not matter",
			episodes: []watch.EpisodeRef{
				episode("s2e2", 2, 2), episode("s1e2", 1, 2), episode("s2e1", 2, 1), episode("s1e1", 1, 1),
			},
			states: watchedStates("s1e2"), want: "s2e1",
		},
		{
			name:     "gaps in numbering",
			episodes: []watch.EpisodeRef{episode("e1", 1, 1), episode("e4", 1, 4), episode("e9", 1, 9)},
			states:   watchedStates("e1"), want: "e4",
		},
		{
			name: "specials are skipped as next episode",
			episodes: []watch.EpisodeRef{
				episode("sp1", 0, 1), episode("s1e1", 1, 1), episode("s1e2", 1, 2),
			},
			states: watchedStates("s1e1"), want: "s1e2",
		},
		{
			name: "a watched special does not start Up next",
			episodes: []watch.EpisodeRef{
				episode("sp1", 0, 1), episode("s1e1", 1, 1),
			},
			states: watchedStates("sp1"), want: "",
		},
		{
			name:     "specials only",
			episodes: []watch.EpisodeRef{episode("sp1", 0, 1), episode("sp2", 0, 2)},
			states:   watchedStates("sp1"), want: "",
		},
		{
			name: "multi-episode file counts as one step",
			episodes: []watch.EpisodeRef{
				episode("e1", 1, 1), {ItemID: "e2-3", Season: 1, Episode: 2, EpisodeEnd: 3}, episode("e4", 1, 4),
			},
			states: watchedStates("e1"), want: "e2-3",
		},
		{
			name: "after a watched multi-episode file",
			episodes: []watch.EpisodeRef{
				episode("e1", 1, 1), {ItemID: "e2-3", Season: 1, Episode: 2, EpisodeEnd: 3}, episode("e4", 1, 4),
			},
			states: watchedStates("e1", "e2-3"), want: "e4",
		},
		{
			name: "absolute numbering in season 1",
			episodes: []watch.EpisodeRef{
				episode("a104", 1, 104), episode("a105", 1, 105), episode("a99", 1, 99),
			},
			states: watchedStates("a104"), want: "a105",
		},
		{
			name:     "equal numbers order by item id",
			episodes: []watch.EpisodeRef{episode("e1", 1, 1), episode("e2-b", 1, 2), episode("e2-a", 1, 2)},
			states:   watchedStates("e1"), want: "e2-a",
		},
		{
			name:     "an in-progress episode is still unwatched",
			episodes: season1,
			states: map[string]watch.State{
				"s1e1": {Watched: true, PlayCount: 1},
				"s1e2": {PositionSeconds: 300},
			},
			want: "s1e2",
		},
		{
			name:     "a rewatch in progress still counts as watched",
			episodes: season1,
			states: map[string]watch.State{
				"s1e1": {Watched: true, PlayCount: 2, PositionSeconds: 300},
			},
			want: "s1e2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, found := watch.NextUp(test.episodes, test.states)
			if found != (test.want != "") || got.ItemID != test.want {
				t.Errorf("NextUp() = %+v, %v; want item %q", got, found, test.want)
			}
		})
	}
}

func TestNextUpDoesNotReorderInput(t *testing.T) {
	t.Parallel()

	episodes := []watch.EpisodeRef{episode("b", 1, 2), episode("a", 1, 1)}
	watch.NextUp(episodes, watchedStates("a"))

	if episodes[0].ItemID != "b" || episodes[1].ItemID != "a" {
		t.Errorf("NextUp reordered its input: %+v", episodes)
	}
}
