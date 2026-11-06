package sqlite_test

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/domain/watch"
)

// watchSince is the start of the rows' window at watchNow.
func watchSince() time.Time { return watchNow().Add(-watch.ContinueWatchingWindow) }

// staleTime is outside the rows' window.
func staleTime() time.Time { return watchSince().Add(-time.Hour) }

// hoursAgo is a time inside the window, hours before watchNow().
func hoursAgo(hours int) time.Time {
	return watchNow().Add(-time.Duration(hours) * time.Hour)
}

func watchedAt(moment time.Time) watch.State {
	return watch.State{Watched: true, PlayCount: 1, LastPlayed: moment}
}

func inProgressAt(moment time.Time) watch.State {
	return watch.State{PositionSeconds: 600, LastPlayed: moment}
}

// rowShow is a show of the row fixture: its episodes and the profile's states by episode.
type rowShow struct {
	id       string
	episodes []watch.EpisodeRef
	states   map[int]watch.State // by index into episodes
}

// episodeRefs numbers the episodes of a show: each entry is {season, episode, episode end}.
func episodeRefs(showID string, numbers ...[3]int) []watch.EpisodeRef {
	refs := make([]watch.EpisodeRef, len(numbers))
	for index, number := range numbers {
		refs[index] = watch.EpisodeRef{
			ItemID: fmt.Sprintf("%s-s%02de%02d", showID, number[0], number[1]),
			Season: number[0], Episode: number[1], EpisodeEnd: number[2],
		}
	}

	return refs
}

// insertRowShow stores the show with one season item per season number and its episodes,
// then the states.
func (fixture watchFixture) insertRowShow(t *testing.T, show rowShow) {
	t.Helper()

	insertItem(t, fixture.inspection, newItemSpec(show.id, showsLibraryID, "show", show.id))

	seasons := map[int]bool{}
	for _, episode := range show.episodes {
		seasonID := fmt.Sprintf("%s-s%02d", show.id, episode.Season)
		if !seasons[episode.Season] {
			seasons[episode.Season] = true
			spec := newItemSpec(seasonID, showsLibraryID, "season", seasonID)
			spec.parentID, spec.number = show.id, episode.Season
			insertItem(t, fixture.inspection, spec)
		}

		spec := newItemSpec(episode.ItemID, showsLibraryID, "episode", episode.ItemID)
		spec.parentID, spec.number = seasonID, episode.Episode
		insertItem(t, fixture.inspection, spec)

		if episode.EpisodeEnd != 0 {
			execute(t, fixture.inspection, "UPDATE items SET number_end = ? WHERE id = ?", episode.EpisodeEnd, episode.ItemID)
		}
	}

	for index, state := range show.states {
		fixture.put(t, watchProfileID, show.episodes[index].ItemID, state)
	}
}

// upNextShows covers gaps, specials, multi-episode files, in-progress and hidden episodes,
// stale activity and finished shows.
func upNextShows() []rowShow {
	return []rowShow{
		{
			id: "gap", episodes: episodeRefs("gap", [3]int{1, 1, 0}, [3]int{1, 2, 0}, [3]int{1, 4, 0}, [3]int{2, 1, 0}, [3]int{0, 1, 0}),
			states: map[int]watch.State{0: watchedAt(hoursAgo(5)), 2: watchedAt(hoursAgo(4))},
		},
		{
			id: "specials-only", episodes: episodeRefs("specials-only", [3]int{1, 1, 0}, [3]int{0, 1, 0}),
			states: map[int]watch.State{1: watchedAt(hoursAgo(1))},
		},
		{
			id: "complete", episodes: episodeRefs("complete", [3]int{1, 1, 0}, [3]int{1, 2, 0}),
			states: map[int]watch.State{0: watchedAt(hoursAgo(2)), 1: watchedAt(hoursAgo(2))},
		},
		{
			id: "continuing", episodes: episodeRefs("continuing", [3]int{1, 1, 0}, [3]int{1, 2, 0}, [3]int{1, 3, 0}),
			states: map[int]watch.State{0: watchedAt(hoursAgo(3)), 1: inProgressAt(hoursAgo(3))},
		},
		{
			id: "hidden", episodes: episodeRefs("hidden", [3]int{1, 1, 0}, [3]int{1, 2, 0}),
			states: map[int]watch.State{0: watchedAt(hoursAgo(6)), 1: {PositionSeconds: 600, LastPlayed: hoursAgo(6), Hidden: true}},
		},
		{
			id: "stale", episodes: episodeRefs("stale", [3]int{1, 1, 0}, [3]int{1, 2, 0}),
			states: map[int]watch.State{0: watchedAt(staleTime())},
		},
		{
			id: "multi", episodes: episodeRefs("multi", [3]int{1, 1, 2}, [3]int{1, 3, 0}),
			states: map[int]watch.State{0: watchedAt(hoursAgo(7))},
		},
		{
			id: "rewatched", episodes: episodeRefs("rewatched", [3]int{1, 1, 0}, [3]int{1, 2, 0}, [3]int{1, 3, 0}, [3]int{1, 4, 0}),
			states: map[int]watch.State{0: watchedAt(hoursAgo(8)), 2: watchedAt(hoursAgo(9))},
		},
		{
			id: "special-between", episodes: episodeRefs("special-between", [3]int{1, 1, 0}, [3]int{0, 5, 0}, [3]int{2, 1, 0}),
			states: map[int]watch.State{0: watchedAt(hoursAgo(10))},
		},
	}
}

