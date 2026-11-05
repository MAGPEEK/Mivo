package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/platform/paths"
)

const (
	// audiobooksLibraryName is the name the test gives the audiobook library.
	audiobooksLibraryName = "Audiobooks"
	// partDurationTolerance absorbs encoder padding (MP3 frames, AAC priming) of one part
	// against its nominal length in the manifest.
	partDurationTolerance = 100 * time.Millisecond
	// embeddedChapterTolerance absorbs the rounding of chapter marks to the stream time base
	// and to the stored milliseconds.
	embeddedChapterTolerance = 2 * time.Millisecond
	// directPlayBookContainer is the book container the test profile plays directly (AAC in
	// MP4); the MP3 books are transcoded to AAC over HLS.
	directPlayBookContainer = "m4b"
	// transcodedBookContainer is the book container the test profile cannot play directly.
	transcodedBookContainer = "mp3"
)

// audiobookCorpusManifest holds the audiobook part of testdata/library/manifest.json (M3-14f).
type audiobookCorpusManifest struct {
	Audiobooks struct {
		Path  string            `json:"path"`
		Books []corpusAudiobook `json:"books"`
	} `json:"audiobooks"`
}

type corpusAudiobook struct {
	Title       string                   `json:"title"`
	Author      string                   `json:"author"`
	Narrator    string                   `json:"narrator"`
	Container   string                   `json:"container"`
	CoverSource string                   `json:"coverSource"`
	Parts       []corpusAudiobookPart    `json:"parts"`
	Chapters    []corpusAudiobookChapter `json:"chapters"`
}

type corpusAudiobookPart struct {
	File            string  `json:"file"`
	DurationSeconds float64 `json:"durationSeconds"`
}

type corpusAudiobookChapter struct {
	File         string  `json:"file"`
	Title        string  `json:"title"`
	StartSeconds float64 `json:"startSeconds"`
	EndSeconds   float64 `json:"endSeconds"`
	Embedded     bool    `json:"embedded"`
}

// TestRunServesTheAudiobookCorpus is the audiobook walking skeleton (M3-14g): a configured
// audiobook library is scanned at startup; books, authors, narrators, covers, the summed
// length and the chapters (titles, file-relative offsets, part files) match the generator's
// manifest, and a book file plays directly or transcoded through the playback path.
func TestRunServesTheAudiobookCorpus(t *testing.T) {
	t.Parallel()

	corpus := copyLibraryCorpus(t)
	books := readAudiobookManifest(t)
	environment := isolatedEnvironment(t)
	writeConfigFile(t, environment[paths.ConfigDirEnv()], fmt.Sprintf(
		"scan:\n  watch: false\n  interval: 0\nlibraries:\n  - name: %s\n    kind: audiobooks\n    path: '%s'\n",
		audiobooksLibraryName, filepath.Join(corpus.root, books.Audiobooks.Path)))

	server := startTestServer(t, environment, runHooks{})
	assertLibraryKindReported(t, server.baseURL, apigen.LibraryKindAudiobooks)

	libraryID := awaitScannedLibraries(t, server.baseURL, 1, time.Time{})[audiobooksLibraryName].ID.String()
	listed := listItems(t, server.baseURL, libraryID)
	assertBookTitles(t, listed, books.Audiobooks.Books)

	filesByContainer := map[string]string{}

	for _, want := range books.Audiobooks.Books {
		book := findByTitle(t, listed, want.Title)
		assertBookSummary(t, want, book)

		detail := assertBookDetail(t, server.baseURL, want, book)
		filesByContainer[want.Container] = detail.Files[0].ID.String()
	}

	assertAudioFilePlays(t, server.baseURL, audioPlayback{label: "m4b book", fileID: filesByContainer[directPlayBookContainer], mode: apigen.PlaybackModeDirectPlay})
	assertAudioFilePlays(t, server.baseURL, audioPlayback{label: "mp3 book", fileID: filesByContainer[transcodedBookContainer], mode: apigen.PlaybackModeTranscode})

	server.stop(t)
	assertNoSessionDirectories(t, environment[envPathsTranscode])
}

// readAudiobookManifest reads the audiobook part of the manifest the corpus copy came from.
func readAudiobookManifest(t *testing.T) audiobookCorpusManifest {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(libraryCorpusDirectory, libraryManifestFile))
	if err != nil {
		t.Fatalf("read the library manifest: %v", err)
	}

	var manifest audiobookCorpusManifest
	if err := json.Unmarshal(content, &manifest); err != nil || len(manifest.Audiobooks.Books) == 0 {
		t.Fatalf("library manifest has no audiobooks (error %v); regenerate it with `go run ./scripts/gen-testmedia`", err)
	}

	return manifest
}

func assertLibraryKindReported(t *testing.T, baseURL string, kind apigen.LibraryKind) {
	t.Helper()

	var info apigen.SystemInfo

	getJSON(t, baseURL+"/api/v1/system/info", &info)

	if !slices.Contains(info.LibraryKinds, kind) {
		t.Errorf("libraryKinds = %v, want %s", info.LibraryKinds, kind)
	}
}

