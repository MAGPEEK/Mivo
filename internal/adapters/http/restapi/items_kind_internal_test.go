package restapi

import (
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

// Every generated ItemKind value passes the type check exactly when some library kind lists it
// at its top (movies, shows, music, audiobooks); the use case then checks the library.
func TestCheckItemKindAcceptsTheTopLevelKindsOfTheContract(t *testing.T) {
	t.Parallel()

	topLevel := map[apigen.ItemKind]bool{
		apigen.ItemKindMovie: true, apigen.ItemKindShow: true, apigen.ItemKindArtist: true,
		apigen.ItemKindAlbum: true, apigen.ItemKindAudiobook: true,
		apigen.ItemKindSeason: false, apigen.ItemKindEpisode: false, apigen.ItemKindExtra: false, apigen.ItemKindTrack: false,
	}

	for kind, want := range topLevel {
		if !kind.Valid() {
			t.Errorf("%q is not a generated ItemKind value", kind)
		}

		if got := checkItemKind(&kind) == nil; got != want {
			t.Errorf("checkItemKind(%q) accepted = %v, want %v", kind, got, want)
		}
	}
}
