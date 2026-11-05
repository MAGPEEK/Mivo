package scan_test

import (
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// partFile is part number of book "book" (match key key) at path with its attached pictures.
func partFile(path, key string, number int, pictures ...int) scan.RefreshFile {
	return scan.RefreshFile{
		State: inMoviesFolder(path, 1),
		Placement: library.Placement{
			Levels:     []library.Level{{Kind: library.ItemKindAudiobook, MatchKey: key, Title: "Book"}},
			PartNumber: library.NumberOf(number),
		},
		ItemIDs: []string{"book"}, EmbeddedPictures: pictures, PicturesKnown: true,
	}
}

func TestAudiobookCoverOrder(t *testing.T) {
	t.Parallel()

	// Path order differs from play order: the cover comes from part 1.
	folderBook := []scan.RefreshFile{partFile("Dune/a.mp3", "dune/dune", 2, 3), partFile("Dune/b.mp3", "dune/dune", 1, 1)}
	standAlone := []scan.RefreshFile{partFile("Shelf/Dune.m4b", "shelf/dune.m4b", 1, 2)}

	tests := []struct {
		name     string
		files    []scan.RefreshFile
		sidecars []library.FileState
		want     artwork.SourceKind
		wantPath string
	}{
		{name: "first part in play order", files: folderBook, want: artwork.SourceEmbedded, wantPath: "Dune/b.mp3"},
		{
			name: "folder cover beats embedded", files: folderBook,
			sidecars: []library.FileState{imageFile("Dune/cover.jpg", 1)},
			want:     artwork.SourceLocal, wantPath: "Dune/cover.jpg",
		},
		{
			name: "a book that stands alone ignores the shared folder cover", files: standAlone,
			sidecars: []library.FileState{imageFile("Shelf/cover.jpg", 1)},
			want:     artwork.SourceEmbedded, wantPath: "Shelf/Dune.m4b",
		},
		{
			name: "a book that stands alone takes images named after it", files: standAlone,
			sidecars: []library.FileState{imageFile("Shelf/cover.jpg", 1), imageFile("Shelf/Dune.jpg", 1)},
			want:     artwork.SourceLocal, wantPath: "Shelf/Dune.jpg",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			rig := newRefreshRig(t, "book")
			rig.mustRefresh(t, scan.RefreshRequest{ItemIDs: []string{"book"}, Files: test.files, Sidecars: test.sidecars})

			cover := rig.catalog.state("book").CurrentArtwork[artwork.KindPoster]
			if cover.Source != test.want || cover.Location.RelativePath != test.wantPath {
				t.Errorf("book cover = %+v, want %s at %s", cover, test.want, test.wantPath)
			}
		})
	}
}

func TestRefreshMergesTheEmbeddedRecordBeforeTheFileName(t *testing.T) {
	t.Parallel()

	rig := newRefreshRig(t, "book")
	people := []metadata.Credit{{Name: "Frank Herbert", Role: metadata.RoleAuthor}}

	rig.mustRefresh(t, scan.RefreshRequest{
		ItemIDs: []string{"book"}, Files: []scan.RefreshFile{partFile("Dune/a.mp3", "dune/dune", 1)},
		Embedded: map[string]metadata.Record{"book": {Title: "Dune", Year: 1965, People: people}},
	})

	record := rig.catalog.state("book").Record
	if record.Title != "Dune" || record.Year != 1965 || !slices.Equal(record.People, people) {
		t.Errorf("record = %+v, want the embedded title, year and author", record)
	}
}
