package libraryfs_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/fs/libraryfs"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// walkStates walks root and returns every reported state by relative path.
func walkStates(t *testing.T, fileSystem *libraryfs.FileSystem, root string) map[string]library.FileState {
	t.Helper()

	states := map[string]library.FileState{}

	if _, err := fileSystem.Walk(t.Context(), root, library.KindMovies, func(state library.FileState) error {
		states[state.RelativePath] = state
		return nil
	}); err != nil {
		t.Fatalf("Walk error = %v", err)
	}

	return states
}

func TestWalkReportsDiscFoldersAsOneEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files []string
		want  map[string]media.Layout
	}{
		{
			name:  "dvd",
			files: []string{"Movie (2019)/VIDEO_TS/VIDEO_TS.IFO", "Movie (2019)/VIDEO_TS/VTS_01_1.VOB"},
			want:  map[string]media.Layout{"Movie (2019)/VIDEO_TS": media.LayoutVideoTS},
		},
		{
			name:  "dvd lower case",
			files: []string{"Movie/video_ts/video_ts.ifo", "Movie/video_ts/vts_01_1.vob"},
			want:  map[string]media.Layout{"Movie/video_ts": media.LayoutVideoTS},
		},
		{
			name:  "bluray",
			files: []string{"Movie/BDMV/index.bdmv", "Movie/BDMV/STREAM/00000.m2ts"},
			want:  map[string]media.Layout{"Movie/BDMV": media.LayoutBDMV},
		},
		{
			name:  "bluray mixed case",
			files: []string{"Movie/Bdmv/INDEX.BDMV", "Movie/Bdmv/Stream/00000.m2ts"},
			want:  map[string]media.Layout{"Movie/Bdmv": media.LayoutBDMV},
		},
		{
			name:  "disc folder directly in the root",
			files: []string{"BDMV/index.bdmv", "BDMV/STREAM/00000.m2ts"},
			want:  map[string]media.Layout{"BDMV": media.LayoutBDMV},
		},
		{
			name: "nested disc beside extras, nfo and poster",
			files: []string{
				"Movies/Film (2001)/BDMV/index.bdmv", "Movies/Film (2001)/BDMV/STREAM/0.m2ts",
				"Movies/Film (2001)/movie.nfo", "Movies/Film (2001)/poster.jpg",
				"Movies/Film (2001)/Extras/Making of.mkv",
			},
			want: map[string]media.Layout{
				"Movies/Film (2001)/BDMV":                 media.LayoutBDMV,
				"Movies/Film (2001)/movie.nfo":            "",
				"Movies/Film (2001)/poster.jpg":           "",
				"Movies/Film (2001)/Extras/Making of.mkv": "",
			},
		},
		{
			name:  "folder without index is walked normally",
			files: []string{"Movie/BDMV/STREAM/00000.m2ts", "Movie/VIDEO_TS/clip.mkv"},
			want:  map[string]media.Layout{"Movie/BDMV/STREAM/00000.m2ts": "", "Movie/VIDEO_TS/clip.mkv": ""},
		},
		{
			name:  "iso beside nfo and poster",
			files: []string{"Film (2001).ISO", "Film (2001).nfo", "Film (2001)-poster.jpg"},
			want: map[string]media.Layout{
				"Film (2001).ISO": media.LayoutISO, "Film (2001).nfo": "", "Film (2001)-poster.jpg": "",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fileSystem, _ := newFileSystem(t)
			root, _ := newLibrary(t)
			writeFiles(t, root, test.files...)

			got := walkStates(t, fileSystem, root)
			if len(got) != len(test.want) {
				t.Errorf("reported %d entries %v, want %d", len(got), got, len(test.want))
			}

			for relativePath, layout := range test.want {
				state, ok := got[relativePath]
				if !ok || state.Layout != layout {
					t.Errorf("%s: reported = %v, layout = %q, want %q", relativePath, ok, state.Layout, layout)
				}
			}
		})
	}
}

func TestWalkDiscUsesTheIndexFileSizeAndClassVideo(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFile(t, filepath.Join(root, "M", "BDMV", "index.bdmv"), "12345")
	writeFile(t, filepath.Join(root, "M", "BDMV", "STREAM", "0.m2ts"), "123456789")

	state := walkStates(t, fileSystem, root)["M/BDMV"]
	if state.Size != int64(len("12345")) || state.Class != library.FileClassVideo || state.ModifiedAt.IsZero() {
		t.Errorf("state = %+v, want index size, video class and a time", state)
	}
}

func TestWalkSkipsEmptyIso(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFile(t, filepath.Join(root, "empty.iso"), "")

	got, skipped := walkWithSkipped(t, fileSystem, root)
	if len(got) != 0 {
		t.Errorf("paths = %v, want none", got)
	}

	assertPaths(t, skipped, "empty.iso")
}

func TestWalkIgnoresIndexThatIsADirectory(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	mkdir(t, filepath.Join(root, "M", "BDMV", "index.bdmv"))
	writeFiles(t, root, "M/BDMV/clip.mkv")

	assertPaths(t, walkPaths(t, fileSystem, root), "M/BDMV/clip.mkv")
}

func TestWalkIgnoresIsoAndIndexLinksOutsideTheRoot(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, outside := newLibrary(t)
	writeFile(t, filepath.Join(outside, "big.iso"), "x")
	writeFile(t, filepath.Join(outside, "index.bdmv"), "x")
	symlinkOrSkip(t, filepath.Join(outside, "big.iso"), filepath.Join(root, "big.iso"))
	symlinkOrSkip(t, filepath.Join(outside, "index.bdmv"), filepath.Join(root, "M", "BDMV", "index.bdmv"))

	if got := walkPaths(t, fileSystem, root); len(got) != 0 {
		t.Errorf("paths = %v, want none", got)
	}
}

// A link to a disc folder, whatever its name, is the same disc: only the real path is reported
// and the inner files are not walked through the link.
func TestWalkReportsADiscOnceForLinksToIt(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "Real/Disc/VIDEO_TS/VIDEO_TS.IFO", "Real/Disc/VIDEO_TS/VTS_01_1.VOB")
	symlinkOrSkip(t, filepath.Join("..", "Real", "Disc", "VIDEO_TS"), filepath.Join(root, "Linked", "VIDEO_TS"))
	symlinkOrSkip(t, filepath.Join("..", "Real", "Disc", "VIDEO_TS"), filepath.Join(root, "Linked", "Alias"))

	got := walkPaths(t, fileSystem, root)
	assertPaths(t, got, "Real/Disc/VIDEO_TS")
}

func TestWalkReturnsTheVisitorErrorForADisc(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "M/BDMV/index.bdmv")

	stop := errors.New("stop")

	_, err := fileSystem.Walk(t.Context(), root, library.KindMovies, func(library.FileState) error { return stop })
	if !errors.Is(err, stop) {
		t.Errorf("Walk error = %v, want the visitor error", err)
	}
}
