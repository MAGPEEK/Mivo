package sqlite_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	appwatch "github.com/magpeek/mivo/internal/app/watch"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/watch"
)

// Profiles of the watch fixture: two profiles of one account.
const (
	watchProfileID      = "profile-anna"
	otherWatchProfileID = "profile-ben"
)

// watchNow is the fixture's current time (2026-11-06 12:00 UTC).
func watchNow() time.Time { return time.Date(2026, time.November, 6, 12, 0, 0, 0, time.UTC) }

// watchFixture is a catalog fixture with an account of two profiles and a watch store.
type watchFixture struct {
	catalogFixture
	store *sqlite.WatchStore
}

func newWatchFixture(t *testing.T) watchFixture {
	t.Helper()

	return watchFixtureOn(t, newCatalogFixture(t))
}

// watchFixtureOn adds the account and the watch store to an existing catalog fixture.
func watchFixtureOn(t *testing.T, catalog catalogFixture) watchFixture {
	t.Helper()

	fixture := watchFixture{catalogFixture: catalog}
	fixture.store = sqlite.NewWatchStore(fixture.db)

	execute(t, fixture.inspection, `INSERT INTO users (id, name, name_key, role, password_hash, created_at, updated_at)
		VALUES ('account-anna', 'Anna', 'anna', 'admin', 'hash', 0, 0)`)
	execute(t, fixture.inspection, `INSERT INTO profiles (id, user_id, name, is_primary, avatar_color, created_at, updated_at)
		VALUES (?, 'account-anna', 'Anna', 1, 'coral', 0, 0), (?, 'account-anna', 'Ben', 0, 'coral', 0, 0)`,
		watchProfileID, otherWatchProfileID)

	return fixture
}

// put stores state for the item of profileID, replacing what is stored.
func (fixture watchFixture) put(t *testing.T, profileID, itemID string, state watch.State) {
	t.Helper()

	err := fixture.store.UpdateStates(t.Context(), appwatch.StateUpdate{
		ProfileID: profileID, Keys: []appwatch.StateKey{{ItemID: itemID}},
		Change: func(watch.State) watch.State { return state }, Now: watchNow(),
	})
	if err != nil {
		t.Fatalf("UpdateStates(%s) error = %v", itemID, err)
	}
}

func (fixture watchFixture) states(t *testing.T, profileID string, itemIDs ...string) map[string]watch.State {
	t.Helper()

	states, err := fixture.store.States(t.Context(), profileID, itemIDs)
	if err != nil {
		t.Fatalf("States error = %v", err)
	}

	return states
}

func (fixture watchFixture) rowCount(t *testing.T) int {
	t.Helper()

	var count int
	if err := fixture.inspection.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM watch_state").Scan(&count); err != nil {
		t.Fatalf("count watch_state rows: %v", err)
	}

	return count
}

func markWatchedAt(now time.Time) func(watch.State) watch.State {
	return func(state watch.State) watch.State { return watch.MarkWatched(state, now) }
}

func TestWatchStatesRoundTripPerProfile(t *testing.T) {
	t.Parallel()

	fixture := newWatchFixture(t)
	stored := watch.State{PositionSeconds: 612.25, PlayCount: 2, Watched: true, LastPlayed: watchNow(), Hidden: true}
	fixture.put(t, watchProfileID, "movie-1", stored)

	got := fixture.states(t, watchProfileID, "movie-1", "movie-2")
	if len(got) != 1 || got["movie-1"] != stored {
		t.Errorf("States = %+v, want only movie-1 = %+v", got, stored)
	}

	if other := fixture.states(t, otherWatchProfileID, "movie-1"); len(other) != 0 {
		t.Errorf("States of the other profile = %+v, want none", other)
	}
}

