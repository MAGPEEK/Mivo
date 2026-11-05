package corpustest_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/fs/libraryfs"
	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/adapters/uuidgen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	approws "github.com/magpeek/mivo/internal/app/rows"
	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/rows"
)

// Unavailable stand-ins next to the corpus: a file ffprobe cannot read, and a real clip in a
// folder that is removed after the scan (an offline share). The corpus itself has no
// unplayable file the scan would mark (encrypted discs are found only at playback).
const (
	unprobeableMovie = "Broken Movie (2024)/Broken Movie (2024).mkv"
	offlineMovie     = "Offline Movie (2024)/Offline Movie (2024).mp4"
	offlineSource    = "movies/Night of the Living Dead (1968).mp4"
)

// rowsCorpus is the library corpus scanned into a real SQLite catalog: a movies library of
// the movies, the discs and two folders of unavailable stand-ins, and a shows library.
type rowsCorpus struct {
	database *sqlite.DB
	store    *sqlite.ManagedLibraryStore
	files    *libraryfs.FileSystem
	scans    *scan.Service
	moviesID string
	showsID  string
}

// TestCorpusRowsLeaveOutUnavailableItems builds the Home and Overview rows (task M5-13) on
// the scanned library corpus: the rows are filled, and no item the catalog marks as
// unavailable (unprobeable, or in an offline folder) appears in any of them.
func TestCorpusRowsLeaveOutUnavailableItems(t *testing.T) {
	readers, root := requireMusicCorpus(t)
	readLibraryManifest(t, root)

	brokenFolder, offlineFolder := standInFolders(t, root)
	corpus := scanRowsCorpus(t, readers, root, brokenFolder, offlineFolder)

	if err := os.RemoveAll(offlineFolder); err != nil {
		t.Fatalf("take the stand-in folder offline: %v", err)
	}

	items, service := corpus.services(t)

	unavailable := unavailableItems(t, items, corpus.moviesID)
	if len(unavailable) != 2 {
		t.Fatalf("unavailable movies = %v, want the two stand-ins", unavailable)
	}

	movies, err := service.LibraryRows(t.Context(), corpus.moviesID, approws.MaxItemLimit)
	if err != nil {
		t.Fatalf("LibraryRows(movies): %v", err)
	}

	shows, err := service.LibraryRows(t.Context(), corpus.showsID, approws.MaxItemLimit)
	if err != nil {
		t.Fatalf("LibraryRows(shows): %v", err)
	}

	home, err := service.Home(t.Context(), approws.MaxItemLimit)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}

	assertFilledRecentlyAdded(t, "movies", movies.Rows)
	assertFilledRecentlyAdded(t, "shows", shows.Rows)
	assertGroupedEpisodeCards(t, shows.Rows[0])

	if len(home.Rows) != 2 || home.Hint != rows.HintNone {
		t.Errorf("Home has %d rows and hint %q, want one per library and none", len(home.Rows), home.Hint)
	}

	for _, rowList := range [][]approws.Row{movies.Rows, shows.Rows, home.Rows} {
		assertNoneOf(t, rowList, unavailable)
	}
}

// standInFolders writes the unavailable stand-ins into two temporary library folders.
func standInFolders(t *testing.T, root string) (brokenFolder, offlineFolder string) {
	t.Helper()

	brokenFolder, offlineFolder = t.TempDir(), t.TempDir()

	clip, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(offlineSource))) //nolint:gosec // fixed corpus path
	if err != nil {
		t.Fatalf("read the corpus clip: %v", err)
	}

	writeStandIn(t, filepath.Join(brokenFolder, filepath.FromSlash(unprobeableMovie)), []byte("not a video"))
	writeStandIn(t, filepath.Join(offlineFolder, filepath.FromSlash(offlineMovie)), clip)

	return brokenFolder, offlineFolder
}

func writeStandIn(t *testing.T, path string, content []byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), standInDirMode); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, content, standInFileMode); err != nil { //nolint:gosec // a fixed name in the test's temporary directory
		t.Fatalf("write %s: %v", path, err)
	}
}

// Modes of the stand-in files in the test's temporary directories.
const (
	standInDirMode  = 0o750
	standInFileMode = 0o600
)

