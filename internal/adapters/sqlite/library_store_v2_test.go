package sqlite_test

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// sequentialIDs hands out "id-1", "id-2", ... and never runs out.
func sequentialIDs() func() (string, error) {
	next := 0

	return func() (string, error) {
		next++

		return fmt.Sprintf("id-%d", next), nil
	}
}

func newManagedStore(t *testing.T, newID func() (string, error)) (*sqlite.ManagedLibraryStore, *sql.DB) {
	t.Helper()

	db, path := openMigratedTestDB(t)

	return sqlite.NewManagedLibraryStore(db, newTestClock().Now, newID), openInspectionDB(t, path)
}

func movieCandidate(name string, paths ...string) library.ManagedLibrary {
	folders := make([]library.Folder, 0, len(paths))
	for _, path := range paths {
		folders = append(folders, library.Folder{Path: path})
	}

	return library.ManagedLibrary{
		Name: name, Kind: library.KindMovies, Folders: folders, MetadataLanguage: "de",
		Options: library.Options{ScanExtras: true},
	}
}

func mustCreate(t *testing.T, store *sqlite.ManagedLibraryStore, candidate library.ManagedLibrary) library.ManagedLibrary {
	t.Helper()

	created, err := store.CreateLibrary(t.Context(), candidate)
	if err != nil {
		t.Fatalf("CreateLibrary(%q) error = %v", candidate.Name, err)
	}

	return created
}

func TestCreateLibraryRoundTrip(t *testing.T) {
	t.Parallel()

	store, _ := newManagedStore(t, sequentialIDs())
	candidate := movieCandidate("Movies", "/media/a", "/media/b")
	candidate.ImportedFromConfig = true

	created := mustCreate(t, store, candidate)

	if created.ID != "id-1" || created.Folders[0].ID != "id-2" || created.Folders[1].ID != "id-3" {
		t.Errorf("created ids = %q %+v, want id-1 with folders id-2, id-3", created.ID, created.Folders)
	}

	got, err := store.GetManaged(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("GetManaged error = %v", err)
	}

	if got.Name != "Movies" || got.Kind != library.KindMovies || got.MetadataLanguage != "de" ||
		!got.ImportedFromConfig || got.Options != candidate.Options || !slices.Equal(got.Folders, created.Folders) {
		t.Errorf("GetManaged = %+v, want %+v", got, created)
	}
}

func TestCreateLibraryRejectsAFolderOfAnotherLibrary(t *testing.T) {
	t.Parallel()

	store, inspection := newManagedStore(t, sequentialIDs())
	mustCreate(t, store, movieCandidate("One", "/media/a"))

	_, err := store.CreateLibrary(t.Context(), movieCandidate("Two", "/media/ok", "/media/a"))
	if !errors.Is(err, domain.ErrConflict) || !strings.Contains(err.Error(), "folder 1") {
		t.Fatalf("error = %v, want ErrConflict naming folder 1", err)
	}

	// The failing second folder rolls the whole library back.
	if got := rowCount(t, inspection, "libraries"); got != 1 {
		t.Errorf("libraries = %d, want 1 (no partial library)", got)
	}

	if got := rowCount(t, inspection, "library_roots"); got != 1 {
		t.Errorf("folders = %d, want 1", got)
	}
}

func TestCreateLibraryRollsBackWhenTheIDGeneratorFails(t *testing.T) {
	t.Parallel()

	newID, _ := idSequence("only-library-id")
	store, inspection := newManagedStore(t, newID)

	if _, err := store.CreateLibrary(t.Context(), movieCandidate("One", "/media/a")); !errors.Is(err, errIDsExhausted) {
		t.Fatalf("error = %v, want errIDsExhausted", err)
	}

	if got := rowCount(t, inspection, "libraries"); got != 0 {
		t.Errorf("libraries = %d, want 0", got)
	}
}

