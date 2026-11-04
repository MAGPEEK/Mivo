package sqlite_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
)

const (
	secondMoviesRootID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	secondMoviesRoot   = "/media/movies-second"
)

// newTwoFolderFixture is a placement fixture whose movies library has a second folder at
// position 1.
func newTwoFolderFixture(t *testing.T) *placementFixture {
	t.Helper()

	fixture := newPlacementFixture(t)
	execute(t, fixture.inspection, `INSERT INTO library_roots (id, library_id, path, path_key, position, created_at)
		VALUES (?1, ?2, ?3, ?3, 1, 0)`, secondMoviesRootID, moviesLibraryID, secondMoviesRoot)

	return fixture
}

// placedRecord is the input of savePlaced: one version of the movie Heat in a folder.
type placedRecord struct{ rootID, fileID, path string }

func (fixture *placementFixture) savePlaced(t *testing.T, record placedRecord) {
	t.Helper()

	placed := placedFile(moviesLibraryID, record.fileID, record.path, moviePlacement("heat1995", "Heat", 1995, ""))
	placed.RootID = record.rootID
	fixture.save(t, placed)
}

func (fixture *placementFixture) onlyItemID(t *testing.T) string {
	t.Helper()

	var itemID string
	if err := fixture.inspection.QueryRowContext(t.Context(), `SELECT id FROM items`).Scan(&itemID); err != nil {
		t.Fatal(err)
	}

	return itemID
}

func TestSameRelativePathInTwoFoldersIsTwoFiles(t *testing.T) {
	t.Parallel()

	fixture := newTwoFolderFixture(t)
	fixture.savePlaced(t, placedRecord{secondMoviesRootID, testID('f', 2), "Heat (1995)/Heat.mkv"})
	fixture.savePlaced(t, placedRecord{moviesLibraryID, testID('f', 1), "Heat (1995)/Heat.mkv"})

	states, err := fixture.catalog.PlacedFiles(t.Context(), moviesLibraryID)
	if err != nil {
		t.Fatal(err)
	}

	if len(states) != 2 {
		t.Fatalf("placed files = %d, want 2", len(states))
	}

	roots := map[string]string{states[0].State.RootID: states[0].ID, states[1].State.RootID: states[1].ID}
	if roots[moviesLibraryID] != testID('f', 1) || roots[secondMoviesRootID] != testID('f', 2) {
		t.Errorf("file by root id = %v, want each file in its own folder", roots)
	}

	if items := countRows(t, fixture.inspection, `SELECT count(*) FROM items`); items != 1 {
		t.Errorf("items = %d, want 1 (two versions of one movie)", items)
	}

	if err := fixture.catalog.RemovePlacedFiles(t.Context(), moviesLibraryID, []string{testID('f', 1)}); err != nil {
		t.Fatal(err)
	}

	left := countRows(t, fixture.inspection, `SELECT count(*) FROM media_files WHERE root_id = ?`, secondMoviesRootID)
	if left != 1 {
		t.Errorf("files of the second folder = %d, want 1", left)
	}
}

func TestFoldersOrderThePrimaryFileAndResolveTheirPaths(t *testing.T) {
	t.Parallel()

	fixture := newTwoFolderFixture(t)
	// "A" sorts before "Z", but the first folder wins (ADR 0027 section 5).
	fixture.savePlaced(t, placedRecord{moviesLibraryID, testID('f', 1), "Z/Heat.mkv"})
	fixture.savePlaced(t, placedRecord{secondMoviesRootID, testID('f', 2), "A/Heat.mkv"})

	detail, err := fixture.reader.GetItem(t.Context(), fixture.onlyItemID(t))
	if err != nil {
		t.Fatal(err)
	}

	if len(detail.Files) != 2 || detail.Files[0].ID != testID('f', 1) {
		t.Errorf("files = %v, want the first folder's file first", detail.Files)
	}

	for fileID, wantRoot := range map[string]string{testID('f', 1): moviesRoot, testID('f', 2): secondMoviesRoot} {
		playable, err := fixture.reader.PlayableFile(t.Context(), fileID)
		if err != nil || playable.LibraryRoot != wantRoot {
			t.Errorf("PlayableFile(%s) root = %q, %v; want %q", fileID, playable.LibraryRoot, err, wantRoot)
		}
	}
}

