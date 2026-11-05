package corpustest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/domain/media"
)

const (
	// libraryCorpusDirectory is the generated library tree (M3-17, M3-13f) seen from here.
	libraryCorpusDirectory = "../../../testdata/library"
	// libraryManifestSchemaVersion is the library manifest version this test understands.
	libraryManifestSchemaVersion = 1
	// Cover sources of the music manifest.
	coverSourceEmbedded = "embedded"
	coverSourceFolder   = "folder"
)

// jpegSignature starts every JPEG file.
func jpegSignature() []byte { return []byte{0xFF, 0xD8, 0xFF} }

// libraryManifest holds the music part of testdata/library/manifest.json and the case
// results; scripts/gen-testmedia is package main, so the shape is repeated.
type libraryManifest struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Music         musicManifest       `json:"music"`
	Cases         []libraryCaseResult `json:"cases"`
}

type musicManifest struct {
	Path   string          `json:"path"`
	Albums []musicAlbumRow `json:"albums"`
}

type musicAlbumRow struct {
	AlbumArtist string          `json:"albumArtist"`
	Title       string          `json:"title"`
	Year        int             `json:"year"`
	Compilation bool            `json:"compilation"`
	Tracks      []musicTrackRow `json:"tracks"`
}

type musicTrackRow struct {
	File        string `json:"file"`
	Artist      string `json:"artist"`
	Album       string `json:"album"`
	Disc        int    `json:"disc"`
	Track       int    `json:"track"`
	Title       string `json:"title"`
	Tagged      bool   `json:"tagged"`
	CoverSource string `json:"coverSource"`
	CoverFile   string `json:"coverFile"`
}

type libraryCaseResult struct {
	Name   string   `json:"name"`
	Status string   `json:"status"`
	Reason string   `json:"reason"`
	Files  []string `json:"files"`
}

// musicReaders are the adapters under test.
type musicReaders struct {
	prober    *ffmpeg.Prober
	extractor *ffmpeg.CoverExtractor
}

// TestCorpusMusicTagsAreReadable checks every music file of the library manifest: it exists,
// FFmpeg reads the tags the manifest names (none for untagged files) and the cover is where
// the manifest says.
func TestCorpusMusicTagsAreReadable(t *testing.T) {
	t.Parallel()

	readers, root := requireMusicCorpus(t)
	manifest := readLibraryManifest(t, root)

	if len(manifest.Music.Albums) == 0 {
		t.Fatal("the library manifest lists no music album; regenerate it with `go run ./scripts/gen-testmedia`")
	}

	results := caseResultsByFile(manifest.Cases)
	musicRoot := filepath.Join(root, filepath.FromSlash(manifest.Music.Path))

	for _, album := range manifest.Music.Albums {
		for _, track := range album.Tracks {
			t.Run(track.File, func(t *testing.T) {
				t.Parallel()

				result := results[path.Join(manifest.Music.Path, track.File)]
				if result.Status != statusGenerated {
					t.Skipf("case %s was not generated: %s", result.Name, result.Reason)
				}

				file := filepath.Join(musicRoot, filepath.FromSlash(track.File))
				readers.assertTags(t, file, wantedTags(album, track))
				readers.assertCover(t, musicRoot, track)
			})
		}
	}
}

func requireMusicCorpus(t *testing.T) (musicReaders, string) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping the corpus music integration test in -short mode")
	}

	ffmpegPath, ffmpegErr := exec.LookPath("ffmpeg")
	ffprobePath, ffprobeErr := exec.LookPath("ffprobe")

	if ffmpegErr != nil || ffprobeErr != nil {
		t.Skip("skipping the corpus music integration test: ffmpeg or ffprobe is not on PATH " +
			"(install FFmpeg, see docs/development.md)")
	}

	root, err := filepath.Abs(libraryCorpusDirectory)
	if err != nil {
		t.Fatalf("resolve the library directory: %v", err)
	}

	logger := slog.New(slog.DiscardHandler)

	prober, err := ffmpeg.NewProber(ffmpeg.ProberOptions{FFprobePath: ffprobePath, Logger: logger})
	if err != nil {
		t.Fatalf("NewProber: %v", err)
	}

	extractor, err := ffmpeg.NewCoverExtractor(ffmpeg.CoverExtractorOptions{
		FFmpegPath: ffmpegPath, FFprobePath: ffprobePath, Logger: logger,
	})
	if err != nil {
		t.Fatalf("NewCoverExtractor: %v", err)
	}

	return musicReaders{prober: prober, extractor: extractor}, root
}

