package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// libraryOptions selects only the library cases and writes them to a temporary directory.
func libraryOptions(t *testing.T) options {
	t.Helper()

	return options{
		outDir: t.TempDir(), libraryDir: t.TempDir(), ffmpeg: "fake",
		only: mustParseFilter(t, libraryCasePrefix+"*"), jobs: 4,
	}
}

func readLibraryManifest(t *testing.T, dir string) libraryManifest {
	t.Helper()

	data, err := fs.ReadFile(os.DirFS(dir), manifestFileName)
	if err != nil {
		t.Fatalf("read library manifest: %v", err)
	}

	var result libraryManifest
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("decode library manifest: %v", err)
	}

	return result
}

// TestGenerateLibraryWritesTheTreeWithFakeFFmpeg runs the program logic with the fake FFmpeg
// and checks the tree layout on disk against the manifest it wrote.
func TestGenerateLibraryWritesTheTreeWithFakeFFmpeg(t *testing.T) {
	t.Parallel()

	opts := libraryOptions(t)
	ffmpeg := newFixtureFFmpeg(t)
	stdout := mustGenerateCorpus(t, opts, ffmpeg)

	result := readLibraryManifest(t, opts.libraryDir)
	if result.SchemaVersion != libraryManifestSchemaVersion || result.FFmpegVersion != "9.0.2-full_build-www.gyan.dev" {
		t.Errorf("manifest header = %d %q", result.SchemaVersion, result.FFmpegVersion)
	}

	if !reflect.DeepEqual(result.Libraries, libraryCatalog()) || !reflect.DeepEqual(result.Music, musicCatalog()) ||
		!reflect.DeepEqual(result.Discs, discCatalog()) || !reflect.DeepEqual(result.Audiobooks, audiobookCatalog()) {
		t.Error("the manifest catalog differs from libraryCatalog(), musicCatalog(), discCatalog() or audiobookCatalog()")
	}

	wantCases := len(libraryCases()) + len(discAssemblyCases())
	if len(result.Cases) != wantCases || result.count(statusGenerated) != len(result.Cases) {
		t.Errorf("cases = %+v, want every library case and disc generated", result.Cases)
	}

	checkCatalogFilesExist(t, opts.libraryDir, result)

	// One run per case (discs are assembled without FFmpeg); the cover runs read the poster
	// or the album cover image as input, the subtitled disc sources the generated PGS stream.
	if encodes := len(ffmpeg.encodedOutputs()); encodes != len(libraryCases()) {
		t.Errorf("%d FFmpeg runs, want %d", encodes, len(libraryCases()))
	}

	checkCoverInputs(t, ffmpeg.textInputs())

	if got := readOutput(t, opts.libraryDir, "shows/Dragon Ball Super/Dragon Ball Super - 131.mkv"); got != fakeMediaContent {
		t.Errorf("episode 131 = %q, want a copy of episode 001", got)
	}

	if got := readOutput(t, opts.libraryDir, "movies/Spring (2019)/Spring (2019).nfo"); got != tmdbMovieURL+"1000002\n" {
		t.Errorf("URL NFO = %q", got)
	}

	checkSummaryLines(t, stdout,
		"library\n", "  artwork     4 generated", "  music       14 generated", "  shows       4 generated",
		"  audiobooks  7 generated", "  movies/ (13 movies)\n", "  audiobooks/ (4 books)\n",
		"    show  Dragon Ball Super: 1 season, 131 episodes\n",
		"    movie Big Buck Bunny (2008): 2 versions, 2 extras, artwork poster backdrop logo\n",
		"    movie Caminandes (2013): 1 version, nfo malformed\n",
		"    movie Glass Half (2015): 1 version, artwork poster(embedded)\n",
		"  music/ (5 albums)\n",
		"    album Generated Orchestra - Two Disc Symphony (2019): 4 flac tracks on 2 discs, cover embedded\n",
		"    album Various Artists - Generated Hits (2024): 3 mp3 tracks, compilation, cover none\n",
		"    album Folder Artist - Folder Album: 2 flac tracks, untagged, cover none\n",
		"  discs       10 generated", "  discs/ (5 discs)\n",
		"    bdmv     Feature (2021): 7 s main title\n",
		"    video_ts Scrambled Movie (2018): css stand-in, disc_encrypted\n",
	)
}

// checkCoverInputs expects the poster for the MP4 cover, the album cover for each FLAC track
// with an embedded cover, the book cover and the chapter list of the chaptered M4B and the
// PGS subtitles of the DVD feature and the two Blu-ray clips.
func checkCoverInputs(t *testing.T, inputs []seenInput) {
	t.Helper()

	names := make([]string, 0, len(inputs))
	for _, input := range inputs {
		names = append(names, filepath.Base(input.path))
	}

	slices.Sort(names)

	want := []string{
		"chapters.ffmeta", folderCoverName, folderCoverName, folderCoverName, folderCoverName, folderCoverName, "poster.jpg",
		subtitleFileName, subtitleFileName, subtitleFileName,
	}
	if !slices.Equal(names, want) {
		t.Errorf("file inputs = %v, want %v", names, want)
	}
}

