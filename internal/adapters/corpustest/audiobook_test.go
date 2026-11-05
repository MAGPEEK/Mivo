package corpustest_test

import (
	"encoding/json"
	"io/fs"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/naming"
)

const (
	// authorTagAlbumArtist marks books whose author is also written as album artist.
	authorTagAlbumArtist = "album_artist"
	// partDurationTolerance absorbs encoder padding (MP3 frames, AAC priming) of a part.
	partDurationTolerance = 100 * time.Millisecond
	// chapterTolerance absorbs the rounding of millisecond chapter marks to the stream
	// time base.
	chapterTolerance = time.Millisecond
)

// audiobookManifest is the audiobooks object of the library manifest (task M3-14f).
type audiobookManifest struct {
	Path  string         `json:"path"`
	Books []audiobookRow `json:"books"`
}

type audiobookRow struct {
	Title       string                `json:"title"`
	Author      string                `json:"author"`
	AuthorTag   string                `json:"authorTag"`
	Composer    string                `json:"composer"`
	Narrator    string                `json:"narrator"`
	Path        string                `json:"path"`
	CoverSource string                `json:"coverSource"`
	CoverFile   string                `json:"coverFile"`
	Parts       []audiobookPartRow    `json:"parts"`
	Chapters    []audiobookChapterRow `json:"chapters"`
}

type audiobookPartRow struct {
	File            string  `json:"file"`
	Title           string  `json:"title"`
	Disc            int     `json:"disc"`
	Track           int     `json:"track"`
	DurationSeconds float64 `json:"durationSeconds"`
}

type audiobookChapterRow struct {
	File         string  `json:"file"`
	Title        string  `json:"title"`
	StartSeconds float64 `json:"startSeconds"`
	EndSeconds   float64 `json:"endSeconds"`
	Embedded     bool    `json:"embedded"`
}

// TestCorpusAudiobookPartsAreReadable checks every part file of the audiobook manifest: the
// tags, the length, the narrator tag, the chapters the file carries and the cover.
func TestCorpusAudiobookPartsAreReadable(t *testing.T) {
	t.Parallel()

	readers, root := requireMusicCorpus(t)
	manifest := readAudiobookManifest(t, root)
	results := caseResultsByFile(manifest.Cases)
	bookRoot := filepath.Join(root, filepath.FromSlash(manifest.Audiobooks.Path))

	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Fatalf("look up ffprobe: %v", err)
	}

	for _, book := range manifest.Audiobooks.Books {
		for _, part := range book.Parts {
			t.Run(part.File, func(t *testing.T) {
				t.Parallel()

				result := results[path.Join(manifest.Audiobooks.Path, part.File)]
				if result.Status != statusGenerated {
					t.Skipf("case %s was not generated: %s", result.Name, result.Reason)
				}

				file := filepath.Join(bookRoot, filepath.FromSlash(part.File))
				readers.assertPartTags(t, file, book, part)
				assertNarratorTag(t, ffprobePath, file, book)
				readers.assertEmbeddedChapters(t, file, book.chaptersOf(part.File))
				readers.assertCover(t, bookRoot, musicTrackRow{
					File: part.File, CoverSource: book.CoverSource, CoverFile: book.CoverFile,
				})
			})
		}
	}
}

// TestCorpusAudiobooksGroupIntoTheManifestBooks reads the tags of every audio file of the
// audiobook tree and groups them with naming.GroupAudiobooks: the books, their authors and
// the play order of their parts must be the ones the manifest names.
func TestCorpusAudiobooksGroupIntoTheManifestBooks(t *testing.T) {
	t.Parallel()

	readers, root := requireMusicCorpus(t)
	manifest := readAudiobookManifest(t, root)
	requireGeneratedAudiobooks(t, manifest)

	bookRoot := filepath.Join(root, filepath.FromSlash(manifest.Audiobooks.Path))
	books := naming.GroupAudiobooks(readers.audiobookFiles(t, bookRoot))

	if len(books) != len(manifest.Audiobooks.Books) {
		t.Fatalf("%d books, want %d", len(books), len(manifest.Audiobooks.Books))
	}

	for _, want := range manifest.Audiobooks.Books {
		index := slices.IndexFunc(books, func(book naming.Audiobook) bool {
			return book.Title == want.Title && book.Folder == want.Path
		})
		if index < 0 {
			t.Errorf("no book %q in %q among %+v", want.Title, want.Path, books)

			continue
		}

		got := books[index]
		gotParts := make([]string, 0, len(got.Parts))

		for _, part := range got.Parts {
			gotParts = append(gotParts, part.Path)
		}

		if got.Author != want.Author || !slices.Equal(gotParts, want.partFiles()) {
			t.Errorf("book %q: author %q, parts %v; want %q, %v", want.Title, got.Author, gotParts, want.Author, want.partFiles())
		}
	}
}

func readAudiobookManifest(t *testing.T, root string) libraryManifest {
	t.Helper()

	manifest := readLibraryManifest(t, root)
	if len(manifest.Audiobooks.Books) == 0 {
		t.Fatal("the library manifest lists no audiobook; regenerate it with `go run ./scripts/gen-testmedia`")
	}

	return manifest
}

