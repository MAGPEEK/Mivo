package sqlite_test

import (
	"maps"
	"testing"
)

func TestTopLevelItemCountsCountsMoviesAndShowsOnly(t *testing.T) {
	t.Parallel()

	store, inspection := newManagedStore(t, sequentialIDs())
	movies := mustCreate(t, store, movieCandidate("Movies", "/media/movies"))
	shows := mustCreate(t, store, movieCandidate("Shows", "/media/shows"))
	empty := mustCreate(t, store, movieCandidate("Empty", "/media/empty"))

	insert := `INSERT INTO items (id, library_id, parent_id, kind, match_key, title, sort_title, extra_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 't', 't', ?, 0, 0)`
	execute(t, inspection, insert, "m1", movies.ID, nil, "movie", "a", nil)
	execute(t, inspection, insert, "m2", movies.ID, nil, "movie", "b", nil)
	execute(t, inspection, insert, "x1", movies.ID, "m1", "extra", "c", "trailer")
	execute(t, inspection, insert, "s1", shows.ID, nil, "show", "a", nil)
	execute(t, inspection, insert, "se1", shows.ID, "s1", "season", "a", nil)
	execute(t, inspection, insert, "ep1", shows.ID, "se1", "episode", "a", nil)

	counts, err := store.TopLevelItemCounts(t.Context())
	if err != nil {
		t.Fatalf("TopLevelItemCounts() error = %v", err)
	}

	want := map[string]int64{movies.ID: 2, shows.ID: 1}
	if !maps.Equal(counts, want) {
		t.Errorf("TopLevelItemCounts() = %v, want %v (library %s has no items and no entry)", counts, want, empty.ID)
	}
}

func TestTopLevelItemCountsHonoursCancelledContext(t *testing.T) {
	t.Parallel()

	store, _ := newManagedStore(t, sequentialIDs())

	if _, err := store.TopLevelItemCounts(cancelledContext(t)); err == nil {
		t.Error("TopLevelItemCounts(cancelled) error = nil, want an error")
	}
}