// expectedUpNext applies the row rule of ADR 0032 §8 with domain/watch: shows with a watched
// episode in the window and no episode in Continue watching, their watch.NextUp episode,
// newest activity first.
func expectedUpNext(shows []rowShow) []string {
	type candidate struct {
		itemID   string
		activity time.Time
	}

	var candidates []candidate

	for _, show := range shows {
		states, activity, continuing := map[string]watch.State{}, time.Time{}, false
		for index, state := range show.states {
			states[show.episodes[index].ItemID] = state
			continuing = continuing || watch.InContinueWatching(state, watchNow())

			if state.Watched && !state.LastPlayed.Before(watchSince()) && state.LastPlayed.After(activity) {
				activity = state.LastPlayed
			}
		}

		if next, found := watch.NextUp(show.episodes, states); found && !continuing && !activity.IsZero() {
			candidates = append(candidates, candidate{itemID: next.ItemID, activity: activity})
		}
	}

	slices.SortFunc(candidates, func(left, right candidate) int { return right.activity.Compare(left.activity) })

	itemIDs := make([]string, len(candidates))
	for index, candidate := range candidates {
		itemIDs[index] = candidate.itemID
	}

	return itemIDs
}

func TestWatchUpNextAgreesWithTheDomainRule(t *testing.T) {
	t.Parallel()

	fixture := newWatchFixture(t)
	shows := upNextShows()

	for _, show := range shows {
		fixture.insertRowShow(t, show)
	}

	want := expectedUpNext(shows)
	if pinned := []string{"gap-s02e01", "hidden-s01e02", "multi-s01e03", "rewatched-s01e04", "special-between-s02e01"}; !slices.Equal(want, pinned) {
		t.Fatalf("fixture yields %v under the domain rule, want %v", want, pinned)
	}

	query := sqlite.WatchRowQuery{ProfileID: watchProfileID, PlayedSince: watchSince(), Limit: watch.UpNextLimit}
	assertRow(t, upNextOf(fixture), query, want)

	query.Limit = 2
	assertRow(t, upNextOf(fixture), query, want[:2])

	query.Limit, query.LibraryID = watch.UpNextLimit, moviesLibraryID
	assertRow(t, upNextOf(fixture), query, []string{})

	query.LibraryID, query.ProfileID = "", otherWatchProfileID
	assertRow(t, upNextOf(fixture), query, []string{})
}

// Finished shows with newer activity than a whole batch do not hide an older show's next
// episode.
func TestWatchUpNextLooksPastFinishedShows(t *testing.T) {
	t.Parallel()

	fixture := newWatchFixture(t)

	const finishedShows = watch.UpNextLimit + 5

	for number := range finishedShows {
		id := fmt.Sprintf("finished-%02d", number)
		fixture.insertRowShow(t, rowShow{
			id: id, episodes: episodeRefs(id, [3]int{1, 1, 0}),
			states: map[int]watch.State{0: watchedAt(hoursAgo(1))},
		})
	}

	fixture.insertRowShow(t, rowShow{
		id: "older", episodes: episodeRefs("older", [3]int{1, 1, 0}, [3]int{1, 2, 0}),
		states: map[int]watch.State{0: watchedAt(hoursAgo(2))},
	})

	query := sqlite.WatchRowQuery{ProfileID: watchProfileID, PlayedSince: watchSince(), Limit: watch.UpNextLimit}
	assertRow(t, upNextOf(fixture), query, []string{"older-s01e02"})
}

