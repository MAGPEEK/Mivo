package libraryfs_test

import (
	"maps"
	"testing"

	"github.com/magpeek/mivo/internal/domain/library"
)

func TestWalkClassifiesFilesForTheLibraryKind(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)

	writeFiles(t, root, "Artist/Album/01 Song.flac", "Artist/Album/cover.jpg", "Artist/Album/clip.mkv", "Movie.mkv")

	tests := []struct {
		kind library.Kind
		want map[string]library.FileClass
	}{
		{kind: library.KindMusic, want: map[string]library.FileClass{
			"Artist/Album/01 Song.flac": library.FileClassAudio,
			"Artist/Album/cover.jpg":    library.FileClassImage,
		}},
		{kind: library.KindMovies, want: map[string]library.FileClass{
			"Artist/Album/cover.jpg": library.FileClassImage,
			"Artist/Album/clip.mkv":  library.FileClassVideo,
			"Movie.mkv":              library.FileClassVideo,
		}},
	}

	for _, test := range tests {
		got := map[string]library.FileClass{}

		_, err := fileSystem.Walk(t.Context(), root, test.kind, func(state library.FileState) error {
			got[state.RelativePath] = state.Class

			return nil
		})
		if err != nil {
			t.Fatalf("Walk(%s) error = %v", test.kind, err)
		}

		if !maps.Equal(got, test.want) {
			t.Errorf("Walk(%s) classes = %v, want %v", test.kind, got, test.want)
		}
	}
}
