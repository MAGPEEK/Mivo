package main

import (
	"path"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// audiobookCatalogFiles lists the parts and the cover images (once each).
func audiobookCatalogFiles(audiobooks expectedAudiobooks) []string {
	var files []string

	for _, book := range audiobooks.Books {
		for _, part := range book.Parts {
			files = append(files, path.Join(audiobooks.Path, part.File))
		}

		image := path.Join(audiobooks.Path, book.CoverFile)
		if book.CoverSource == coverSourceFolder && !slices.Contains(files, image) {
			files = append(files, image)
		}
	}

	return files
}

func audiobookNamed(t *testing.T, title string) expectedAudiobook {
	t.Helper()

	for _, book := range audiobookCatalog().Books {
		if book.Title == title {
			return book
		}
	}

	t.Fatalf("no audiobook %q", title)

	return expectedAudiobook{}
}

// audiobookPartSpecOf finds the spec of a part by its file, relative to the library root.
func audiobookPartSpecOf(t *testing.T, file string) audiobookPartSpec {
	t.Helper()

	for _, spec := range audiobookSpecs() {
		for _, part := range spec.parts {
			if part.file == file {
				return part
			}
		}
	}

	t.Fatalf("no audiobook part %q", file)

	return audiobookPartSpec{}
}

// TestAudiobookCatalogCoversTheRequiredBooks pins the books task M3-14f asks for.
func TestAudiobookCatalogCoversTheRequiredBooks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		title       string
		container   string
		parts       int
		chapters    int
		standsAlone bool
		coverSource string
		composer    string
		narrator    string
	}{
		// A composer alone is no narrator, so the M4B books expect none (M3-14a/M3-14e).
		{"The Clockwork Orchard", audiobookContainerM4B, 1, 3, true, coverSourceEmbedded, "Nora Vale", ""},
		{"The Paper Lighthouse", audiobookContainerMP3, 3, 3, false, coverSourceFolder, "", "Iris Fenn"},
		{"Garden Notes", audiobookContainerMP3, 1, 1, false, coverSourceNone, "", ""},
		{"Midnight Recipes", audiobookContainerM4B, 1, 1, true, coverSourceFolder, "Otto Brine", ""},
	}

	for _, test := range tests {
		book := audiobookNamed(t, test.title)

		if book.Container != test.container || len(book.Parts) != test.parts || len(book.Chapters) != test.chapters ||
			book.StandsAlone != test.standsAlone || book.CoverSource != test.coverSource ||
			book.Composer != test.composer || book.Narrator != test.narrator {
			t.Errorf("%s = %+v", test.title, book)
		}

		for _, part := range book.Parts {
			if !strings.HasSuffix(part.File, "."+test.container) || path.Dir(part.File) != book.Path {
				t.Errorf("%s: part %s is not a %s file in %s", test.title, part.File, test.container, book.Path)
			}
		}
	}

	if garden, recipes := audiobookNamed(t, "Garden Notes"), audiobookNamed(t, "Midnight Recipes"); garden.Path != recipes.Path {
		t.Errorf("the single MP3 book and the M4B must share a folder: %q, %q", garden.Path, recipes.Path)
	}
}

func TestAudiobookChaptersAreFileRelativeInPlayOrder(t *testing.T) {
	t.Parallel()

	orchard := audiobookNamed(t, "The Clockwork Orchard")
	file := orchard.Parts[0].File
	want := []expectedChapter{
		{File: file, Title: "The Seed Vault", StartSeconds: 0, EndSeconds: 2, Embedded: true},
		{File: file, Title: "Brass Branches", StartSeconds: 2, EndSeconds: 4, Embedded: true},
		{File: file, Title: "Harvest Moon", StartSeconds: 4, EndSeconds: 6, Embedded: true},
	}

	if !reflect.DeepEqual(orchard.Chapters, want) || orchard.Parts[0].DurationSeconds != 6 {
		t.Errorf("chapters = %+v, parts %+v", orchard.Chapters, orchard.Parts)
	}

	lighthouse := audiobookNamed(t, "The Paper Lighthouse")
	titles := make([]string, 0, len(lighthouse.Chapters))

	for index, entry := range lighthouse.Chapters {
		titles = append(titles, entry.Title)

		if entry.Embedded || entry.StartSeconds != 0 || entry.EndSeconds != 2 || entry.File != lighthouse.Parts[index].File {
			t.Errorf("derived chapter %d = %+v", index, entry)
		}
	}

	if !slices.Equal(titles, []string{"Storm", "Keeper", "Dawn"}) {
		t.Errorf("play order = %v, want the disc and track order", titles)
	}
}

