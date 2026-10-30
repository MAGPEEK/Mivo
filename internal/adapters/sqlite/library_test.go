package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// Library IDs handed out by the test generators, in order.
const (
	firstLibraryID  = "11111111-1111-4111-8111-111111111111"
	secondLibraryID = "22222222-2222-4222-8222-222222222222"
	thirdLibraryID  = "33333333-3333-4333-8333-333333333333"
	fourthLibraryID = "44444444-4444-4444-8444-444444444444"
)

// Roots are opaque to the repository; these look like what the configuration produces.
const (
	moviesRoot = "/media/movies"
	showsRoot  = "/media/shows"
	animeRoot  = "/media/anime"
)

var errIDsExhausted = errors.New("test id generator exhausted")

// syncStartMilli is the first instant of the test clock (2026-10-30 12:00 UTC) in Unix
// milliseconds.
const syncStartMilli int64 = 1_793_361_600_000

// testClock is a manually advanced clock for created_at/updated_at assertions.
type testClock struct {
	mutex sync.Mutex
	now   time.Time
}

func newTestClock() *testClock {
	return &testClock{now: time.UnixMilli(syncStartMilli).UTC()}
}

func (clock *testClock) Now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	return clock.now
}

func (clock *testClock) Advance(duration time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	clock.now = clock.now.Add(duration)
}

// idSequence returns a newID function that hands out ids in order and fails when they are
// used up, plus a function reporting how many it handed out.
func idSequence(ids ...string) (func() (string, error), func() int) {
	var (
		mutex sync.Mutex
		next  int
	)

	newID := func() (string, error) {
		mutex.Lock()
		defer mutex.Unlock()

		if next == len(ids) {
			return "", errIDsExhausted
		}

		next++

		return ids[next-1], nil
	}

	used := func() int {
		mutex.Lock()
		defer mutex.Unlock()

		return next
	}

	return newID, used
}

func newTestLibraryRepository(t *testing.T) (*sqlite.LibraryRepository, *testClock, *sql.DB) {
	t.Helper()

	db, path := openMigratedTestDB(t)
	clock := newTestClock()

	return sqlite.NewLibraryRepository(db, clock.Now), clock, openInspectionDB(t, path)
}

func mustSync(t *testing.T, repository *sqlite.LibraryRepository, definitions []library.Definition,
	newID func() (string, error),
) []library.Library {
	t.Helper()

	return mustSyncResult(t, repository, definitions, newID).Libraries
}

func mustSyncResult(t *testing.T, repository *sqlite.LibraryRepository, definitions []library.Definition,
	newID func() (string, error),
) library.SyncResult {
	t.Helper()

	result, err := repository.SyncLibraries(t.Context(), definitions, newID)
	if err != nil {
		t.Fatalf("SyncLibraries error = %v", err)
	}

	return result
}

// assertNothingRemoved fails when a sync reports removed libraries although every stored root
// is still configured.
func assertNothingRemoved(t *testing.T, result library.SyncResult) {
	t.Helper()

	if result.Removed == nil || len(result.Removed) != 0 {
		t.Errorf("SyncLibraries removed = %#v, want an empty, non-nil list", result.Removed)
	}
}

func TestSyncLibrariesInsertsNewRootsAndReturnsAllOrderedByName(t *testing.T) {
	t.Parallel()

	repository, _, inspection := newTestLibraryRepository(t)
	newID, _ := idSequence(firstLibraryID, secondLibraryID, thirdLibraryID)

	result := mustSyncResult(t, repository, []library.Definition{
		{Name: "Shows", Root: showsRoot, Kind: library.KindShows},
		{Name: "anime", Root: animeRoot, Kind: library.KindMovies},
		{Name: "Movies", Root: moviesRoot, Kind: library.KindMovies},
	}, newID)
	synced := result.Libraries
	assertNothingRemoved(t, result)

	want := []library.Library{
		{ID: secondLibraryID, Name: "anime", Root: animeRoot, Kind: library.KindMovies},
		{ID: thirdLibraryID, Name: "Movies", Root: moviesRoot, Kind: library.KindMovies},
		{ID: firstLibraryID, Name: "Shows", Root: showsRoot, Kind: library.KindShows},
	}
	if !slices.Equal(synced, want) {
		t.Errorf("SyncLibraries = %+v, want %+v", synced, want)
	}

	for _, stored := range want {
		created, updated := libraryTimestamps(t, inspection, stored.ID)
		if created != syncStartMilli || updated != syncStartMilli {
			t.Errorf("library %s created_at, updated_at = %d, %d, want both %d",
				stored.Name, created, updated, syncStartMilli)
		}
	}
}

