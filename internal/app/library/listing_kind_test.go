package library_test

import (
	"testing"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/library"
)

func TestListItemsChecksTheKindPerLibraryKind(t *testing.T) {
	t.Parallel()

	const (
		musicID      = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
		audiobooksID = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
		photosID     = "ffffffff-ffff-4fff-8fff-ffffffffffff"
	)

	tests := []struct {
		name      string
		libraryID string
		kind      library.ItemKind
		wantKind  library.ItemKind
		wantErr   bool
	}{
		{name: "music default", libraryID: musicID, wantKind: library.ItemKindArtist},
		{name: "music artist", libraryID: musicID, kind: library.ItemKindArtist, wantKind: library.ItemKindArtist},
		{name: "music album", libraryID: musicID, kind: library.ItemKindAlbum, wantKind: library.ItemKindAlbum},
		{name: "music movie", libraryID: musicID, kind: library.ItemKindMovie, wantErr: true},
		{name: "audiobooks default", libraryID: audiobooksID, wantKind: library.ItemKindAudiobook},
		{name: "audiobooks album", libraryID: audiobooksID, kind: library.ItemKindAlbum, wantErr: true},
		{name: "photos default", libraryID: photosID, wantKind: library.ItemKindPhotoAlbum},
		{name: "photos timeline", libraryID: photosID, kind: library.ItemKindPhoto, wantKind: library.ItemKindPhoto},
		{name: "photos movie", libraryID: photosID, kind: library.ItemKindMovie, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := newBrowseEnvironment()
			environment.store.put(
				library.ManagedLibrary{ID: musicID, Name: "Music", Kind: library.KindMusic},
				library.ManagedLibrary{ID: audiobooksID, Name: "Books", Kind: library.KindAudiobooks},
				library.ManagedLibrary{ID: photosID, Name: "Photos", Kind: library.KindPhotos},
			)

			query := library.ListingQuery{Filter: library.ListingFilter{Kind: test.kind}}

			_, err := environment.service(t).ListItems(t.Context(), test.libraryID, library.PageRequest{}, query)
			if test.wantErr {
				assertWrappedError(t, err, applibrary.ErrItemKindMismatch, "library")

				return
			}

			if err != nil {
				t.Fatalf("ListItems error = %v", err)
			}

			if got := environment.items.recordedQueries(); len(got) != 1 || got[0].Filter.Kind != test.wantKind {
				t.Errorf("reader queries = %+v, want kind %q", got, test.wantKind)
			}
		})
	}
}
