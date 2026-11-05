package library_test

import (
	"reflect"
	"testing"

	"github.com/magpeek/mivo/internal/domain/library"
)

func TestKindTopLevelItemKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind library.Kind
		want []library.ItemKind
	}{
		{library.KindMovies, []library.ItemKind{library.ItemKindMovie}},
		{library.KindShows, []library.ItemKind{library.ItemKindShow}},
		{library.KindMusic, []library.ItemKind{library.ItemKindArtist, library.ItemKindAlbum}},
		{library.KindAudiobooks, []library.ItemKind{library.ItemKindAudiobook}},
		{library.KindPhotos, nil},
	}
	for _, test := range tests {
		if got := test.kind.TopLevelItemKinds(); !reflect.DeepEqual(got, test.want) {
			t.Errorf("%q.TopLevelItemKinds() = %v, want %v", test.kind, got, test.want)
		}
	}

	for kind, want := range map[library.ItemKind]bool{
		library.ItemKindMovie: true, library.ItemKindAlbum: true, library.ItemKindAudiobook: true,
		library.ItemKindEpisode: false, library.ItemKindTrack: false, library.ItemKindPhoto: false,
	} {
		if got := library.IsTopLevelItemKind(kind); got != want {
			t.Errorf("IsTopLevelItemKind(%q) = %v, want %v", kind, got, want)
		}
	}
}