func TestSyncLibrariesTwiceWithTheSameInputChangesNothing(t *testing.T) {
	t.Parallel()

	repository, clock, inspection := newTestLibraryRepository(t)
	newID, used := idSequence(firstLibraryID, secondLibraryID, thirdLibraryID)
	definitions := []library.Definition{
		{Name: "Movies", Root: moviesRoot, Kind: library.KindMovies},
		{Name: "Shows", Root: showsRoot, Kind: library.KindShows},
	}

	first := mustSync(t, repository, definitions, newID)

	clock.Advance(time.Hour)

	secondResult := mustSyncResult(t, repository, definitions, newID)
	second := secondResult.Libraries
	assertNothingRemoved(t, secondResult)

	if !slices.Equal(first, second) {
		t.Errorf("second SyncLibraries = %+v, want the first result %+v", second, first)
	}

	if got := used(); got != len(definitions) {
		t.Errorf("ids generated = %d, want %d (the second sync must not create ids)", got, len(definitions))
	}

	for _, stored := range second {
		if _, updated := libraryTimestamps(t, inspection, stored.ID); updated != syncStartMilli {
			t.Errorf("library %s updated_at = %d, want unchanged %d", stored.Name, updated, syncStartMilli)
		}
	}
}

// A library's identity is its root: a new name or kind for a known root keeps the ID and the
// items, and only that library's updated_at moves.
func TestSyncLibrariesUpdatesNameAndKindOfAKnownRoot(t *testing.T) {
	t.Parallel()

	repository, clock, inspection := newTestLibraryRepository(t)
	newID, _ := idSequence(firstLibraryID, secondLibraryID)

	mustSync(t, repository, []library.Definition{
		{Name: "Movies", Root: moviesRoot, Kind: library.KindMovies},
		{Name: "Shows", Root: showsRoot, Kind: library.KindShows},
	}, newID)
	insertCatalogTree(t, inspection, firstLibraryID, "kept")

	clock.Advance(time.Minute)

	result := mustSyncResult(t, repository, []library.Definition{
		{Name: "Films", Root: moviesRoot, Kind: library.KindShows},
		{Name: "Shows", Root: showsRoot, Kind: library.KindShows},
	}, newID)
	synced := result.Libraries

	// A renamed or re-kinded library keeps its root, so it is not reported as removed.
	assertNothingRemoved(t, result)

	want := []library.Library{
		{ID: firstLibraryID, Name: "Films", Root: moviesRoot, Kind: library.KindShows},
		{ID: secondLibraryID, Name: "Shows", Root: showsRoot, Kind: library.KindShows},
	}
	if !slices.Equal(synced, want) {
		t.Errorf("SyncLibraries = %+v, want %+v", synced, want)
	}

	renamedAt := syncStartMilli + time.Minute.Milliseconds()
	assertTimestamps(t, inspection, firstLibraryID, libraryTimes{created: syncStartMilli, updated: renamedAt})
	assertTimestamps(t, inspection, secondLibraryID, libraryTimes{created: syncStartMilli, updated: syncStartMilli})

	// Milestone 2 derives every item's kind from its library's kind.
	if got := itemKinds(t, inspection, firstLibraryID); !slices.Equal(got, []string{"episode"}) {
		t.Errorf("item kinds after the kind change = %v, want [episode]", got)
	}
}

