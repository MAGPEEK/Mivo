package sqlite_test

import (
	"errors"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

func TestArtworkReturnsTheStoredLocationOfEverySource(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	movie := newItemSpec(testID('m', 1), moviesLibraryID, "movie", "Movie")
	insertItem(t, fixture.inspection, movie)
	insertFile(t, fixture.inspection, fileSpec{
		id: testID('f', 1), itemID: movie.id, libraryID: moviesLibraryID,
		relativePath: "Movie/Movie.mkv",
	})
	insertLocalArtwork(t, fixture.inspection, artworkSpec{
		id: testID('a', 1), itemID: movie.id, kind: "poster",
		libraryID: moviesLibraryID, relativePath: "Movie/poster.jpg", tag: "taglocal", selected: true,
	})
	execute(t, fixture.inspection, `INSERT INTO artwork (id, item_id, kind, source, media_file_id, stream_index,
		size_bytes, modified_at, tag, created_at) VALUES (?, ?, 'thumb', 'embedded', ?, 2, 10, ?, 'tagembedded', 0)`,
		testID('a', 2), movie.id, testID('f', 1), fileModifiedMilli)
	execute(t, fixture.inspection, `INSERT INTO artwork (id, item_id, kind, source, data_path, size_bytes,
		modified_at, tag, created_at) VALUES (?, ?, 'backdrop', 'upload', 'artwork/ab/abcd.jpg', 30, ?, 'tagupload', 0)`,
		testID('a', 3), movie.id, fileModifiedMilli)

	modified := time.UnixMilli(fileModifiedMilli).UTC()
	tests := []struct {
		id   string
		want artwork.Stored
	}{
		{testID('a', 1), artwork.Stored{
			ID: testID('a', 1), Kind: artwork.KindPoster, Source: artwork.SourceLocal, LibraryRoot: moviesRoot,
			Location: artwork.LocalLocation("Movie/poster.jpg"), Size: 2048, ModifiedAt: modified, Tag: "taglocal",
		}},
		{testID('a', 2), artwork.Stored{
			ID: testID('a', 2), Kind: artwork.KindThumb, Source: artwork.SourceEmbedded, LibraryRoot: moviesRoot,
			Location: artwork.EmbeddedLocation("Movie/Movie.mkv", 2), Size: 10, ModifiedAt: modified, Tag: "tagembedded",
		}},
		{testID('a', 3), artwork.Stored{
			ID: testID('a', 3), Kind: artwork.KindBackdrop, Source: artwork.SourceUpload,
			Location: artwork.DataLocation("artwork/ab/abcd.jpg"), Size: 30, ModifiedAt: modified, Tag: "tagupload",
		}},
	}

	for _, test := range tests {
		got, err := fixture.reader.Artwork(t.Context(), test.id)
		if err != nil {
			t.Fatalf("Artwork(%q) error = %v", test.id, err)
		}

		assertDeepEqual(t, "Artwork("+test.id+")", got, test.want)
	}

	tags, err := fixture.reader.ArtworkTags(t.Context())
	if err != nil {
		t.Fatalf("ArtworkTags error = %v", err)
	}

	assertDeepEqual(t, "tags", tags, map[string]string{
		testID('a', 1): "taglocal", testID('a', 2): "tagembedded", testID('a', 3): "tagupload",
	})
}

func TestArtworkOfAnUnknownIDIsNotFound(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)

	if _, err := fixture.reader.Artwork(t.Context(), unknownID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Artwork error = %v, want domain.ErrNotFound", err)
	}

	tags, err := fixture.reader.ArtworkTags(t.Context())
	if err != nil || len(tags) != 0 {
		t.Errorf("ArtworkTags = %v, %v, want an empty map", tags, err)
	}
}

func TestArtworkWithAnUnknownKindIsAServerError(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	movie := newItemSpec(testID('m', 1), moviesLibraryID, "movie", "Movie")
	insertItem(t, fixture.inspection, movie)
	insertLocalArtwork(t, fixture.inspection, artworkSpec{
		id: testID('a', 1), itemID: movie.id, kind: "hologram",
		libraryID: moviesLibraryID, relativePath: "Movie/x.jpg", tag: "tag",
	})

	_, err := fixture.reader.Artwork(t.Context(), testID('a', 1))
	assertUndecodableRowError(t, "Artwork", err)
}