func requireGeneratedAudiobooks(t *testing.T, manifest libraryManifest) {
	t.Helper()

	results := caseResultsByFile(manifest.Cases)

	for _, book := range manifest.Audiobooks.Books {
		for _, part := range book.Parts {
			if result := results[path.Join(manifest.Audiobooks.Path, part.File)]; result.Status != statusGenerated {
				t.Skipf("case %s was not generated: %s", result.Name, result.Reason)
			}
		}
	}
}

// audiobookFiles walks the audiobook tree and returns the grouping input of every audio file.
func (readers musicReaders) audiobookFiles(t *testing.T, bookRoot string) []naming.AudiobookFile {
	t.Helper()

	var files []naming.AudiobookFile

	err := filepath.WalkDir(bookRoot, func(file string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !naming.IsAudiobookFileName(entry.Name()) {
			return walkErr
		}

		relative, err := filepath.Rel(bookRoot, file)
		if err != nil {
			return err
		}

		tags, _, err := readers.prober.ReadAudioTags(t.Context(), file)
		if err != nil {
			return err
		}

		files = append(files, naming.NewAudiobookFile(filepath.ToSlash(relative), tags))

		return nil
	})
	if err != nil {
		t.Fatalf("read the audiobook tree: %v", err)
	}

	return files
}

// assertPartTags compares the tags the scan reads today; composer and narrator are checked
// on the raw tags by assertNarratorTag until ReadAudioTags fills them (M3-14g).
func (readers musicReaders) assertPartTags(t *testing.T, file string, book audiobookRow, part audiobookPartRow) {
	t.Helper()

	tags, duration, err := readers.prober.ReadAudioTags(t.Context(), file)
	if err != nil {
		t.Fatalf("ReadAudioTags: %v", err)
	}

	tags.Composer, tags.Narrator, tags.ComposerIsNarrator = "", "", false

	want := media.AudioTags{Artist: book.Author, Album: book.Title, Title: part.Title, Disc: part.Disc, Track: part.Track}
	if book.AuthorTag == authorTagAlbumArtist {
		want.AlbumArtist = book.Author
	}

	if tags != want {
		t.Errorf("tags = %+v, want %+v", tags, want)
	}

	if nominal := secondsToDuration(part.DurationSeconds); (duration - nominal).Abs() > partDurationTolerance {
		t.Errorf("duration = %v, want %v ± %v", duration, nominal, partDurationTolerance)
	}
}

// assertNarratorTag reads the raw format tags: the composer and the explicit narrator tag
// are exactly the ones the manifest names (an absent tag reads as "").
func assertNarratorTag(t *testing.T, ffprobePath, file string, book audiobookRow) {
	t.Helper()

	output, err := exec.CommandContext(t.Context(), ffprobePath, //nolint:gosec // ffprobe from PATH, corpus file
		"-v", "error", "-show_entries", "format_tags", "-of", "json", file).Output()
	if err != nil {
		t.Fatalf("ffprobe format tags: %v", err)
	}

	var probe struct {
		Format struct {
			Tags map[string]string `json:"tags"`
		} `json:"format"`
	}

	if err := json.Unmarshal(output, &probe); err != nil {
		t.Fatalf("decode ffprobe output: %v", err)
	}

	tags := probe.Format.Tags
	if tags["composer"] != book.Composer || tags["narrator"] != book.Narrator {
		t.Errorf("composer tag %q, narrator tag %q; want %q, %q", tags["composer"], tags["narrator"], book.Composer, book.Narrator)
	}
}

// assertEmbeddedChapters compares the chapters the file carries; a part whose manifest
// chapter is derived must carry none.
func (readers musicReaders) assertEmbeddedChapters(t *testing.T, file string, want []audiobookChapterRow) {
	t.Helper()

	chapters, err := readers.prober.ReadChapters(t.Context(), file)
	if err != nil {
		t.Fatalf("ReadChapters: %v", err)
	}

	want = slices.DeleteFunc(slices.Clone(want), func(entry audiobookChapterRow) bool { return !entry.Embedded })
	if len(chapters) != len(want) {
		t.Fatalf("%d chapters %+v, want %d", len(chapters), chapters, len(want))
	}

	for index, entry := range want {
		got := chapters[index]
		start, end := secondsToDuration(entry.StartSeconds), secondsToDuration(entry.EndSeconds)

		if got.Title != entry.Title || (got.Start-start).Abs() > chapterTolerance || (got.End-end).Abs() > chapterTolerance {
			t.Errorf("chapter %d = %q %v-%v, want %q %v-%v", index, got.Title, got.Start, got.End, entry.Title, start, end)
		}
	}
}

func (book audiobookRow) chaptersOf(file string) []audiobookChapterRow {
	var chapters []audiobookChapterRow

	for _, entry := range book.Chapters {
		if entry.File == file {
			chapters = append(chapters, entry)
		}
	}

	return chapters
}

func (book audiobookRow) partFiles() []string {
	files := make([]string, 0, len(book.Parts))
	for _, part := range book.Parts {
		files = append(files, part.File)
	}

	return files
}

func secondsToDuration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}