// A root that is no longer configured is deleted together with its items, files and
// streams (ON DELETE CASCADE through the adapter's writer connection); other libraries keep
// theirs.
func TestSyncLibrariesRemovesUnconfiguredRootsWithTheirCatalog(t *testing.T) {
	t.Parallel()

	repository, _, inspection := newTestLibraryRepository(t)
	newID, _ := idSequence(firstLibraryID, secondLibraryID)

	before := mustSync(t, repository, []library.Definition{
		{Name: "Movies", Root: moviesRoot, Kind: library.KindMovies},
		{Name: "Shows", Root: showsRoot, Kind: library.KindShows},
	}, newID)
	insertCatalogTree(t, inspection, firstLibraryID, "removed")
	insertCatalogTree(t, inspection, firstLibraryID, "removed-too")
	insertCatalogTree(t, inspection, secondLibraryID, "kept")

	result := mustSyncResult(t, repository, []library.Definition{
		{Name: "Shows", Root: showsRoot, Kind: library.KindShows},
	}, newID)

	want := []library.Library{{ID: secondLibraryID, Name: "Shows", Root: showsRoot, Kind: library.KindShows}}
	if !slices.Equal(result.Libraries, want) {
		t.Errorf("SyncLibraries = %+v, want %+v", result.Libraries, want)
	}

	// The removed library is reported with the item count it had before the delete.
	wantRemoved := []library.Summary{{Library: before[0], ItemCount: 2}}
	if !slices.Equal(result.Removed, wantRemoved) {
		t.Errorf("SyncLibraries removed = %+v, want %+v", result.Removed, wantRemoved)
	}

	wantRows := map[string]int{"libraries": 1, "items": 1, "media_files": 1, "media_streams": catalogTreeStreams}
	for table, want := range wantRows {
		if got := rowCount(t, inspection, table); got != want {
			t.Errorf("%s rows after removal = %d, want %d (only the kept library's rows)", table, got, want)
		}
	}
}

func TestSyncLibrariesWithoutDefinitionsRemovesEveryLibrary(t *testing.T) {
	t.Parallel()

	repository, _, inspection := newTestLibraryRepository(t)
	newID, _ := idSequence(firstLibraryID, secondLibraryID)

	before := mustSync(t, repository, []library.Definition{
		{Name: "Shows", Root: showsRoot, Kind: library.KindShows},
		{Name: "Movies", Root: moviesRoot, Kind: library.KindMovies},
	}, newID)

	result := mustSyncResult(t, repository, nil, newID)
	if result.Libraries == nil || len(result.Libraries) != 0 {
		t.Errorf("SyncLibraries(nil) = %#v, want an empty, non-nil list", result.Libraries)
	}

	// Ordered by name like the library list, each with its (empty) item count.
	wantRemoved := []library.Summary{{Library: before[0]}, {Library: before[1]}}
	if !slices.Equal(result.Removed, wantRemoved) {
		t.Errorf("SyncLibraries(nil) removed = %+v, want %+v", result.Removed, wantRemoved)
	}

	if got := rowCount(t, inspection, "libraries"); got != 0 {
		t.Errorf("libraries rows = %d, want 0", got)
	}
}

