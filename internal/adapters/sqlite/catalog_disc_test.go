package sqlite_test

import (
	"testing"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/media"
)

// Discs keep their layout through the placement writer and the readers (ADR 0029); a single
// file is stored as 'file' and read back as the zero layout.
func TestSavePlacedFileStoresTheLayout(t *testing.T) {
	t.Parallel()

	fixture := newPlacementFixture(t)

	disc := placedFile(moviesLibraryID, testID('f', 1), "Heat (1995)/BDMV", moviePlacement("heat|1995", "Heat", 1995, ""))
	disc.File.Layout = media.LayoutBDMV
	disc.File.Description = media.Description{}
	disc.File.ProbeError = media.ProbeErrorDiscEncrypted

	image := placedFile(moviesLibraryID, testID('f', 2), "Alien (1979).iso", moviePlacement("alien|1979", "Alien", 1979, ""))
	image.File.Layout = media.LayoutISO

	single := placedFile(moviesLibraryID, testID('f', 3), "Up (2009).mkv", moviePlacement("up|2009", "Up", 2009, ""))

	fixture.save(t, disc)
	fixture.save(t, image)
	fixture.save(t, single)

	assertPlacedDiscStates(t, fixture, disc.File, image.File, single.File)

	for _, test := range []struct {
		file       media.File
		wantColumn string
	}{
		{file: disc.File, wantColumn: "bdmv"},
		{file: image.File, wantColumn: "iso"},
		{file: single.File, wantColumn: "file"},
	} {
		var column string
		if err := fixture.inspection.QueryRowContext(t.Context(),
			"SELECT layout FROM media_files WHERE id = ?", test.file.ID).Scan(&column); err != nil {
			t.Fatal(err)
		}

		if column != test.wantColumn {
			t.Errorf("%s: layout column = %q, want %q", test.file.RelativePath, column, test.wantColumn)
		}

		playable, err := fixture.reader.PlayableFile(t.Context(), test.file.ID, access.Unrestricted())
		if err != nil {
			t.Fatalf("PlayableFile(%s) error = %v", test.file.RelativePath, err)
		}

		want := test.file
		want.ItemID = playable.File.ItemID

		assertDeepEqual(t, "PlayableFile", playable, appplayback.PlayableFile{File: want, LibraryRoot: moviesRoot})
	}
}

// assertPlacedDiscStates checks that PlacedFiles reports the layout and probe error the scan
// needs to probe a disc_unsupported disc again; files is every saved file.
func assertPlacedDiscStates(t *testing.T, fixture *placementFixture, files ...media.File) {
	t.Helper()

	states, err := fixture.catalog.PlacedFiles(t.Context(), moviesLibraryID)
	if err != nil {
		t.Fatalf("PlacedFiles error = %v", err)
	}

	want := map[string]media.File{}
	for _, file := range files {
		want[file.ID] = file
	}

	for _, state := range states {
		file := want[state.ID]
		if state.State.Layout != file.Layout || state.ProbeError != file.ProbeError {
			t.Errorf("%s: layout %q, probe error %q, want %q, %q",
				state.State.RelativePath, state.State.Layout, state.ProbeError, file.Layout, file.ProbeError)
		}
	}
}