func seedCatalog(t *testing.T, inspection *sql.DB, libraryID string, folder library.Folder) {
	t.Helper()

	name := strings.TrimPrefix(folder.Path, "/media/")
	itemID := "item-" + name
	insertItem(t, inspection, newItemSpec(itemID, libraryID, "movie", name))
	execute(t, inspection, `INSERT INTO media_files (id, item_id, library_id, root_id, relative_path,
		size_bytes, modified_at, container, format_name, duration_ms, bit_rate, probed_at)
		VALUES (?, ?, ?, ?, ?, 1, 0, 'mkv', '', 0, 0, 0)`, "file-"+name, itemID, libraryID, folder.ID, name+".mkv")
}

func TestUpdateLibraryKeepsAddsRemovesAndRenumbersFolders(t *testing.T) {
	t.Parallel()

	store, inspection := newManagedStore(t, sequentialIDs())
	created := mustCreate(t, store, movieCandidate("Movies", "/media/a", "/media/b", "/media/c"))
	keptA, removedB, keptC := created.Folders[0], created.Folders[1], created.Folders[2]
	seedCatalog(t, inspection, created.ID, keptA)
	seedCatalog(t, inspection, created.ID, removedB)
	seedCatalog(t, inspection, created.ID, keptC)

	updated := created
	updated.Name = "Films"
	updated.MetadataLanguage = "fr"
	updated.Options = library.Options{ReadNFO: true}
	updated.Folders = []library.Folder{keptC, {Path: "/media/new"}, keptA}

	if err := store.UpdateLibrary(t.Context(), updated); err != nil {
		t.Fatalf("UpdateLibrary error = %v", err)
	}

	got, err := store.GetManaged(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("GetManaged error = %v", err)
	}

	wantFolders := []library.Folder{keptC, {ID: "id-5", Path: "/media/new"}, keptA}
	if got.Name != "Films" || got.MetadataLanguage != "fr" || got.Options != updated.Options ||
		got.Kind != library.KindMovies || !slices.Equal(got.Folders, wantFolders) {
		t.Errorf("GetManaged = %+v, want folders %+v", got, wantFolders)
	}

	files := queryStrings(t, inspection, "SELECT id FROM media_files ORDER BY id")
	if want := []string{"file-a", "file-c"}; !slices.Equal(files, want) {
		t.Errorf("files after removing folder b = %q, want %q", files, want)
	}

	if got := rowCount(t, inspection, "items"); got != 3 {
		t.Errorf("items = %d, want 3 (orphans are removed by the next scan)", got)
	}
}

func TestUpdateLibraryMovesAFolderToAnotherLibraryAfterRemoval(t *testing.T) {
	t.Parallel()

	store, _ := newManagedStore(t, sequentialIDs())
	one := mustCreate(t, store, movieCandidate("One", "/media/a"))
	two := mustCreate(t, store, movieCandidate("Two", "/media/b"))

	conflicting := two
	conflicting.Folders = append(slices.Clone(two.Folders), library.Folder{Path: "/media/a"})

	if err := store.UpdateLibrary(t.Context(), conflicting); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("error = %v, want ErrConflict", err)
	}

	// The failed update left both libraries as they were.
	got, err := store.GetManaged(t.Context(), two.ID)
	if err != nil || len(got.Folders) != 1 {
		t.Fatalf("library two = %+v, %v; want its single folder", got, err)
	}

	if err := store.UpdateLibrary(t.Context(), library.ManagedLibrary{
		ID: one.ID, Name: "One", Folders: []library.Folder{{Path: "/media/z"}},
	}); err != nil {
		t.Fatalf("UpdateLibrary error = %v", err)
	}

	if err := store.UpdateLibrary(t.Context(), conflicting); err != nil {
		t.Fatalf("UpdateLibrary after the folder was freed: %v", err)
	}
}

func TestUpdateLibraryUnknownIDsAreNotFound(t *testing.T) {
	t.Parallel()

	store, _ := newManagedStore(t, sequentialIDs())
	one := mustCreate(t, store, movieCandidate("One", "/media/a"))
	two := mustCreate(t, store, movieCandidate("Two", "/media/b"))

	unknown := movieCandidate("Ghost", "/media/g")
	unknown.ID = "ghost"

	foreign := one
	foreign.Folders = two.Folders

	for name, candidate := range map[string]library.ManagedLibrary{"library": unknown, "folder of another library": foreign} {
		if err := store.UpdateLibrary(t.Context(), candidate); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s: error = %v, want ErrNotFound", name, err)
		}
	}

	got, err := store.GetManaged(t.Context(), one.ID)
	if err != nil || !slices.Equal(got.Folders, one.Folders) {
		t.Errorf("library one = %+v, %v; want it unchanged", got, err)
	}
}

