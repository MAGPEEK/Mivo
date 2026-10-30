package sqlite_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/domain/library"
)

// A failed sync must return the single writer connection: the rollback tests read through
// the reader pool and stay green even when a failed write leaks its transaction, but then
// the next write would wait for the connection until its deadline. The catalog writes have
// the same check in catalog_writer_test.go.
func TestSyncLibrariesAfterAFailedSyncSucceeds(t *testing.T) {
	t.Parallel()

	repository, _, inspection := newTestLibraryRepository(t)
	newID, _ := idSequence(firstLibraryID, secondLibraryID)
	definitions := []library.Definition{
		{Name: "Movies", Root: moviesRoot, Kind: library.KindMovies},
		{Name: "Shows", Root: showsRoot, Kind: library.KindShows},
	}

	failStatement(t, inspection, "BEFORE INSERT ON libraries")

	if _, err := repository.SyncLibraries(withWriteTimeout(t), definitions, newID); err == nil {
		t.Fatal("SyncLibraries error = nil, want the trigger's error")
	}

	execute(t, inspection, "DROP TRIGGER fail_statement")

	retryID, _ := idSequence(firstLibraryID, secondLibraryID)
	if _, err := repository.SyncLibraries(withWriteTimeout(t), definitions, retryID); err != nil {
		t.Fatalf("SyncLibraries after a failed sync error = %v", err)
	}

	if got := rowCount(t, inspection, "libraries"); got != len(definitions) {
		t.Errorf("libraries = %d, want %d", got, len(definitions))
	}
}
