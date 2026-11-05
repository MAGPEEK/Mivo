package naming_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/naming"
)

func TestNewAudiobookFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tags media.AudioTags
		want naming.AudiobookFile
	}{
		{name: "no tags", want: naming.AudiobookFile{Path: "a.mp3"}},
		{
			name: "album artist is the author",
			tags: media.AudioTags{AlbumArtist: " Herbert ", Artist: "Reader", Album: "Dune", Disc: 1, Track: 2},
			want: naming.AudiobookFile{Path: "a.mp3", Author: "Herbert", Album: "Dune", Disc: 1, Track: 2},
		},
		{
			name: "artist when no album artist, narrator fields copied",
			tags: media.AudioTags{Artist: "Herbert", Composer: "Scott", Narrator: "Simon", ComposerIsNarrator: true},
			want: naming.AudiobookFile{Path: "a.mp3", Author: "Herbert", Composer: "Scott", Narrator: "Simon", ComposerIsNarrator: true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := naming.NewAudiobookFile("a.mp3", test.tags); got != test.want {
				t.Errorf("NewAudiobookFile() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestAudiobookStandsAlone(t *testing.T) {
	t.Parallel()

	for relativePath, want := range map[string]bool{
		"book.mp3": true, "Shelf/book.m4b": true, "Shelf/book.M4B": true, "Shelf/book.mp3": false, "Shelf/book.m4a": false,
	} {
		if got := naming.AudiobookStandsAlone(relativePath); got != want {
			t.Errorf("AudiobookStandsAlone(%q) = %t, want %t", relativePath, got, want)
		}
	}
}

func TestAudiobookPlacement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		files     []naming.AudiobookFile
		wantKey   string
		wantTitle string
		wantParts int
	}{
		{
			name:      "folder book keys on folder and title",
			files:     []naming.AudiobookFile{{Path: "Herbert/Dune/02.mp3", Track: 2}, {Path: "Herbert/Dune/01.mp3", Track: 1, Album: "Dune "}},
			wantKey:   "folder:herbert/dune/dune",
			wantTitle: "Dune",
			wantParts: 2,
		},
		{
			name:      "M4B in a shared folder keys on its path",
			files:     []naming.AudiobookFile{{Path: "Shelf/Dune.m4b", Album: "Dune"}},
			wantKey:   "file:shelf/dune.m4b",
			wantTitle: "Dune",
			wantParts: 1,
		},
		{
			name:      "root file without a stem falls back to its name",
			files:     []naming.AudiobookFile{{Path: ".mp3"}},
			wantKey:   "file:.mp3",
			wantTitle: ".mp3",
			wantParts: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			books := naming.GroupAudiobooks(test.files)
			if len(books) != 1 || len(books[0].Parts) != test.wantParts {
				t.Fatalf("GroupAudiobooks() = %+v, want one book with %d parts", books, test.wantParts)
			}

			first := books[0].PartPlacement(0).Levels[0]
			if first.MatchKey != test.wantKey || first.Title != test.wantTitle {
				t.Errorf("level = %+v, want key %q title %q", first, test.wantKey, test.wantTitle)
			}

			for index := range books[0].Parts {
				assertPartPlacement(t, books[0], index, first)
			}
		})
	}
}

func assertPartPlacement(t *testing.T, book naming.Audiobook, index int, want library.Level) {
	t.Helper()

	placement := book.PartPlacement(index)
	if err := placement.Validate(); err != nil {
		t.Fatalf("PartPlacement(%d) invalid: %v", index, err)
	}

	level := placement.Levels[0]
	if number, _ := placement.PartNumber.Get(); level != want ||
		level.Kind != library.ItemKindAudiobook || number != index+1 {
		t.Errorf("PartPlacement(%d) = %+v, want level %+v part %d", index, placement, want, index+1)
	}
}

func TestAudiobookAuthorIsTheFirstTaggedPart(t *testing.T) {
	t.Parallel()

	books := naming.GroupAudiobooks([]naming.AudiobookFile{
		{Path: "Dune/01.mp3", Track: 1},
		{Path: "Dune/02.mp3", Track: 2, Author: " Frank Herbert "},
	})

	if len(books) != 1 || books[0].Author != "Frank Herbert" {
		t.Errorf("GroupAudiobooks() = %+v, want author Frank Herbert", books)
	}
}