// A season without a show (the schema allows a parentless season) has no Up next of its own
// and must not empty the row of the real shows, neither through a watched nor through an
// in-progress episode.
func TestWatchUpNextIgnoresEpisodesOfAParentlessSeason(t *testing.T) {
	t.Parallel()

	fixture := newWatchFixture(t)
	fixture.insertRowShow(t, upNextShows()[0])

	orphanSeason := newItemSpec("orphan-season", showsLibraryID, "season", "Orphan season")
	orphanSeason.number = 1
	insertItem(t, fixture.inspection, orphanSeason)

	for number, state := range []watch.State{watchedAt(hoursAgo(1)), inProgressAt(hoursAgo(1))} {
		id := fmt.Sprintf("orphan-e%02d", number+1)
		spec := newItemSpec(id, showsLibraryID, "episode", id)
		spec.parentID, spec.number = orphanSeason.id, number+1
		insertItem(t, fixture.inspection, spec)
		fixture.put(t, watchProfileID, id, state)
	}

	query := sqlite.WatchRowQuery{ProfileID: watchProfileID, PlayedSince: watchSince(), Limit: watch.UpNextLimit}
	assertRow(t, upNextOf(fixture), query, []string{"gap-s02e01"})
}

// Deleting an episode or the whole show leaves its rows but drops it from the rows.
func TestWatchUpNextIgnoresRowsOfDeletedItems(t *testing.T) {
	t.Parallel()

	fixture := newWatchFixture(t)
	fixture.insertRowShow(t, upNextShows()[0])

	query := sqlite.WatchRowQuery{ProfileID: watchProfileID, PlayedSince: watchSince(), Limit: watch.UpNextLimit}

	execute(t, fixture.inspection, "DELETE FROM items WHERE id = 'gap-s02e01'")
	assertRow(t, upNextOf(fixture), query, []string{})

	execute(t, fixture.inspection, "DELETE FROM items WHERE id = 'gap'")
	assertRow(t, upNextOf(fixture), query, []string{})
}

func TestWatchContinueWatchingListsInProgressItemsOnePerShow(t *testing.T) {
	t.Parallel()

	fixture := newWatchFixture(t)
	for number := 1; number <= 4; number++ {
		id := fmt.Sprintf("movie-%d", number)
		insertItem(t, fixture.inspection, newItemSpec(id, moviesLibraryID, "movie", id))
	}

	fixture.put(t, watchProfileID, "movie-1", inProgressAt(hoursAgo(2)))
	fixture.put(t, watchProfileID, "movie-2", watch.State{PositionSeconds: 600, LastPlayed: hoursAgo(1), Hidden: true})
	fixture.put(t, watchProfileID, "movie-3", inProgressAt(staleTime()))
	fixture.put(t, watchProfileID, "movie-4", watchedAt(hoursAgo(1)))
	fixture.put(t, watchProfileID, "deleted-movie", inProgressAt(hoursAgo(1)))
	fixture.insertRowShow(t, rowShow{
		id: "show", episodes: episodeRefs("show", [3]int{1, 1, 0}, [3]int{1, 2, 0}),
		states: map[int]watch.State{0: inProgressAt(hoursAgo(3)), 1: inProgressAt(hoursAgo(1))},
	})

	query := sqlite.WatchRowQuery{ProfileID: watchProfileID, PlayedSince: watchSince(), Limit: watch.ContinueWatchingLimit}
	assertRow(t, continueWatchingOf(fixture), query, []string{"show-s01e02", "movie-1"})

	query.Limit = 1
	assertRow(t, continueWatchingOf(fixture), query, []string{"show-s01e02"})

	query.Limit, query.LibraryID = watch.ContinueWatchingLimit, moviesLibraryID
	assertRow(t, continueWatchingOf(fixture), query, []string{"movie-1"})

	query.Limit = 0
	assertRow(t, continueWatchingOf(fixture), query, []string{})
	assertRow(t, upNextOf(fixture), query, []string{})
}

// rowLister is one of the store's row queries with its name for messages.
type rowLister struct {
	name string
	list func(context.Context, sqlite.WatchRowQuery) ([]string, error)
}

func upNextOf(fixture watchFixture) rowLister {
	return rowLister{name: "UpNext", list: fixture.store.UpNext}
}

func continueWatchingOf(fixture watchFixture) rowLister {
	return rowLister{name: "ContinueWatching", list: fixture.store.ContinueWatching}
}