func checkCatalogFilesExist(t *testing.T, dir string, result libraryManifest) {
	t.Helper()

	for _, file := range catalogFiles(result.Libraries, result.Music, result.Discs, result.Audiobooks) {
		if _, err := fs.Stat(os.DirFS(dir), file); err != nil {
			t.Errorf("catalog file missing on disk: %v", err)
		}
	}
}

func checkSummaryLines(t *testing.T, summary string, lines ...string) {
	t.Helper()

	for _, want := range lines {
		if !strings.Contains(summary, want) {
			t.Errorf("summary lacks %q:\n%s", want, summary)
		}
	}
}

func TestGenerateLibraryKeepsFilesOnTheSecondRun(t *testing.T) {
	t.Parallel()

	opts := libraryOptions(t)
	mustGenerateCorpus(t, opts, newFixtureFFmpeg(t))

	if err := os.Remove(filepath.Join(opts.libraryDir, "shows", "Dragon Ball Super", "Dragon Ball Super - 077.mkv")); err != nil {
		t.Fatal(err)
	}

	ffmpeg := newFixtureFFmpeg(t)
	mustGenerateCorpus(t, opts, ffmpeg)

	if encodes := len(ffmpeg.encodedOutputs()); encodes != 0 {
		t.Errorf("%d FFmpeg runs on the second run, want none", encodes)
	}

	if got := readOutput(t, opts.libraryDir, "shows/Dragon Ball Super/Dragon Ball Super - 077.mkv"); got != fakeMediaContent {
		t.Errorf("the deleted copy was not restored: %q", got)
	}
}

func TestGenerateLibraryFailureSkipsTheCoversAndFailsTheRun(t *testing.T) {
	t.Parallel()

	opts := libraryOptions(t)
	ffmpeg := newFixtureFFmpeg(t)
	ffmpeg.failOutputs["poster.jpg"] = "Conversion failed!"

	err := generateCorpus(context.Background(), opts, ffmpeg, io.Discard)
	if !errors.Is(err, errCasesFailed) {
		t.Fatalf("error = %v, want errCasesFailed", err)
	}

	result := readLibraryManifest(t, opts.libraryDir)
	statuses := map[string]string{}

	for _, entry := range result.Cases {
		statuses[entry.Name] = string(entry.Status) + " " + string(entry.SkipKind)
	}

	for name, want := range map[string]string{
		"library-artwork-poster":                             "failed ",
		"library-movies-glass-half-2015-cover":               "skipped missing-input",
		"library-movies-night-of-the-living-dead-1968-cover": "skipped missing-input",
		"library-shows-dragon-ball-super":                    "generated ",
	} {
		if statuses[name] != want {
			t.Errorf("%s = %q, want %q", name, statuses[name], want)
		}
	}
}

func TestGenerateCorpusWithoutLibraryDirWritesNoLibrary(t *testing.T) {
	t.Parallel()

	opts := libraryOptions(t)
	opts.libraryDir = ""

	if out := mustGenerateCorpus(t, opts, newFixtureFFmpeg(t)); strings.Contains(out, "library\n") {
		t.Errorf("summary mentions a library:\n%s", out)
	}
}

func TestGenerateCorpusFailsWhenTheLibraryManifestCannotBeWritten(t *testing.T) {
	t.Parallel()

	opts := libraryOptions(t)
	opts.only = mustParseFilter(t, "none")
	opts.libraryDir = filepath.Join(opts.libraryDir, "blocked")
	writeFile(t, opts.libraryDir, "a file, not a folder")

	if err := generateCorpus(context.Background(), opts, newFixtureFFmpeg(t), io.Discard); err == nil ||
		errors.Is(err, errCasesFailed) {
		t.Errorf("error = %v, want the manifest write error", err)
	}
}

func TestLibraryManifestJSONShape(t *testing.T) {
	t.Parallel()

	c := clipCase("library-shows-a", libraryKindShows, "shows/a/1.mkv", []string{"shows/a/2.mkv"})
	c.Sidecars = []sidecarFile{{Output: "shows/a/tvshow.nfo"}}

	result := newLibraryManifest("9", []mediaCase{c}, []manifestEntry{
		newSkippedEntry(c, skipMissingInput, "cover image x was not built"),
	})

	data, err := json.Marshal(result.Cases)
	if err != nil {
		t.Fatal(err)
	}

	const want = `[{"name":"library-shows-a","category":"shows","status":"skipped","skipKind":"missing-input",` +
		`"reason":"cover image x was not built","files":["shows/a/1.mkv","shows/a/2.mkv","shows/a/tvshow.nfo"]}]`
	if string(data) != want {
		t.Errorf("cases JSON =\n%s\nwant\n%s", data, want)
	}

	var episode bytes.Buffer
	if err := json.NewEncoder(&episode).Encode(expectedEpisode{Number: 1, Files: []string{"e.mkv"}}); err != nil {
		t.Fatal(err)
	}

	if got := strings.TrimSpace(episode.String()); got != `{"number":1,"files":["e.mkv"]}` {
		t.Errorf("episode JSON = %s", got)
	}
}

func TestWriteLibrarySummaryReportsWriteErrors(t *testing.T) {
	t.Parallel()

	output := testWriter{err: errors.New("disk full")}
	if err := writeLibrarySummary(&output, libraryManifest{}, "m"); err == nil {
		t.Error("writeLibrarySummary ignored the write error")
	}
}
