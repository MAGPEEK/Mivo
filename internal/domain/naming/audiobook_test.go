package naming_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"

	"github.com/magpeek/mivo/internal/domain/naming"
)

func paths(book naming.Audiobook) []string {
	out := make([]string, 0, len(book.Parts))
	for _, part := range book.Parts {
		out = append(out, part.Path)
	}

	return out
}

func TestIsAudiobookFileName(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]bool{
		"a/b.MP3": true, "x.m4b": true, "x.flac": true, "x.mkv": false, "x": false, "x.nfo": false,
	} {
		if got := naming.IsAudiobookFileName(name); got != want {
			t.Errorf("IsAudiobookFileName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestGroupAudiobooks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		files      []naming.AudiobookFile
		wantTitles []string
		wantFolder []string
		wantParts  [][]string
		narrator   []string
	}{
		{
			name: "folder is one book titled by album, ordered by disc and track",
			files: []naming.AudiobookFile{
				{Path: "Dune/b.mp3", Album: "Dune", Disc: 2, Track: 1},
				{Path: "Dune/a.mp3", Album: "Dune", Disc: 1, Track: 2},
				{Path: "Dune/c.mp3", Album: "Dune", Disc: 1, Track: 1},
				{Path: "Dune/cover.jpg"},
			},
			wantTitles: []string{"Dune"},
			wantFolder: []string{"Dune"},
			wantParts:  [][]string{{"Dune/c.mp3", "Dune/a.mp3", "Dune/b.mp3"}},
			narrator:   []string{""},
		},
		{
			name: "no tags: folder name, natural order",
			files: []naming.AudiobookFile{
				{Path: "Book/Part 10.mp3"}, {Path: "Book/part 2.mp3"}, {Path: "Book/Part 1.mp3"},
			},
			wantTitles: []string{"Book"},
			wantFolder: []string{"Book"},
			wantParts:  [][]string{{"Book/Part 1.mp3", "Book/part 2.mp3", "Book/Part 10.mp3"}},
			narrator:   []string{""},
		},
		{
			name: "missing track on one part falls back to names",
			files: []naming.AudiobookFile{
				{Path: "B/2.mp3", Track: 1}, {Path: "B/1.mp3"},
			},
			wantTitles: []string{"B"},
			wantFolder: []string{"B"},
			wantParts:  [][]string{{"B/1.mp3", "B/2.mp3"}},
			narrator:   []string{""},
		},
		{
			name: "files in the root and m4b in a shared folder are own books",
			files: []naming.AudiobookFile{
				{Path: "One.mp3"},
				{Path: "Two.m4a", Album: "Second"},
				{Path: "Shelf/X.M4B"},
				{Path: "Shelf/Y.m4b", Album: "Why"},
			},
			wantTitles: []string{"One", "Second", "Why", "X"},
			wantFolder: []string{"", "", "Shelf", "Shelf"},
			wantParts:  [][]string{{"One.mp3"}, {"Two.m4a"}, {"Shelf/Y.m4b"}, {"Shelf/X.M4B"}},
			narrator:   []string{"", "", "", ""},
		},
		{
			name: "different albums in one folder split",
			files: []naming.AudiobookFile{
				{Path: "S/a.mp3", Album: "Alpha"}, {Path: "S/b.mp3", Album: " alpha "}, {Path: "S/c.mp3", Album: "Beta"},
			},
			wantTitles: []string{"Alpha", "Beta"},
			wantFolder: []string{"S", "S"},
			wantParts:  [][]string{{"S/a.mp3", "S/b.mp3"}, {"S/c.mp3"}},
			narrator:   []string{"", ""},
		},
		{
			name: "narrator tag wins, composer only when it says so",
			files: []naming.AudiobookFile{
				{Path: "N/1.mp3", Composer: "Composer", Narrator: "Reader"},
				{Path: "C/1.mp3", Composer: "Composer"},
				{Path: "D/1.mp3", Composer: "Voice", ComposerIsNarrator: true},
				{Path: "E/1.mp3", Composer: " ", ComposerIsNarrator: true},
			},
			wantTitles: []string{"C", "D", "E", "N"},
			wantFolder: []string{"C", "D", "E", "N"},
			wantParts:  [][]string{{"C/1.mp3"}, {"D/1.mp3"}, {"E/1.mp3"}, {"N/1.mp3"}},
			narrator:   []string{"", "Voice", "", "Reader"},
		},
		{
			name: "nested folder title and large numbers",
			files: []naming.AudiobookFile{
				{Path: "A/B/12345678901234567890.mp3"}, {Path: "A/B/12345678901234567891.mp3"},
			},
			wantTitles: []string{"B"},
			wantFolder: []string{"A/B"},
			wantParts:  [][]string{{"A/B/12345678901234567890.mp3", "A/B/12345678901234567891.mp3"}},
			narrator:   []string{""},
		},
		{
			name: "untagged parts join the tagged book of their folder",
			files: []naming.AudiobookFile{
				{Path: "F/1.mp3", Album: "Tagged"}, {Path: "F/2.mp3"}, {Path: "F/3.mp3", Album: "Tagged"},
			},
			wantTitles: []string{"Tagged"},
			wantFolder: []string{"F"},
			wantParts:  [][]string{{"F/1.mp3", "F/2.mp3", "F/3.mp3"}},
			narrator:   []string{""},
		},
		{
			name: "untagged parts follow the first of several albums",
			files: []naming.AudiobookFile{
				{Path: "G/a.mp3", Album: "Zed"}, {Path: "G/b.mp3", Album: "Yes"}, {Path: "G/c.mp3"},
			},
			wantTitles: []string{"Yes", "Zed"},
			wantFolder: []string{"G", "G"},
			wantParts:  [][]string{{"G/b.mp3"}, {"G/a.mp3", "G/c.mp3"}},
			narrator:   []string{"", ""},
		},
		{
			name: "loose untagged m4a next to m4b files is not merged into them",
			files: []naming.AudiobookFile{
				{Path: "H/one.m4b", Album: "One"}, {Path: "H/two.m4b", Album: "Two"}, {Path: "H/loose.m4a"},
			},
			wantTitles: []string{"H", "One", "Two"},
			wantFolder: []string{"H", "H", "H"},
			wantParts:  [][]string{{"H/loose.m4a"}, {"H/one.m4b"}, {"H/two.m4b"}},
			narrator:   []string{"", "", ""},
		},
		{name: "no audio", files: []naming.AudiobookFile{{Path: "a/b.jpg"}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			books := naming.GroupAudiobooks(test.files)
			if len(books) != len(test.wantTitles) {
				t.Fatalf("got %d books, want %d: %+v", len(books), len(test.wantTitles), books)
			}

			for index, book := range books {
				if book.Title != test.wantTitles[index] || book.Folder != test.wantFolder[index] ||
					book.Narrator != test.narrator[index] || !reflect.DeepEqual(paths(book), test.wantParts[index]) {
					t.Errorf("book %d = %+v (parts %v), want title %q folder %q narrator %q parts %v",
						index, book, paths(book), test.wantTitles[index], test.wantFolder[index],
						test.narrator[index], test.wantParts[index])
				}
			}
		})
	}
}

func TestGroupAudiobooksNaturalTies(t *testing.T) {
	t.Parallel()

	books := naming.GroupAudiobooks([]naming.AudiobookFile{
		{Path: "T/a1.mp3"}, {Path: "T/a01.mp3"}, {Path: "T/a.mp3"}, {Path: "T/A.mp3"},
	})

	got := paths(books[0])
	want := []string{"T/A.mp3", "T/a.mp3", "T/a01.mp3", "T/a1.mp3"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestPlaceRejectsAudiobooks(t *testing.T) {
	t.Parallel()

	_, err := naming.Place(library.KindAudiobooks, naming.FileContext{Path: "a/b.mp3"})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("Place(audiobooks) err = %v, want ErrInvalid", err)
	}
}