// Invalid input is rejected before the transaction starts, so the stored libraries stay as
// they were.
func TestSyncLibrariesRejectsInvalidInputAndKeepsTheStoredLibraries(t *testing.T) {
	t.Parallel()

	movies := library.Definition{Name: "Movies", Root: moviesRoot, Kind: library.KindMovies}
	validIDs := func() (string, error) { return fourthLibraryID, nil }

	tests := []struct {
		name        string
		definitions []library.Definition
		newID       func() (string, error)
	}{
		{name: "nil id generator", definitions: []library.Definition{movies}},
		{name: "duplicate root", definitions: []library.Definition{movies, {Name: "Other", Root: moviesRoot, Kind: library.KindShows}}, newID: validIDs},
		{name: "empty name", definitions: []library.Definition{{Root: animeRoot, Kind: library.KindMovies}}, newID: validIDs},
		{name: "unknown kind", definitions: []library.Definition{{Name: "Music", Root: animeRoot, Kind: "music"}}, newID: validIDs},
		{name: "empty root", definitions: []library.Definition{{Name: "Nowhere", Kind: library.KindMovies}}, newID: validIDs},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			repository, _, _ := newTestLibraryRepository(t)
			newID, _ := idSequence(firstLibraryID)
			before := mustSync(t, repository, []library.Definition{{Name: "Shows", Root: showsRoot, Kind: library.KindShows}}, newID)

			_, err := repository.SyncLibraries(t.Context(), test.definitions, test.newID)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("SyncLibraries error = %v, want domain.ErrInvalid", err)
			}

			assertStoredLibraries(t, repository, before)
		})
	}
}

// When an ID cannot be created halfway through, the whole sync is rolled back: the update
// and the removal done before the failure are undone as well.
func TestSyncLibrariesRollsBackEverythingWhenAnIDCannotBeCreated(t *testing.T) {
	t.Parallel()

	repository, _, _ := newTestLibraryRepository(t)
	newID, _ := idSequence(firstLibraryID, secondLibraryID)
	before := mustSync(t, repository, []library.Definition{
		{Name: "Movies", Root: moviesRoot, Kind: library.KindMovies},
		{Name: "Shows", Root: showsRoot, Kind: library.KindShows},
	}, newID)

	failingID := func() (string, error) { return "", errIDsExhausted }

	_, err := repository.SyncLibraries(t.Context(), []library.Definition{
		{Name: "Films", Root: moviesRoot, Kind: library.KindMovies},
		{Name: "Anime", Root: animeRoot, Kind: library.KindMovies},
	}, failingID)
	if !errors.Is(err, errIDsExhausted) {
		t.Fatalf("SyncLibraries error = %v, want the generator error", err)
	}

	assertStoredLibraries(t, repository, before)
}

// Each case makes one statement of the sync fail through a trigger. The sync below renames
// and re-kinds Movies (library and item update), adds Anime (insert) and removes Shows
// (delete); whichever step fails, the steps before it are rolled back too.
func TestSyncLibrariesRollsBackEverythingWhenAStatementFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		trigger string
	}{
		{name: "library update", trigger: "BEFORE UPDATE ON libraries"},
		{name: "item kind update", trigger: "BEFORE UPDATE ON items"},
		{name: "library insert", trigger: "BEFORE INSERT ON libraries"},
		{name: "library delete", trigger: "BEFORE DELETE ON libraries"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			repository, _, inspection := newTestLibraryRepository(t)
			newID, _ := idSequence(firstLibraryID, secondLibraryID, thirdLibraryID)
			before := mustSync(t, repository, []library.Definition{
				{Name: "Movies", Root: moviesRoot, Kind: library.KindMovies},
				{Name: "Shows", Root: showsRoot, Kind: library.KindShows},
			}, newID)
			insertCatalogTree(t, inspection, firstLibraryID, "a")

			failStatement(t, inspection, test.trigger)

			_, err := repository.SyncLibraries(t.Context(), []library.Definition{
				{Name: "Films", Root: moviesRoot, Kind: library.KindShows},
				{Name: "Anime", Root: animeRoot, Kind: library.KindMovies},
			}, newID)
			if err == nil {
				t.Fatal("SyncLibraries error = nil, want the trigger's error")
			}

			assertStoredLibraries(t, repository, before)

			if got := itemKinds(t, inspection, firstLibraryID); !slices.Equal(got, []string{"movie"}) {
				t.Errorf("item kinds = %v, want unchanged [movie]", got)
			}
		})
	}
}