// The change sees the stored state, the attach key is stored, an unchanged result is not
// rewritten (updated_at stays) and a zero result for an item without a row adds no row.
func TestWatchUpdateStatesReadsModifiesAndSkipsUnchanged(t *testing.T) {
	t.Parallel()

	fixture := newWatchFixture(t)
	ctx := t.Context()
	keys := []appwatch.StateKey{{ItemID: "episode-1", AttachKey: "tmdb:episode:1"}, {ItemID: "episode-2"}}

	mark := appwatch.StateUpdate{ProfileID: watchProfileID, Keys: keys, Change: markWatchedAt(watchNow()), Now: watchNow()}
	if err := fixture.store.UpdateStates(ctx, mark); err != nil {
		t.Fatalf("UpdateStates error = %v", err)
	}

	later := watchNow().Add(time.Hour)
	mark.Change, mark.Now = markWatchedAt(later), later

	if err := fixture.store.UpdateStates(ctx, mark); err != nil {
		t.Fatalf("second UpdateStates error = %v", err)
	}

	var attachKey string

	var updatedAt int64

	err := fixture.inspection.QueryRowContext(ctx, `SELECT attach_key, updated_at FROM watch_state
		WHERE profile_id = ? AND item_id = 'episode-1'`, watchProfileID).Scan(&attachKey, &updatedAt)
	if err != nil || attachKey != "tmdb:episode:1" || updatedAt != watchNow().UnixMilli() {
		t.Errorf("row = (%q, %d, %v), want the attach key and the first update time (unchanged result skipped)",
			attachKey, updatedAt, err)
	}

	if got := fixture.states(t, watchProfileID, "episode-2")["episode-2"]; got.PlayCount != 1 || !got.LastPlayed.Equal(watchNow()) {
		t.Errorf("episode-2 = %+v, want one play at the first mark", got)
	}

	unmark := appwatch.StateUpdate{
		ProfileID: watchProfileID, Keys: []appwatch.StateKey{{ItemID: "episode-3"}},
		Change: watch.MarkUnwatched, Now: watchNow(),
	}
	if err := fixture.store.UpdateStates(ctx, unmark); err != nil {
		t.Fatalf("UpdateStates(unwatched) error = %v", err)
	}

	if got := fixture.rowCount(t); got != len(keys) {
		t.Errorf("rows = %d, want %d (a zero result adds no row)", got, len(keys))
	}
}

// A key named twice is changed once.
func TestWatchUpdateStatesChangesADuplicateKeyOnce(t *testing.T) {
	t.Parallel()

	fixture := newWatchFixture(t)
	increment := func(state watch.State) watch.State { state.PlayCount++; return state }
	keys := []appwatch.StateKey{{ItemID: "movie-1", AttachKey: "first"}, {ItemID: "movie-1", AttachKey: "second"}}

	err := fixture.store.UpdateStates(t.Context(), appwatch.StateUpdate{
		ProfileID: watchProfileID, Keys: keys, Change: increment, Now: watchNow(),
	})
	if err != nil {
		t.Fatalf("UpdateStates error = %v", err)
	}

	if got := fixture.states(t, watchProfileID, "movie-1")["movie-1"].PlayCount; got != 1 {
		t.Errorf("play count = %d, want 1", got)
	}
}

// Marking a show with more episodes than one statement binds writes all of them.
func TestWatchUpdateStatesWritesMoreKeysThanOneBatch(t *testing.T) {
	t.Parallel()

	fixture := newWatchFixture(t)

	const episodes = 1_201

	keys := make([]appwatch.StateKey, episodes)
	for index := range keys {
		keys[index] = appwatch.StateKey{ItemID: fmt.Sprintf("episode-%04d", index)}
	}

	err := fixture.store.UpdateStates(t.Context(), appwatch.StateUpdate{
		ProfileID: watchProfileID, Keys: keys, Change: markWatchedAt(watchNow()), Now: watchNow(),
	})
	if err != nil {
		t.Fatalf("UpdateStates error = %v", err)
	}

	if got := fixture.rowCount(t); got != episodes {
		t.Errorf("rows = %d, want %d", got, episodes)
	}

	ids := make([]string, episodes)
	for index, key := range keys {
		ids[index] = key.ItemID
	}

	if got := len(fixture.states(t, watchProfileID, ids...)); got != episodes {
		t.Errorf("States returned %d entries, want %d", got, episodes)
	}
}

// Concurrent read-modify-writes of the same item never lose an increment.
func TestWatchUpdateStatesConcurrentIncrementsAreNotLost(t *testing.T) {
	t.Parallel()

	fixture := newWatchFixture(t)

	const (
		writers           = 4
		updatesPerWriter  = 10
		expectedPlayCount = writers * updatesPerWriter
	)

	increment := func(state watch.State) watch.State { state.PlayCount++; return state }

	var group sync.WaitGroup
	for range writers {
		group.Go(func() {
			for range updatesPerWriter {
				err := fixture.store.UpdateStates(t.Context(), appwatch.StateUpdate{
					ProfileID: watchProfileID, Keys: []appwatch.StateKey{{ItemID: "movie-1"}}, Change: increment, Now: watchNow(),
				})
				if err != nil {
					t.Errorf("UpdateStates error = %v", err)
				}
			}
		})
	}

	group.Wait()

	if got := fixture.states(t, watchProfileID, "movie-1")["movie-1"].PlayCount; got != expectedPlayCount {
		t.Errorf("play count = %d, want %d", got, expectedPlayCount)
	}
}