func TestAudiobookPartTags(t *testing.T) {
	t.Parallel()

	lighthouse := audiobookNamed(t, "The Paper Lighthouse")
	dawn := audiobookPartSpecOf(t, "Bram Quill/The Paper Lighthouse/Dawn.mp3")
	want := map[string]string{
		"title": "Dawn", "album": "The Paper Lighthouse", "artist": "Bram Quill", "album_artist": "Bram Quill",
		"narrator": "Iris Fenn", "track": "1/1", "disc": "2/2",
	}

	if got := audiobookPartTags(lighthouse, dawn); !reflect.DeepEqual(got, want) {
		t.Errorf("tags = %v, want %v", got, want)
	}

	garden := audiobookNamed(t, "Garden Notes")
	want = map[string]string{"title": "Garden Notes", "album": "Garden Notes", "artist": "Cleo Marsh"}

	if got := audiobookPartTags(garden, audiobookPartSpecOf(t, "Short Reads/Garden Notes.mp3")); !reflect.DeepEqual(got, want) {
		t.Errorf("tags = %v, want %v", got, want)
	}

	orchard := audiobookNamed(t, "The Clockwork Orchard")
	got := audiobookPartTags(orchard, audiobookPartSpecOf(t, "Ada Inkwell/The Clockwork Orchard.m4b"))

	if _, hasNarrator := got[narratorTag]; got[composerTag] != "Nora Vale" || hasNarrator {
		t.Errorf("M4B tags = %v, want a composer and no narrator", got)
	}
}

func TestAudiobookCasesBuildTheCoverFirstAndEmbedItInTheChapteredM4B(t *testing.T) {
	t.Parallel()

	cases := libraryAudiobookCases()
	cover := cases[0]

	if cover.Name != audiobookCoverCaseName || cover.Output != "audiobooks/Bram Quill/The Paper Lighthouse/cover.jpg" ||
		!slices.Equal(cover.Copies, []string{"audiobooks/Short Reads/Midnight Recipes.jpg"}) || cover.Source.Height <= cover.Source.Width {
		t.Fatalf("first audiobook case = %+v, want the portrait cover image and its copy", cover)
	}

	for _, c := range cases[1:] {
		chaptered := strings.HasSuffix(c.Output, "The Clockwork Orchard.m4b")
		if (c.Cover == cover.Output) != chaptered || (len(c.Inputs) == 1) != chaptered || (len(c.Expect.Chapters) > 0) != chaptered {
			t.Errorf("%s: cover %q, %d inputs, %d chapters", c.Name, c.Cover, len(c.Inputs), len(c.Expect.Chapters))
		}

		if c.Category != categoryAudiobooks || c.Source.hasVideo() || !c.Source.hasAudio() {
			t.Errorf("%s: category %s, source %+v, want audio-only audiobook", c.Name, c.Category, c.Source)
		}
	}
}

func TestAudiobookFormatOf(t *testing.T) {
	t.Parallel()

	for container, muxer := range map[string]string{audiobookContainerM4B: "ipod", audiobookContainerMP3: "mp3"} {
		if got := audiobookFormatOf(container).muxer; got != muxer {
			t.Errorf("audiobookFormatOf(%s).muxer = %s, want %s", container, got, muxer)
		}
	}
}

func TestWriteAudiobookSummary(t *testing.T) {
	t.Parallel()

	var empty strings.Builder
	if writeAudiobookSummary(&empty, expectedAudiobooks{}); empty.Len() != 0 {
		t.Errorf("summary without books = %q, want nothing", empty.String())
	}

	var text strings.Builder

	writeAudiobookSummary(&text, audiobookCatalog())
	checkSummaryLines(t, text.String(),
		"  audiobooks/ (4 books)\n",
		"    book Ada Inkwell - The Clockwork Orchard: 1 m4b part, 3 chapters, stands alone, composer Nora Vale, cover embedded\n",
		"    book Bram Quill - The Paper Lighthouse: 3 mp3 parts, narrator Iris Fenn, cover folder\n",
		"    book Cleo Marsh - Garden Notes: 1 mp3 part, cover none\n",
	)
}