func readLibraryManifest(t *testing.T, root string) libraryManifest {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(root, manifestFileName)) //nolint:gosec // fixed corpus path
	if errors.Is(err, fs.ErrNotExist) {
		t.Skip("skipping the corpus music integration test: testdata/library/manifest.json is missing " +
			"(generate it with `go run ./scripts/gen-testmedia`)")
	}

	if err != nil {
		t.Fatalf("read the library manifest: %v", err)
	}

	var manifest libraryManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode the library manifest: %v", err)
	}

	if manifest.SchemaVersion != libraryManifestSchemaVersion {
		t.Fatalf("the library manifest has schema version %d, want %d; regenerate it with `go run ./scripts/gen-testmedia`",
			manifest.SchemaVersion, libraryManifestSchemaVersion)
	}

	return manifest
}

// caseResultsByFile maps every written file to the result of the case that writes it.
func caseResultsByFile(cases []libraryCaseResult) map[string]libraryCaseResult {
	results := map[string]libraryCaseResult{}

	for _, result := range cases {
		for _, file := range result.Files {
			results[file] = result
		}
	}

	return results
}

// wantedTags are the tags a tagged track carries; an untagged track carries none.
func wantedTags(album musicAlbumRow, track musicTrackRow) media.AudioTags {
	if !track.Tagged {
		return media.AudioTags{}
	}

	return media.AudioTags{
		AlbumArtist: album.AlbumArtist, Artist: track.Artist, Album: track.Album, Title: track.Title,
		Disc: track.Disc, Track: track.Track, Year: album.Year, Compilation: album.Compilation,
	}
}

func (readers musicReaders) assertTags(t *testing.T, file string, want media.AudioTags) {
	t.Helper()

	tags, duration, err := readers.prober.ReadAudioTags(t.Context(), file)
	if err != nil {
		t.Fatalf("ReadAudioTags: %v", err)
	}

	if tags != want {
		t.Errorf("tags = %+v, want %+v", tags, want)
	}

	if duration <= 0 {
		t.Errorf("duration = %v, want a positive length", duration)
	}
}

// assertCover checks the embedded picture, the folder image or that the file has no cover.
func (readers musicReaders) assertCover(t *testing.T, musicRoot string, track musicTrackRow) {
	t.Helper()

	file := filepath.Join(musicRoot, filepath.FromSlash(track.File))
	cover, err := readers.extractor.ExtractCover(t.Context(), file)

	switch track.CoverSource {
	case coverSourceEmbedded:
		if err != nil || !bytes.HasPrefix(cover, jpegSignature()) {
			t.Errorf("embedded cover: %d bytes, error %v; want a JPEG", len(cover), err)
		}
	case coverSourceFolder:
		assertJPEGFile(t, filepath.Join(musicRoot, filepath.FromSlash(track.CoverFile)))

		if !errors.Is(err, ffmpeg.ErrNoCover) {
			t.Errorf("ExtractCover error = %v, want ffmpeg.ErrNoCover next to a folder cover", err)
		}
	default:
		if !errors.Is(err, ffmpeg.ErrNoCover) {
			t.Errorf("ExtractCover error = %v, want ffmpeg.ErrNoCover", err)
		}
	}
}

func assertJPEGFile(t *testing.T, file string) {
	t.Helper()

	data, err := os.ReadFile(file) //nolint:gosec // path from the generated corpus manifest
	if err != nil || !bytes.HasPrefix(data, jpegSignature()) {
		t.Errorf("folder cover %s: error %v; want a JPEG file", filepath.Base(file), err)
	}
}
