package sqlite_test

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

const absoluteEpisodeCount = 131 // DT-24

func TestSavePlacedFileKeepsIDsOnRescan(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	record := placedFile(showsLibraryID, testID('f', 1), "Show/S01E01.mkv", episodePlacement("show", 1, 1))

	first := fixture.save(t, record)
	counts := catalogRowCounts(t, fixture.inspection)
	second := fixture.save(t, record)

	assertDeepEqual(t, "chain", second, first)
	assertDeepEqual(t, "row counts", catalogRowCounts(t, fixture.inspection), counts)

	if len(first) != 3 || fixture.counter != 3 {
		t.Errorf("chain = %v after %d generated IDs, want 3 items", first, fixture.counter)
	}
}

func TestSavePlacedFileStoresItemData(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	placement := moviePlacement("the matrix|1999", "The Matrix", 1999, "")
	fixture.save(t, placedFile(moviesLibraryID, testID('f', 1), "The Matrix (1999).mkv", placement))

	var title, sortTitle string

	var year, created int64

	err := fixture.inspection.QueryRowContext(t.Context(),
		"SELECT title, sort_title, year, created_at FROM items").Scan(&title, &sortTitle, &year, &created)
	if err != nil {
		t.Fatal(err)
	}

	if title != "The Matrix" || sortTitle != "matrix" || year != 1999 || created != scannedMilli {
		t.Errorf("item = %q %q %d %d", title, sortTitle, year, created)
	}

	// A later title in the placement does not rewrite the stored item.
	placement.Levels[0].Title = "Matrix, The (retitled)"
	fixture.save(t, placedFile(moviesLibraryID, testID('f', 1), "The Matrix (1999).mkv", placement))

	err = fixture.inspection.QueryRowContext(t.Context(), "SELECT title FROM items").Scan(&title)
	if err != nil || title != "The Matrix" {
		t.Errorf("title = %q, %v; want unchanged", title, err)
	}
}

func TestSecondVersionJoinsExistingMovie(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	hd := fixture.save(t, placedFile(moviesLibraryID, testID('f', 1), "Heat (1995) - 1080p.mkv", moviePlacement("heat|1995", "Heat", 1995, "1080p")))
	uhd := fixture.save(t, placedFile(moviesLibraryID, testID('f', 2), "Heat (1995) - 4K.mkv", moviePlacement("heat|1995", "Heat", 1995, "4K")))

	assertDeepEqual(t, "chain", uhd, hd)

	if rowCount(t, fixture.inspection, "media_files") != 2 || itemCount(t, fixture, "movie") != 1 {
		t.Errorf("want one movie with two files")
	}
}

func TestRenamedEpisodeFileKeepsEpisode(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	placement := episodePlacement("show", 1, 1)
	before := fixture.save(t, placedFile(showsLibraryID, testID('f', 1), "Show/old name.mkv", placement))
	after := fixture.save(t, placedFile(showsLibraryID, testID('f', 1), "Show/new name.mkv", placement))

	assertDeepEqual(t, "chain", after, before)

	if rowCount(t, fixture.inspection, "media_files") != 1 || itemCount(t, fixture, "episode") != 1 {
		t.Errorf("want one file and one episode")
	}
}

func TestChangedPlacementMovesFileAndRemovesOldShow(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	fileID := testID('f', 1)
	old := fixture.save(t, placedFile(showsLibraryID, fileID, "Old/S01E01.mkv", episodePlacement("old", 1, 1)))
	moved := fixture.save(t, placedFile(showsLibraryID, fileID, "New/S01E01.mkv", episodePlacement("new", 1, 1)))

	if old[0] == moved[0] {
		t.Fatalf("show kept its ID %q", old[0])
	}

	if itemCount(t, fixture, "show") != 1 || rowCount(t, fixture.inspection, "items") != 3 {
		t.Errorf("old show still stored: %d items", rowCount(t, fixture.inspection, "items"))
	}
}

func TestMoveFileKeepsStreamsAndPrunesFormerChain(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	record := placedFile(showsLibraryID, testID('f', 1), "Old/S01E01.mkv", episodePlacement("old", 1, 1))
	fixture.save(t, record)

	chain, err := fixture.catalog.MoveFile(t.Context(), scan.MoveRequest{
		LibraryID: showsLibraryID, FileID: record.File.ID, Placement: episodePlacement("new", 2, 3), NewID: fixture.newID,
	})
	if err != nil {
		t.Fatalf("MoveFile error = %v", err)
	}

	states, err := fixture.catalog.PlacedFiles(t.Context(), showsLibraryID)
	if err != nil || len(states) != 1 {
		t.Fatalf("PlacedFiles = %v, %v", states, err)
	}

	assertDeepEqual(t, "item ids", states[0].ItemIDs, chain)
	assertDeepEqual(t, "placement key", states[0].PlacementKey, episodePlacement("new", 2, 3).Key())

	if got := streamRowCount(t, fixture.inspection, record.File.ID); got != streamsOf(record.File) {
		t.Errorf("streams = %d, want %d", got, streamsOf(record.File))
	}

	if rowCount(t, fixture.inspection, "items") != 3 {
		t.Errorf("items = %d, want only the new chain", rowCount(t, fixture.inspection, "items"))
	}
}