func scanRowsCorpus(t *testing.T, readers musicReaders, root string, extraMovieFolders ...string) rowsCorpus {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(t.Output(), &slog.HandlerOptions{Level: slog.LevelWarn}))
	corpus := rowsCorpus{database: openRowsDatabase(t, logger)}
	corpus.store = sqlite.NewManagedLibraryStore(corpus.database, time.Now, uuidgen.New().NewID)

	files, err := libraryfs.New(logger)
	if err != nil {
		t.Fatalf("libraryfs.New: %v", err)
	}

	corpus.files = files
	movieFolders := append([]string{filepath.Join(root, "movies"), filepath.Join(root, "discs")}, extraMovieFolders...)
	corpus.moviesID = corpus.createLibrary(t, library.KindMovies, movieFolders...)
	corpus.showsID = corpus.createLibrary(t, library.KindShows, filepath.Join(root, "shows"))

	corpus.scans, err = scan.NewService(scan.Dependencies{
		Libraries: corpus.store, Walker: files, Resolver: files, Prober: readers.prober,
		CatalogV2: sqlite.NewLibraryCatalog(corpus.database, time.Now), IDs: uuidgen.New(), Clock: time.Now, Logger: logger,
	})
	if err != nil {
		t.Fatalf("scan.NewService: %v", err)
	}

	t.Cleanup(func() { closeWithTimeout(t, "scan service", corpus.scans.Close) })

	for _, libraryID := range []string{corpus.moviesID, corpus.showsID} {
		if _, err := corpus.scans.Start(t.Context(), libraryID); err != nil {
			t.Fatalf("start the scan: %v", err)
		}

		if status := waitForScan(t, corpus.scans, libraryID); status.State != scan.StateIdle {
			t.Fatalf("scan of %s ended with state %s, code %q", libraryID, status.State, status.ErrorCode)
		}
	}

	return corpus
}

func openRowsDatabase(t *testing.T, logger *slog.Logger) *sqlite.DB {
	t.Helper()

	database, err := sqlite.Open(t.Context(), sqlite.Options{Path: filepath.Join(t.TempDir(), databaseFileName), Logger: logger})
	if err != nil {
		t.Fatalf("open the database: %v", err)
	}

	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close the database: %v", err)
		}
	})

	if err := database.Migrate(t.Context()); err != nil {
		t.Fatalf("migrate the database: %v", err)
	}

	return database
}

func (corpus rowsCorpus) createLibrary(t *testing.T, kind library.Kind, roots ...string) string {
	t.Helper()

	folders := make([]library.Folder, 0, len(roots))
	for _, root := range roots {
		folders = append(folders, library.Folder{Path: root})
	}

	created, err := corpus.store.CreateLibrary(t.Context(), library.ManagedLibrary{
		Name: string(kind), Kind: kind, Folders: folders, Options: library.DefaultOptions(),
	})
	if err != nil {
		t.Fatalf("create the %s library: %v", kind, err)
	}

	return created.ID
}

// services wires the item and rows use cases like cmd/mivo does.
func (corpus rowsCorpus) services(t *testing.T) (*applibrary.Service, *approws.Service) {
	t.Helper()

	reader := sqlite.NewCatalogReader(corpus.database)

	items, err := applibrary.NewService(applibrary.Dependencies{
		Libraries: corpus.store, Items: reader, Children: reader, Listings: reader, Roots: corpus.files,
	})
	if err != nil {
		t.Fatalf("applibrary.NewService: %v", err)
	}

	service, err := approws.NewService(approws.Dependencies{
		Libraries: approws.NewAllLibraries(corpus.store), Rows: reader, Summaries: reader,
		Catalog: items, Scans: corpus.scans, Clock: time.Now,
	})
	if err != nil {
		t.Fatalf("approws.NewService: %v", err)
	}

	return items, service
}

// unavailableItems returns the IDs of the movies the listing's quick filter "unavailable"
// finds.
func unavailableItems(t *testing.T, items *applibrary.Service, libraryID string) map[string]string {
	t.Helper()

	page, err := items.ListItems(t.Context(), libraryID, library.PageRequest{Limit: library.MaxPageLimit},
		library.ListingQuery{Filter: library.ListingFilter{Quick: library.QuickUnavailable}})
	if err != nil {
		t.Fatalf("list the unavailable movies: %v", err)
	}

	unavailable := make(map[string]string, len(page.Items))
	for _, summary := range page.Items {
		unavailable[summary.Item.ID] = summary.Item.Title
	}

	return unavailable
}

func assertFilledRecentlyAdded(t *testing.T, name string, rowList []approws.Row) {
	t.Helper()

	if len(rowList) == 0 || rowList[0].Kind != rows.KindRecentlyAdded || len(rowList[0].Items) == 0 {
		t.Fatalf("%s rows = %+v, want a filled recentlyAdded row first", name, rowList)
	}
}

// assertGroupedEpisodeCards checks that the shows of one scan become show or season cards
// that count their new episodes.
func assertGroupedEpisodeCards(t *testing.T, row approws.Row) {
	t.Helper()

	for _, card := range row.Items {
		kind := card.Summary.Item.Kind
		if (kind != library.ItemKindShow && kind != library.ItemKindSeason) || card.NewEpisodeCount < 1 {
			t.Errorf("recentlyAdded card %q is a %s with %d new episodes, want a show or season with at least one",
				card.Summary.Item.Title, kind, card.NewEpisodeCount)
		}
	}
}

func assertNoneOf(t *testing.T, rowList []approws.Row, unavailable map[string]string) {
	t.Helper()

	for _, row := range rowList {
		for _, item := range row.Items {
			if title, found := unavailable[item.Summary.Item.ID]; found {
				t.Errorf("row %s lists the unavailable item %q", row.ID, title)
			}
		}
	}
}