func TestWritersRefuseAFolderOfAnotherLibrary(t *testing.T) {
	t.Parallel()

	fixture := newTwoFolderFixture(t)

	for _, rootID := range []string{"", showsLibraryID, unknownID} {
		record := placedFile(moviesLibraryID, testID('f', 1), "Heat.mkv", moviePlacement("heat", "Heat", 1995, ""))
		record.RootID = rootID

		if _, err := fixture.catalog.SavePlacedFile(t.Context(), record, fixture.newID); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("SavePlacedFile root %q error = %v, want ErrInvalid", rootID, err)
		}

		nfo := sidecar("Heat.nfo", library.FileClassNFO, 1)
		nfo.RootID = rootID

		err := fixture.catalog.SaveSidecarStates(t.Context(), moviesLibraryID, []library.FileState{nfo}, nil)
		if !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("SaveSidecarStates root %q error = %v, want ErrInvalid", rootID, err)
		}
	}

	rows := countRows(t, fixture.inspection,
		`SELECT (SELECT count(*) FROM media_files) + (SELECT count(*) FROM library_sidecars)`)
	if rows != 0 {
		t.Errorf("rows after refused writes = %d, want 0", rows)
	}
}

func TestSidecarsAreStoredAndRemovedPerFolder(t *testing.T) {
	t.Parallel()

	fixture := newTwoFolderFixture(t)
	first := sidecar("Heat/movie.nfo", library.FileClassNFO, 1)
	second := first
	second.RootID = secondMoviesRootID

	if err := fixture.catalog.SaveSidecarStates(t.Context(), moviesLibraryID, []library.FileState{first, second}, nil); err != nil {
		t.Fatal(err)
	}

	if err := fixture.catalog.SaveSidecarStates(t.Context(), moviesLibraryID, nil, []library.FileState{first}); err != nil {
		t.Fatal(err)
	}

	got, err := fixture.catalog.SidecarStates(t.Context(), moviesLibraryID)
	if err != nil {
		t.Fatal(err)
	}

	assertDeepEqual(t, "sidecars", got, []library.FileState{second})
}

func TestArtworkIsFoundPerFolder(t *testing.T) {
	t.Parallel()

	fixture := newTwoFolderFixture(t)
	fixture.savePlaced(t, placedRecord{moviesLibraryID, testID('f', 1), "Heat/Heat.mkv"})
	fixture.savePlaced(t, placedRecord{secondMoviesRootID, testID('f', 2), "Heat/Heat.mkv"})

	itemID := fixture.onlyItemID(t)
	local := localCandidate(artwork.KindPoster, "Heat/poster.jpg", 1)
	local.Location.RootID = secondMoviesRootID
	embedded := embeddedCandidate(artwork.KindThumb, "Heat/Heat.mkv", 1)
	embedded.Location.RootID = secondMoviesRootID

	err := fixture.catalog.SaveArtwork(t.Context(), itemID, []artwork.Candidate{local, embedded},
		map[artwork.Kind]artwork.Candidate{artwork.KindPoster: local, artwork.KindThumb: embedded})
	if err != nil {
		t.Fatal(err)
	}

	rows := countRows(t, fixture.inspection, `SELECT count(*) FROM artwork WHERE media_file_id = ?`, testID('f', 2))
	if rows != 1 {
		t.Errorf("embedded rows of the second folder's file = %d, want 1", rows)
	}

	state, err := fixture.catalog.ItemMetadataState(t.Context(), itemID)
	if err != nil {
		t.Fatal(err)
	}

	assertDeepEqual(t, "selected poster", state.CurrentArtwork[artwork.KindPoster].Location, local.Location)
	assertDeepEqual(t, "selected thumb", state.CurrentArtwork[artwork.KindThumb].Location, embedded.Location)

	var stored string
	if err := fixture.inspection.QueryRowContext(t.Context(), `SELECT id FROM artwork WHERE source = 'local'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}

	got, err := fixture.reader.Artwork(t.Context(), stored)
	if err != nil || got.LibraryRoot != secondMoviesRoot {
		t.Errorf("Artwork root = %q, %v; want %q", got.LibraryRoot, err, secondMoviesRoot)
	}

	foreign := localCandidate(artwork.KindBackdrop, "Heat/fanart.jpg", 1)
	foreign.Location.RootID = showsLibraryID

	if err := fixture.catalog.SaveArtwork(t.Context(), itemID, []artwork.Candidate{foreign}, nil); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("SaveArtwork foreign folder error = %v, want ErrInvalid", err)
	}
}