// failStatement installs a trigger that aborts every statement matching event, for example
// "BEFORE DELETE ON libraries".
func failStatement(t *testing.T, inspection *sql.DB, event string) {
	t.Helper()

	// The event comes from the test table, never from input.
	_, err := inspection.ExecContext(t.Context(),
		"CREATE TRIGGER fail_statement "+event+" BEGIN SELECT RAISE(ABORT, 'statement failed by test'); END")
	if err != nil {
		t.Fatalf("create trigger %s: %v", event, err)
	}
}

func TestListLibrariesReturnsTheItemCountOfEachLibrary(t *testing.T) {
	t.Parallel()

	repository, _, inspection := newTestLibraryRepository(t)
	newID, _ := idSequence(firstLibraryID, secondLibraryID)
	synced := mustSync(t, repository, []library.Definition{
		{Name: "Movies", Root: moviesRoot, Kind: library.KindMovies},
		{Name: "Shows", Root: showsRoot, Kind: library.KindShows},
	}, newID)

	for _, suffix := range []string{"a", "b", "c"} {
		insertCatalogTree(t, inspection, firstLibraryID, suffix)
	}

	listed, err := repository.ListLibraries(t.Context())
	if err != nil {
		t.Fatalf("ListLibraries error = %v", err)
	}

	want := []library.Summary{
		{Library: synced[0], ItemCount: 3},
		{Library: synced[1], ItemCount: 0},
	}
	if !slices.Equal(listed, want) {
		t.Errorf("ListLibraries = %+v, want %+v", listed, want)
	}
}

func TestListLibrariesOfAnEmptyCatalogIsAnEmptyList(t *testing.T) {
	t.Parallel()

	repository, _, _ := newTestLibraryRepository(t)

	listed, err := repository.ListLibraries(t.Context())
	if err != nil || listed == nil || len(listed) != 0 {
		t.Errorf("ListLibraries = %#v, %v, want an empty, non-nil list", listed, err)
	}
}

func TestGetLibraryReturnsTheLibraryWithItsItemCount(t *testing.T) {
	t.Parallel()

	repository, _, inspection := newTestLibraryRepository(t)
	newID, _ := idSequence(firstLibraryID)
	synced := mustSync(t, repository, []library.Definition{{Name: "Shows", Root: showsRoot, Kind: library.KindShows}}, newID)

	insertCatalogTree(t, inspection, firstLibraryID, "a")
	insertCatalogTree(t, inspection, firstLibraryID, "b")

	got, err := repository.GetLibrary(t.Context(), firstLibraryID)
	if err != nil {
		t.Fatalf("GetLibrary error = %v", err)
	}

	if want := (library.Summary{Library: synced[0], ItemCount: 2}); got != want {
		t.Errorf("GetLibrary = %+v, want %+v", got, want)
	}
}

func TestGetLibraryOfAnUnknownIDReturnsNotFound(t *testing.T) {
	t.Parallel()

	repository, _, _ := newTestLibraryRepository(t)

	_, err := repository.GetLibrary(t.Context(), firstLibraryID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetLibrary error = %v, want domain.ErrNotFound", err)
	}
}

// A stored kind the domain does not know (e.g. edited by hand, or written by a newer version
// after a downgrade) is reported, not handed to clients. The CHECK constraint is bypassed by
// rebuilding the row with ignore_check_constraints.
func TestReadingAStoredUnknownKindReturnsInvalid(t *testing.T) {
	t.Parallel()

	repository, _, inspection := newTestLibraryRepository(t)
	newID, _ := idSequence(firstLibraryID)
	mustSync(t, repository, []library.Definition{{Name: "Movies", Root: moviesRoot, Kind: library.KindMovies}}, newID)

	storeUncheckedLibraryKind(t, inspection, firstLibraryID, "music")

	if _, err := repository.ListLibraries(t.Context()); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("ListLibraries error = %v, want domain.ErrInvalid", err)
	}

	if _, err := repository.GetLibrary(t.Context(), firstLibraryID); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("GetLibrary error = %v, want domain.ErrInvalid", err)
	}

	definitions := []library.Definition{{Name: "Movies", Root: moviesRoot, Kind: library.KindMovies}}
	if _, err := repository.SyncLibraries(t.Context(), definitions, newID); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("SyncLibraries error = %v, want domain.ErrInvalid", err)
	}
}

func TestLibraryRepositoryReportsDatabaseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ctx     func(t *testing.T) context.Context
		closeDB bool
		wantErr error
	}{
		{name: "cancelled context", ctx: cancelledContext, wantErr: context.Canceled},
		{name: "closed database", ctx: func(t *testing.T) context.Context { return t.Context() }, closeDB: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			db, _ := openMigratedTestDB(t)
			repository := sqlite.NewLibraryRepository(db, newTestClock().Now)

			if test.closeDB {
				if err := db.Close(); err != nil {
					t.Fatalf("Close error = %v", err)
				}
			}

			newID, _ := idSequence(firstLibraryID)
			definitions := []library.Definition{{Name: "Movies", Root: moviesRoot, Kind: library.KindMovies}}

			_, syncErr := repository.SyncLibraries(test.ctx(t), definitions, newID)
			_, listErr := repository.ListLibraries(test.ctx(t))
			_, getErr := repository.GetLibrary(test.ctx(t), firstLibraryID)

			errs := map[string]error{"SyncLibraries": syncErr, "ListLibraries": listErr, "GetLibrary": getErr}
			for operation, err := range errs {
				assertDatabaseError(t, operation, err, test.wantErr)
			}
		})
	}
}

// A nil clock falls back to the system clock, so timestamps are still written.
func TestNewLibraryRepositoryWithoutClockUsesTheSystemClock(t *testing.T) {
	t.Parallel()

	db, path := openMigratedTestDB(t)
	repository := sqlite.NewLibraryRepository(db, nil)
	newID, _ := idSequence(firstLibraryID)

	before := time.Now().UnixMilli()

	mustSync(t, repository, []library.Definition{{Name: "Movies", Root: moviesRoot, Kind: library.KindMovies}}, newID)

	created, _ := libraryTimestamps(t, openInspectionDB(t, path), firstLibraryID)
	if created < before || created > time.Now().UnixMilli() {
		t.Errorf("created_at = %d, want the current time (>= %d)", created, before)
	}
}

func assertStoredLibraries(t *testing.T, repository *sqlite.LibraryRepository, want []library.Library) {
	t.Helper()

	listed, err := repository.ListLibraries(t.Context())
	if err != nil {
		t.Fatalf("ListLibraries error = %v", err)
	}

	got := make([]library.Library, 0, len(listed))
	for _, summary := range listed {
		got = append(got, summary.Library)
	}

	if !slices.Equal(got, want) {
		t.Errorf("stored libraries = %+v, want unchanged %+v", got, want)
	}
}

// libraryTimes are a library's created_at and updated_at in Unix milliseconds.
type libraryTimes struct {
	created, updated int64
}

func assertTimestamps(t *testing.T, inspection *sql.DB, libraryID string, want libraryTimes) {
	t.Helper()

	created, updated := libraryTimestamps(t, inspection, libraryID)
	if got := (libraryTimes{created: created, updated: updated}); got != want {
		t.Errorf("library %s created_at, updated_at = %+v, want %+v", libraryID, got, want)
	}
}

func libraryTimestamps(t *testing.T, inspection *sql.DB, libraryID string) (created, updated int64) {
	t.Helper()

	err := inspection.QueryRowContext(t.Context(),
		"SELECT created_at, updated_at FROM libraries WHERE id = ?", libraryID).Scan(&created, &updated)
	if err != nil {
		t.Fatalf("read timestamps of library %s: %v", libraryID, err)
	}

	return created, updated
}

func itemKinds(t *testing.T, inspection *sql.DB, libraryID string) []string {
	t.Helper()

	rows, err := inspection.QueryContext(t.Context(),
		"SELECT kind FROM items WHERE library_id = ? ORDER BY id", libraryID)
	if err != nil {
		t.Fatalf("read item kinds: %v", err)
	}

	defer func() { _ = rows.Close() }()

	kinds := []string{}

	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			t.Fatalf("scan item kind: %v", err)
		}

		kinds = append(kinds, kind)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("read item kinds: %v", err)
	}

	return kinds
}