func TestMoveFileErrors(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	record := placedFile(moviesLibraryID, testID('f', 1), "A (2000).mkv", moviePlacement("a|2000", "A", 2000, ""))
	fixture.save(t, record)

	tests := []struct {
		name    string
		request scan.MoveRequest
		want    error
	}{
		{"unknown file", scan.MoveRequest{LibraryID: moviesLibraryID, FileID: unknownID, Placement: record.Placement}, domain.ErrNotFound},
		{"unknown library", scan.MoveRequest{LibraryID: unknownID, FileID: record.File.ID, Placement: record.Placement}, domain.ErrNotFound},
		{"file of another library", scan.MoveRequest{LibraryID: showsLibraryID, FileID: record.File.ID, Placement: record.Placement}, domain.ErrNotFound},
		{"invalid placement", scan.MoveRequest{LibraryID: moviesLibraryID, FileID: record.File.ID}, domain.ErrInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.request.NewID = fixture.newID

			if _, err := fixture.catalog.MoveFile(t.Context(), test.request); !errors.Is(err, test.want) {
				t.Errorf("MoveFile error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestAbsoluteNumberedEpisodesShareOneShowAndSeason(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)

	for episode := 1; episode <= absoluteEpisodeCount; episode++ {
		path := fmt.Sprintf("Anime/Anime - %03d.mkv", episode)
		fixture.save(t, placedFile(showsLibraryID, testID('f', episode), path, episodePlacement("anime", 1, episode)))
	}

	for kind, want := range map[string]int{"show": 1, "season": 1, "episode": absoluteEpisodeCount} {
		if got := itemCount(t, fixture, kind); got != want {
			t.Errorf("%s items = %d, want %d", kind, got, want)
		}
	}
}

func TestSavePlacedFileErrors(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	first := placedFile(moviesLibraryID, testID('f', 1), "A (2000).mkv", moviePlacement("a|2000", "A", 2000, ""))
	fixture.save(t, first)

	zeroProbe := placedFile(moviesLibraryID, testID('f', 2), "B.mkv", moviePlacement("b", "B", 0, ""))
	zeroProbe.ProbedAt = time.Time{}

	tests := []struct {
		name   string
		record scan.PlacedFile
		want   error
	}{
		{"unknown library", placedFile(unknownID, testID('f', 2), "B.mkv", moviePlacement("b", "B", 0, "")), domain.ErrNotFound},
		{"conflicting path", placedFile(moviesLibraryID, testID('f', 3), "A (2000).mkv", moviePlacement("c", "C", 0, "")), domain.ErrConflict},
		{"invalid placement", placedFile(moviesLibraryID, testID('f', 4), "D.mkv", library.Placement{}), domain.ErrInvalid},
		{"zero probe time", zeroProbe, domain.ErrInvalid},
		{"file id of another library", placedFile(showsLibraryID, first.File.ID, "A.mkv", episodePlacement("a", 1, 1)), domain.ErrInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := catalogRowCounts(t, fixture.inspection)

			if _, err := fixture.catalog.SavePlacedFile(t.Context(), test.record, fixture.newID); !errors.Is(err, test.want) {
				t.Fatalf("SavePlacedFile error = %v, want %v", err, test.want)
			}

			assertDeepEqual(t, "row counts", catalogRowCounts(t, fixture.inspection), before)
		})
	}
}

func TestRemovePlacedFilesPrunesBottomUpAndIsIdempotent(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	one := placedFile(showsLibraryID, testID('f', 1), "S/e1.mkv", episodePlacement("s", 1, 1))
	two := placedFile(showsLibraryID, testID('f', 2), "S/e2.mkv", episodePlacement("s", 1, 2))
	fixture.save(t, one)
	fixture.save(t, two)

	remove := func(ids ...string) {
		t.Helper()

		if err := fixture.catalog.RemovePlacedFiles(t.Context(), showsLibraryID, ids); err != nil {
			t.Fatalf("RemovePlacedFiles error = %v", err)
		}
	}

	remove(one.File.ID, unknownID)

	if rowCount(t, fixture.inspection, "items") != 3 {
		t.Errorf("items = %d, want show, season and one episode", rowCount(t, fixture.inspection, "items"))
	}

	if err := fixture.catalog.RemovePlacedFiles(t.Context(), moviesLibraryID, []string{two.File.ID}); err != nil {
		t.Fatalf("other library: %v", err)
	}

	remove(two.File.ID, two.File.ID)
	remove(two.File.ID)
	remove()

	if rowCount(t, fixture.inspection, "items") != 0 || rowCount(t, fixture.inspection, "media_files") != 0 {
		t.Errorf("catalog not empty: %v", catalogRowCounts(t, fixture.inspection))
	}
}

func TestRemovingMovieFileTakesItsExtras(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	movie := moviePlacement("m|2000", "M", 2000, "")
	extra := library.Placement{Levels: []library.Level{
		movie.Levels[0],
		{Kind: library.ItemKindExtra, MatchKey: "trailer", Title: "Trailer", ExtraType: library.ExtraTypeTrailer},
	}}
	main := placedFile(moviesLibraryID, testID('f', 1), "M (2000)/M.mkv", movie)
	fixture.save(t, main)
	fixture.save(t, placedFile(moviesLibraryID, testID('f', 2), "M (2000)/trailers/T.mkv", extra))

	if itemCount(t, fixture, "extra") != 1 {
		t.Fatal("extra not stored")
	}

	if err := fixture.catalog.RemovePlacedFiles(t.Context(), moviesLibraryID, []string{main.File.ID}); err != nil {
		t.Fatal(err)
	}

	if rowCount(t, fixture.inspection, "items") != 0 || rowCount(t, fixture.inspection, "media_files") != 0 {
		t.Errorf("movie or extra left: %v", catalogRowCounts(t, fixture.inspection))
	}
}

func TestRemovingExtraKeepsMovie(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)
	movie := moviePlacement("m|2000", "M", 2000, "")
	extra := library.Placement{Levels: []library.Level{
		movie.Levels[0],
		{Kind: library.ItemKindExtra, MatchKey: "trailer", Title: "Trailer", ExtraType: library.ExtraTypeTrailer},
	}}
	fixture.save(t, placedFile(moviesLibraryID, testID('f', 1), "M.mkv", movie))
	trailer := placedFile(moviesLibraryID, testID('f', 2), "T.mkv", extra)
	fixture.save(t, trailer)

	if err := fixture.catalog.RemovePlacedFiles(t.Context(), moviesLibraryID, []string{trailer.File.ID}); err != nil {
		t.Fatal(err)
	}

	if itemCount(t, fixture, "movie") != 1 || itemCount(t, fixture, "extra") != 0 {
		t.Errorf("want the movie only: %v", catalogRowCounts(t, fixture.inspection))
	}
}

func TestPlacedFilesReturnsKeysAndChains(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)

	part := moviePlacement("m|2000", "M", 2000, "Director's Cut")
	part.PartNumber = library.NumberOf(2)
	record := placedFile(moviesLibraryID, testID('f', 1), "M.mkv", part)
	chain := fixture.save(t, record)

	states, err := fixture.catalog.PlacedFiles(t.Context(), moviesLibraryID)
	if err != nil || len(states) != 1 {
		t.Fatalf("PlacedFiles = %v, %v", states, err)
	}

	got := states[0]
	if got.ID != record.File.ID || got.PlacementKey != part.Key() || !slices.Equal(got.ItemIDs, chain) {
		t.Errorf("state = %+v, want key %q chain %v", got, part.Key(), chain)
	}

	if got.State.RelativePath != "M.mkv" || got.State.Class != library.FileClassVideo ||
		got.State.Size != record.File.Size || got.State.ModifiedAt.UnixMilli() != fileModifiedMilli {
		t.Errorf("file state = %+v", got.State)
	}

	if other, err := fixture.catalog.PlacedFiles(t.Context(), unknownID); err != nil || len(other) != 0 {
		t.Errorf("unknown library = %v, %v; want none", other, err)
	}
}

func TestSavePlacedFileWithReaderConcurrently(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)

	var group sync.WaitGroup

	group.Add(1)

	go func() {
		defer group.Done()

		for range 20 {
			if _, err := fixture.catalog.PlacedFiles(t.Context(), showsLibraryID); err != nil {
				t.Errorf("PlacedFiles error = %v", err)
			}
		}
	}()

	for episode := 1; episode <= 20; episode++ {
		record := placedFile(showsLibraryID, testID('f', episode), fmt.Sprintf("S/e%d.mkv", episode), episodePlacement("s", 1, episode))
		if _, err := fixture.catalog.SavePlacedFile(t.Context(), record, fixture.newID); err != nil {
			t.Fatalf("SavePlacedFile error = %v", err)
		}
	}

	group.Wait()
}