func assertRow(t *testing.T, lister rowLister, query sqlite.WatchRowQuery, want []string) {
	t.Helper()

	got, err := lister.list(t.Context(), query)
	if err != nil {
		t.Fatalf("%s(%+v) error = %v", lister.name, query, err)
	}

	if !slices.Equal(got, want) {
		t.Errorf("%s(%+v) = %v, want %v", lister.name, query, got, want)
	}
}

// benchmarkShows is the size of the Up next benchmark library.
const benchmarkShows = 2_000

// BenchmarkWatchUpNext answers Up next on 2 000 shows of 2 seasons with 10 episodes each, the
// first 5 watched in every show (target: under 50 ms on the dev machine; not a gate).
func BenchmarkWatchUpNext(b *testing.B) {
	db, path := openMigratedTestDB(b)
	seedUpNextBenchmark(b, openInspectionDB(b, path))

	store := sqlite.NewWatchStore(db)
	query := sqlite.WatchRowQuery{ProfileID: watchProfileID, PlayedSince: watchSince(), Limit: watch.UpNextLimit}

	for b.Loop() {
		itemIDs, err := store.UpNext(b.Context(), query)
		if err != nil || len(itemIDs) != watch.UpNextLimit {
			b.Fatalf("UpNext = %d items, %v; want %d", len(itemIDs), err, watch.UpNextLimit)
		}
	}
}

// seedUpNextBenchmark inserts the benchmark library with set-based statements; one insert
// per row would dominate the setup.
func seedUpNextBenchmark(b *testing.B, inspection *sql.DB) {
	b.Helper()

	statements := []string{
		`INSERT INTO libraries (id, name, kind, created_at, updated_at) VALUES ('shows', 'Shows', 'shows', 0, 0)`,
		`INSERT INTO users (id, name, name_key, role, password_hash, created_at, updated_at)
			VALUES ('account-anna', 'Anna', 'anna', 'admin', 'hash', 0, 0)`,
		`INSERT INTO profiles (id, user_id, name, is_primary, avatar_color, created_at, updated_at)
			VALUES ('` + watchProfileID + `', 'account-anna', 'Anna', 1, 'coral', 0, 0)`,
		fmt.Sprintf(`WITH RECURSIVE shows(number) AS (SELECT 1 UNION ALL SELECT number + 1 FROM shows WHERE number < %d)
			INSERT INTO items (id, library_id, kind, match_key, title, sort_title, created_at, updated_at)
			SELECT 'show-' || number, 'shows', 'show', 'show-' || number, 'Show', 'show', 0, 0 FROM shows`, benchmarkShows),
		`WITH RECURSIVE seasons(number) AS (SELECT 1 UNION ALL SELECT number + 1 FROM seasons WHERE number < 2)
			INSERT INTO items (id, library_id, parent_id, kind, match_key, title, sort_title, number, created_at, updated_at)
			SELECT shows.id || '-s' || seasons.number, 'shows', shows.id, 'season', 's' || seasons.number,
			       'Season', 'season', seasons.number, 0, 0
			FROM items AS shows CROSS JOIN seasons WHERE shows.kind = 'show'`,
		`WITH RECURSIVE episodes(number) AS (SELECT 1 UNION ALL SELECT number + 1 FROM episodes WHERE number < 10)
			INSERT INTO items (id, library_id, parent_id, kind, match_key, title, sort_title, number, created_at, updated_at)
			SELECT seasons.id || 'e' || episodes.number, 'shows', seasons.id, 'episode', 'e' || episodes.number,
			       'Episode', 'episode', episodes.number, 0, 0
			FROM items AS seasons CROSS JOIN episodes WHERE seasons.kind = 'season'`,
		// Show n was last watched n minutes before watchNow().
		fmt.Sprintf(`INSERT INTO watch_state (profile_id, item_id, watched, play_count, last_played_at, updated_at)
			SELECT '%s', episodes.id, 1, 1, %d - CAST(substr(seasons.parent_id, 6) AS INTEGER) * %d, 0
			FROM items AS episodes JOIN items AS seasons ON seasons.id = episodes.parent_id
			WHERE episodes.kind = 'episode' AND seasons.number = 1 AND episodes.number <= 5`,
			watchProfileID, watchNow().UnixMilli(), time.Minute.Milliseconds()),
	}

	for _, statement := range statements {
		if _, err := inspection.ExecContext(b.Context(), statement); err != nil {
			b.Fatalf("seed: %s: %v", statement, err)
		}
	}
}