// assertBookTitles checks that the default listing holds exactly the manifest's books,
// all of kind audiobook.
func assertBookTitles(t *testing.T, listed []apigen.ItemSummary, books []corpusAudiobook) {
	t.Helper()

	want := make([]string, 0, len(books))
	for _, book := range books {
		want = append(want, book.Title)
	}

	slices.Sort(want)

	// The listing sorts by sort title ("The" is ignored); the order is the listing tests' concern.
	if got := slices.Sorted(slices.Values(itemTitles(listed))); !slices.Equal(got, want) {
		t.Errorf("books = %v, want %v", got, want)
	}

	for _, item := range listed {
		if item.Kind != apigen.ItemKindAudiobook {
			t.Errorf("%q kind = %s, want audiobook", item.Title, item.Kind)
		}
	}
}

// assertBookSummary checks the card: the 2:3 cover as poster (not square) when the book has one and the
// summed length of all parts as the media duration.
func assertBookSummary(t *testing.T, want corpusAudiobook, book apigen.ItemSummary) {
	t.Helper()

	if want.CoverSource == coverSourceNone {
		if book.Poster != nil {
			t.Errorf("book %q has no cover but lists poster %+v", want.Title, book.Poster)
		}
	} else if book.Poster == nil || book.Square != nil {
		t.Errorf("book %q has a cover but poster = %+v, square = %+v", want.Title, book.Poster, book.Square)
	}

	var nominal time.Duration
	for _, part := range want.Parts {
		nominal += secondsDuration(part.DurationSeconds)
	}

	tolerance := time.Duration(len(want.Parts)) * partDurationTolerance
	if book.Media == nil || book.Media.DurationSeconds == nil ||
		(secondsDuration(*book.Media.DurationSeconds)-nominal).Abs() > tolerance {
		t.Errorf("book %q media = %+v, want durationSeconds %v ± %v", want.Title, book.Media, nominal, tolerance)
	}
}

// assertBookDetail checks the people, the part files in play order and the chapters of a
// book and returns its detail.
func assertBookDetail(t *testing.T, baseURL string, want corpusAudiobook, book apigen.ItemSummary) apigen.ItemDetail {
	t.Helper()

	var detail apigen.ItemDetail

	getJSON(t, baseURL+"/api/v1/items/"+book.ID.String(), &detail)

	assertBookPeople(t, want, detail)

	if want.CoverSource != coverSourceNone {
		assertPosterServed(t, baseURL, want.Title, detail)
	}

	wantFiles := make([]string, 0, len(want.Parts))
	for _, part := range want.Parts {
		wantFiles = append(wantFiles, path.Base(part.File))
	}

	gotFiles := make([]string, 0, len(detail.Files))
	for _, file := range detail.Files {
		gotFiles = append(gotFiles, file.FileName)
	}

	if !slices.Equal(gotFiles, wantFiles) {
		t.Fatalf("book %q files = %v, want %v", want.Title, gotFiles, wantFiles)
	}

	assertBookChapters(t, want, detail)

	return detail
}

// assertBookPeople expects the author and, when the manifest names one, the narrator.
func assertBookPeople(t *testing.T, want corpusAudiobook, detail apigen.ItemDetail) {
	t.Helper()

	wantPeople := []apigen.ItemPerson{{Name: want.Author, Role: apigen.PersonRoleAuthor}}
	if want.Narrator != "" {
		wantPeople = append(wantPeople, apigen.ItemPerson{Name: want.Narrator, Role: apigen.PersonRoleNarrator})
	}

	var got []apigen.ItemPerson
	if detail.People != nil {
		got = *detail.People
	}

	if !slices.Equal(got, wantPeople) {
		t.Errorf("book %q people = %+v, want %+v", want.Title, got, wantPeople)
	}
}

// assertBookChapters compares the chapters with the manifest: titles in play order, offsets
// relative to their part file (derived whole-part chapters end at the probed length, so
// within the encoder padding) and the part file that holds each chapter.
func assertBookChapters(t *testing.T, want corpusAudiobook, detail apigen.ItemDetail) {
	t.Helper()

	var got []apigen.ItemChapter
	if detail.Chapters != nil {
		got = *detail.Chapters
	}

	if len(got) != len(want.Chapters) {
		t.Fatalf("book %q has %d chapters %+v, want %d", want.Title, len(got), got, len(want.Chapters))
	}

	fileIDs := map[string]string{}
	for _, file := range detail.Files {
		fileIDs[file.FileName] = file.ID.String()
	}

	for index, wantChapter := range want.Chapters {
		tolerance := partDurationTolerance
		if wantChapter.Embedded {
			tolerance = embeddedChapterTolerance
		}

		chapter := got[index]
		if chapter.Title != wantChapter.Title || chapter.FileID.String() != fileIDs[path.Base(wantChapter.File)] ||
			!withinMilliseconds(chapter.StartMs, wantChapter.StartSeconds, tolerance) ||
			!withinMilliseconds(chapter.EndMs, wantChapter.EndSeconds, tolerance) {
			t.Errorf("book %q chapter %d = %+v, want %+v in %s", want.Title, index, chapter, wantChapter, fileIDs[path.Base(wantChapter.File)])
		}
	}
}

func withinMilliseconds(milliseconds int64, seconds float64, tolerance time.Duration) bool {
	return (time.Duration(milliseconds)*time.Millisecond - secondsDuration(seconds)).Abs() <= tolerance
}

func secondsDuration(seconds float64) time.Duration {
	return time.Duration(math.Round(seconds * float64(time.Second)))
}