func TestDeleteLibraryCascadesAndReportsUnknownIDs(t *testing.T) {
	t.Parallel()

	store, inspection := newManagedStore(t, sequentialIDs())
	created := mustCreate(t, store, movieCandidate("Movies", "/media/a"))
	seedCatalog(t, inspection, created.ID, created.Folders[0])

	if err := store.DeleteLibrary(t.Context(), created.ID); err != nil {
		t.Fatalf("DeleteLibrary error = %v", err)
	}

	for _, table := range []string{"libraries", "library_roots", "items", "media_files"} {
		if got := rowCount(t, inspection, table); got != 0 {
			t.Errorf("rows of %s = %d, want 0", table, got)
		}
	}

	if err := store.DeleteLibrary(t.Context(), created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("second delete error = %v, want ErrNotFound", err)
	}

	if _, err := store.GetManaged(t.Context(), created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetManaged error = %v, want ErrNotFound", err)
	}
}

func TestListManagedOrdersByNameAndFoldersByPosition(t *testing.T) {
	t.Parallel()

	store, _ := newManagedStore(t, sequentialIDs())
	mustCreate(t, store, movieCandidate("zebra", "/media/z"))
	mustCreate(t, store, movieCandidate("Anime", "/media/b2", "/media/b1"))
	mustCreate(t, store, movieCandidate("anime", "/media/c"))

	listed, err := store.ListManaged(t.Context())
	if err != nil {
		t.Fatalf("ListManaged error = %v", err)
	}

	var names []string
	for _, managed := range listed {
		names = append(names, managed.Name)
	}

	if want := []string{"Anime", "anime", "zebra"}; !slices.Equal(names, want) {
		t.Errorf("names = %q, want %q", names, want)
	}

	if listed[0].Folders[0].Path != "/media/b2" || listed[0].Folders[1].Path != "/media/b1" {
		t.Errorf("folders = %+v, want request order", listed[0].Folders)
	}
}

func TestStoredUnknownKindIsAPlainError(t *testing.T) {
	t.Parallel()

	store, inspection := newManagedStore(t, sequentialIDs())
	created := mustCreate(t, store, movieCandidate("Movies", "/media/a"))

	connection, err := inspection.Conn(t.Context())
	if err != nil {
		t.Fatalf("connection: %v", err)
	}

	defer func() { _ = connection.Close() }()

	// The pragma is per connection, so both statements share one.
	for _, statement := range []string{"PRAGMA ignore_check_constraints = 1", "UPDATE libraries SET kind = 'bogus'"} {
		if _, err := connection.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}

	_, err = store.GetManaged(t.Context(), created.ID)
	if err == nil || errors.Is(err, domain.ErrInvalid) {
		t.Errorf("error = %v, want a plain error", err)
	}
}

func TestImportLedgerIsIdempotent(t *testing.T) {
	t.Parallel()

	store, inspection := newManagedStore(t, sequentialIDs())
	roots := []string{"/media/a", "/media/b", "/media/c"}

	pending, err := store.NeverImported(t.Context(), roots)
	if err != nil || !slices.Equal(pending, roots) {
		t.Fatalf("NeverImported = %q, %v; want all roots", pending, err)
	}

	for range 2 {
		if err := store.RecordImported(t.Context(), roots[:2]); err != nil {
			t.Fatalf("RecordImported error = %v", err)
		}
	}

	if got := rowCount(t, inspection, "library_imports"); got != 2 {
		t.Errorf("ledger rows = %d, want 2", got)
	}

	pending, err = store.NeverImported(t.Context(), append([]string{"/media/a/"}, roots[1:]...))
	if err != nil || !slices.Equal(pending, []string{"/media/c"}) {
		t.Errorf("NeverImported = %q, %v; want only /media/c (trailing slash matches)", pending, err)
	}
}
