package libraryfs_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/fs/libraryfs"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

const corpusSubtitleRoot = "../../../../testdata/library/subs"

// subtitlePaths returns the paths of one video's subtitles.
func subtitlePaths(subtitles []media.ExternalSubtitle) []string {
	paths := make([]string, 0, len(subtitles))
	for _, subtitle := range subtitles {
		paths = append(paths, subtitle.RelativePath)
	}

	return paths
}

func TestListExternalSubtitlesReadsTheCorpus(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(corpusSubtitleRoot)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(root); err != nil {
		t.Fatalf("corpus missing, run `go run ./scripts/gen-testmedia -force`: %v", err)
	}

	fileSystem, _ := newFileSystem(t)

	got, err := fileSystem.ListExternalSubtitles(t.Context(), root, "", []string{"Movie (2020)"})
	if err != nil {
		t.Fatalf("ListExternalSubtitles() error = %v", err)
	}

	subtitles := got["Movie (2020)"]
	want := []string{
		"Movie (2020).de.forced.srt", "Movie (2020).en.srt", "Movie (2020).fr.sdh.ass", "Subs/Movie (2020).es.srt",
	}

	if !slices.Equal(subtitlePaths(subtitles), want) {
		t.Fatalf("paths = %v, want %v", subtitlePaths(subtitles), want)
	}

	german, french := subtitles[0], subtitles[2]
	if german.Language != "de" || !german.Forced || french.Language != "fr" || !french.SDH ||
		french.Format != media.SubtitleFormatASS {
		t.Errorf("parsed german = %+v, french = %+v", german, french)
	}
}

func TestListExternalSubtitlesPlacesPathsBelowTheFolder(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "Movies/Heat/Heat.mkv", "Movies/Heat/Heat.en.srt", "Movies/Heat/Subtitles/Heat.de.srt",
		"Movies/Heat/Deep/Subs/Heat.fr.srt", "Movies/Heat/Heat.txt", "Movies/Heat/Other.en.srt", "Movies/Heat/.Heat.nl.srt")

	got, err := fileSystem.ListExternalSubtitles(t.Context(), root, "Movies/Heat", []string{"Heat"})
	if err != nil {
		t.Fatalf("ListExternalSubtitles() error = %v", err)
	}

	want := []string{"Movies/Heat/Heat.en.srt", "Movies/Heat/Subtitles/Heat.de.srt"}
	if !slices.Equal(subtitlePaths(got["Heat"]), want) {
		t.Errorf("paths = %v, want %v", subtitlePaths(got["Heat"]), want)
	}

	if got["Heat"][0].TrackID() == got["Heat"][1].TrackID() {
		t.Error("two files got the same track id")
	}
}

func TestListExternalSubtitlesGivesAFileToTheLongestBase(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "Movie - 4K.en.srt", "Movie.de.srt", "Movie - 4K.mkv")

	got, err := fileSystem.ListExternalSubtitles(t.Context(), root, "", []string{"Movie", "Movie - 4K"})
	if err != nil {
		t.Fatalf("ListExternalSubtitles() error = %v", err)
	}

	if paths := subtitlePaths(got["Movie - 4K"]); !slices.Equal(paths, []string{"Movie - 4K.en.srt"}) {
		t.Errorf("Movie - 4K = %v", paths)
	}

	if paths := subtitlePaths(got["Movie"]); !slices.Equal(paths, []string{"Movie.de.srt"}) {
		t.Errorf("Movie = %v", paths)
	}
}

func TestListExternalSubtitlesNormalisesNamesToNFC(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	decomposed := "Amélie (2001).fr.srt"
	writeFiles(t, root, decomposed)

	got, err := fileSystem.ListExternalSubtitles(t.Context(), root, "", []string{"Amélie (2001)"})
	if err != nil {
		t.Fatalf("ListExternalSubtitles() error = %v", err)
	}

	if paths := subtitlePaths(got["Amélie (2001)"]); !slices.Equal(paths, []string{"Amélie (2001).fr.srt"}) {
		t.Errorf("paths = %v, want the NFC name", paths)
	}
}

