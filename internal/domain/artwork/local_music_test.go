package artwork_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/domain/artwork"
)

func TestClassifyLocalAlbumFolder(t *testing.T) {
	t.Parallel()

	runClassifyCases(t, artwork.LocalContext{Kind: artwork.ContextAlbumFolder}, []classifyCase{
		{file: "cover.jpg", want: poster(0), found: true},
		{file: "Folder.PNG", want: poster(1), found: true},
		{file: "front.webp", want: poster(2), found: true},
		{file: "poster.jpg"},
		{file: "artist.jpg"},
		{file: "back.jpg"},
	})
}

func TestClassifyLocalArtistFolder(t *testing.T) {
	t.Parallel()

	runClassifyCases(t, artwork.LocalContext{Kind: artwork.ContextArtistFolder}, []classifyCase{
		{file: "artist.jpg", want: poster(0), found: true},
		{file: "cover.jpg"},
		{file: "folder.jpg"},
	})
}