func TestWatchUpdateStatesOfAnUnknownProfileIsNotFound(t *testing.T) {
	t.Parallel()

	fixture := newWatchFixture(t)

	err := fixture.store.UpdateStates(t.Context(), appwatch.StateUpdate{
		ProfileID: "profile-unknown", Keys: []appwatch.StateKey{{ItemID: "movie-1"}},
		Change: markWatchedAt(watchNow()), Now: watchNow(),
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("UpdateStates error = %v, want domain.ErrNotFound", err)
	}

	if err := fixture.store.UpdateStates(t.Context(), appwatch.StateUpdate{ProfileID: watchProfileID}); err != nil {
		t.Errorf("UpdateStates without keys error = %v, want nil", err)
	}
}

// Library removal, rescan pruning and item deletion keep the rows; deleting the profile
// removes them.
func TestWatchRowsSurviveCatalogChangesAndFollowTheProfile(t *testing.T) {
	t.Parallel()

	fixture := newWatchFixture(t)
	insertItem(t, fixture.inspection, newItemSpec("movie-1", moviesLibraryID, "movie", "Movie"))
	insertItem(t, fixture.inspection, newItemSpec("movie-2", moviesLibraryID, "movie", "Movie 2"))
	fixture.put(t, watchProfileID, "movie-1", watch.State{Watched: true, PlayCount: 1, LastPlayed: watchNow()})
	fixture.put(t, watchProfileID, "movie-2", watch.State{PositionSeconds: 100, LastPlayed: watchNow()})
	fixture.put(t, otherWatchProfileID, "movie-1", watch.State{PositionSeconds: 50, LastPlayed: watchNow()})

	execute(t, fixture.inspection, "DELETE FROM items WHERE id = 'movie-2'")
	execute(t, fixture.inspection, "DELETE FROM libraries WHERE id = ?", moviesLibraryID)

	if got := fixture.rowCount(t); got != 3 {
		t.Errorf("rows after the catalog changes = %d, want 3", got)
	}

	execute(t, fixture.inspection, "DELETE FROM profiles WHERE id = ?", otherWatchProfileID)

	if got := fixture.rowCount(t); got != 2 {
		t.Errorf("rows after deleting a profile = %d, want 2", got)
	}

	if got := len(fixture.states(t, watchProfileID, "movie-1", "movie-2")); got != 2 {
		t.Errorf("States of the remaining profile = %d entries, want 2", got)
	}
}

func TestWatchEpisodeCountsCountTheEpisodesBelowShowsAndSeasons(t *testing.T) {
	t.Parallel()

	show := newShowFixture(t, 3)
	fixture := watchFixtureOn(t, show.catalogFixture)

	fixture.put(t, watchProfileID, testID('e', 1), watch.State{Watched: true, PlayCount: 1, LastPlayed: watchNow()})
	fixture.put(t, watchProfileID, testID('e', 2), watch.State{PositionSeconds: 300, LastPlayed: watchNow()})
	fixture.put(t, otherWatchProfileID, testID('e', 3), watch.State{Watched: true, PlayCount: 1, LastPlayed: watchNow()})

	counts, err := fixture.store.EpisodeCounts(t.Context(), watchProfileID,
		[]string{show.showID, show.season1ID, show.season2ID, testID('e', 1), unknownID})
	if err != nil {
		t.Fatalf("EpisodeCounts error = %v", err)
	}

	want := map[string]appwatch.EpisodeCount{
		show.showID:    {Total: 4, Unwatched: 3},
		show.season1ID: {Total: 3, Unwatched: 2},
	}
	if len(counts) != len(want) || counts[show.showID] != want[show.showID] || counts[show.season1ID] != want[show.season1ID] {
		t.Errorf("EpisodeCounts = %+v, want %+v", counts, want)
	}
}

func TestWatchStoreFailsWithACancelledContext(t *testing.T) {
	t.Parallel()

	fixture := newWatchFixture(t)
	ctx := cancelledContext(t)
	query := sqlite.WatchRowQuery{ProfileID: watchProfileID, Limit: 1}

	operations := map[string]func() error{
		"UpdateStates": func() error {
			return fixture.store.UpdateStates(ctx, appwatch.StateUpdate{
				ProfileID: watchProfileID,
				Keys:      []appwatch.StateKey{{ItemID: "movie-1"}}, Change: markWatchedAt(watchNow()), Now: watchNow(),
			})
		},
		"States":           func() error { _, err := fixture.store.States(ctx, watchProfileID, []string{"movie-1"}); return err },
		"EpisodeCounts":    func() error { _, err := fixture.store.EpisodeCounts(ctx, watchProfileID, []string{"s"}); return err },
		"ContinueWatching": func() error { _, err := fixture.store.ContinueWatching(ctx, query); return err },
		"UpNext":           func() error { _, err := fixture.store.UpNext(ctx, query); return err },
	}

	for name, operation := range operations {
		if err := operation(); err == nil {
			t.Errorf("%s with a cancelled context succeeded, want an error", name)
		}
	}
}