func rowCount(t *testing.T, inspection *sql.DB, table string) int {
	t.Helper()

	var count int

	// The table name comes from the test itself, never from input.
	err := inspection.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count)
	if err != nil {
		t.Fatalf("count rows of %s: %v", table, err)
	}

	return count
}

// catalogTreeStreams is the number of streams insertCatalogTree stores for its file.
const catalogTreeStreams = 2

// insertCatalogTree stores one item with one file and two streams in a library, the shape a
// scan of one video file produces. The rows are written directly because the media queries
// arrive with M2-15.
func insertCatalogTree(t *testing.T, inspection *sql.DB, libraryID, suffix string) {
	t.Helper()

	itemID, fileID := "item-"+libraryID+"-"+suffix, "file-"+libraryID+"-"+suffix
	statements := []struct {
		query string
		args  []any
	}{
		{
			query: `INSERT INTO items (id, library_id, kind, title, sort_title, created_at, updated_at)
				VALUES (?, ?, 'movie', ?, ?, 0, 0)`,
			args: []any{itemID, libraryID, "Title " + suffix, "title " + suffix},
		},
		{query: insertMediaFileQuery, args: []any{fileID, itemID, libraryID, suffix + ".mkv"}},
		{query: insertMediaStreamQuery, args: []any{fileID, 0, "video"}},
		{query: insertMediaStreamQuery, args: []any{fileID, 1, "audio"}},
	}

	for _, statement := range statements {
		if _, err := inspection.ExecContext(t.Context(), statement.query, statement.args...); err != nil {
			t.Fatalf("insert catalog row for %s: %v", suffix, err)
		}
	}
}

// insertMediaFileQuery stores a successfully probed file; parameters: id, item_id,
// library_id, relative_path.
const insertMediaFileQuery = `INSERT INTO media_files (id, item_id, library_id, relative_path, size_bytes,
	modified_at, container, format_name, duration_ms, bit_rate, probed_at, probe_error)
	VALUES (?, ?, ?, ?, 1048576, 0, 'mkv', 'matroska,webm', 30000, 2000000, 0, NULL)`

// insertMediaStreamQuery stores a stream; parameters: media_file_id, stream_index, kind.
const insertMediaStreamQuery = `INSERT INTO media_streams (media_file_id, stream_index, kind, codec,
	codec_tag, profile, level, pixel_format, bit_depth, width, height, frame_rate, color_primaries,
	color_transfer, color_space, hdr, channels, channel_layout, sample_rate, language, title,
	is_default, is_forced, is_attached_picture, bit_rate)
	VALUES (?, ?, ?, 'h264', NULL, 'High', 40, 'yuv420p', 8, 1920, 1080, '24000/1001', '', '', '',
	'', 0, '', 0, 'eng', '', 1, 0, 0, 0)`

// storeUncheckedLibraryKind writes a kind the CHECK constraint would refuse, to simulate a
// row the domain cannot read.
func storeUncheckedLibraryKind(t *testing.T, inspection *sql.DB, libraryID, kind string) {
	t.Helper()

	connection, err := inspection.Conn(t.Context())
	if err != nil {
		t.Fatalf("check out inspection connection: %v", err)
	}

	defer func() { _ = connection.Close() }()

	statements := []struct {
		query string
		args  []any
	}{
		{query: "PRAGMA ignore_check_constraints = ON"},
		{query: "UPDATE libraries SET kind = ? WHERE id = ?", args: []any{kind, libraryID}},
		{query: "PRAGMA ignore_check_constraints = OFF"},
	}

	for _, statement := range statements {
		if _, err := connection.ExecContext(t.Context(), statement.query, statement.args...); err != nil {
			t.Fatalf("%s: %v", statement.query, err)
		}
	}
}