func TestListExternalSubtitlesKeepsAVobSubPairAsOneTrack(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "Movie.en.idx", "Movie.en.sub", "Movie.de.idx")

	got, err := fileSystem.ListExternalSubtitles(t.Context(), root, "", []string{"Movie"})
	if err != nil {
		t.Fatalf("ListExternalSubtitles() error = %v", err)
	}

	if paths := subtitlePaths(got["Movie"]); !slices.Equal(paths, []string{"Movie.en.idx"}) {
		t.Errorf("paths = %v", paths)
	}
}

func TestListExternalSubtitlesLimitsCountAndSize(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)

	for position := range libraryfs.MaxSubtitlesPerVideo + 6 {
		writeFiles(t, root, fmt.Sprintf("Movie.part%03d.srt", position))
	}

	writeFile(t, filepath.Join(root, "Big.en.srt"), strings.Repeat("x", int(libraryfs.MaxSubtitleBytes)+1))
	writeFile(t, filepath.Join(root, "Edge.en.srt"), strings.Repeat("x", int(libraryfs.MaxSubtitleBytes)))

	got, err := fileSystem.ListExternalSubtitles(t.Context(), root, "", []string{"Movie", "Big", "Edge"})
	if err != nil {
		t.Fatalf("ListExternalSubtitles() error = %v", err)
	}

	if len(got["Movie"]) != libraryfs.MaxSubtitlesPerVideo {
		t.Errorf("Movie has %d subtitles, want %d", len(got["Movie"]), libraryfs.MaxSubtitlesPerVideo)
	}

	if len(got["Big"]) != 0 || len(got["Edge"]) != 1 {
		t.Errorf("Big = %d, Edge = %d subtitles, want 0 and 1", len(got["Big"]), len(got["Edge"]))
	}
}

func TestListExternalSubtitlesIgnoresLinksLeavingTheRoot(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, outside := newLibrary(t)
	writeFile(t, filepath.Join(outside, "Movie.en.srt"), "1\n")
	writeFile(t, filepath.Join(outside, "Subs", "Movie.de.srt"), "1\n")
	writeFiles(t, root, "Movie.fr.srt", "Real/Movie.es.srt")
	symlinkOrSkip(t, filepath.Join(outside, "Movie.en.srt"), filepath.Join(root, "Movie.en.srt"))
	symlinkOrSkip(t, filepath.Join("..", "outside", "Movie.en.srt"), filepath.Join(root, "Movie.it.srt"))
	symlinkOrSkip(t, filepath.Join("..", "outside", "Subs"), filepath.Join(root, "Subs"))

	got, err := fileSystem.ListExternalSubtitles(t.Context(), root, "", []string{"Movie"})
	if err != nil {
		t.Fatalf("ListExternalSubtitles() error = %v", err)
	}

	if paths := subtitlePaths(got["Movie"]); !slices.Equal(paths, []string{"Movie.fr.srt"}) {
		t.Errorf("paths = %v, want only the file inside the root", paths)
	}
}

func TestListExternalSubtitlesRefusesBadFolders(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "Movies/file.mkv")

	tests := []struct {
		folder string
		want   error
	}{
		{folder: "../outside", want: domain.ErrForbidden},
		{folder: "Missing", want: domain.ErrNotFound},
		{folder: "Movies/file.mkv", want: domain.ErrNotFound},
	}
	for _, test := range tests {
		if _, err := fileSystem.ListExternalSubtitles(t.Context(), root, test.folder, []string{"x"}); !errors.Is(err, test.want) {
			t.Errorf("folder %q error = %v, want %v", test.folder, err, test.want)
		}
	}
}

func TestListExternalSubtitlesStopsOnACancelledContext(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "Subs/Movie.en.srt")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := fileSystem.ListExternalSubtitles(ctx, root, "", []string{"Movie"}); err == nil {
		t.Error("error = nil, want the context error")
	}
}
