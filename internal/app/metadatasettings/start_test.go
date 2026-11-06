package metadatasettings_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/app/metadatasettings"
)

func TestStartState(t *testing.T) {
	t.Parallel()

	allOn := metadatasettings.State{Online: true, TMDB: true, MusicBrainz: true}
	stored := metadatasettings.State{Online: true, TMDB: true, Language: "de", OwnTMDBKey: "ui-key"}

	tests := []struct {
		name       string
		configured metadatasettings.State
		stored     metadatasettings.State
		found      bool
		want       metadatasettings.State
	}{
		{
			name:       "nothing stored",
			configured: metadatasettings.State{Online: true, OwnTMDBKey: "file-key"},
			want:       metadatasettings.State{Online: true, OwnTMDBKey: "file-key"},
		},
		{name: "stored wins", configured: allOn, stored: stored, found: true, want: stored},
		{
			name: "configured off stays off", configured: metadatasettings.State{TMDB: true, MusicBrainz: true},
			stored: allOn, found: true, want: metadatasettings.State{TMDB: true, MusicBrainz: true},
		},
		{
			name: "configured provider off", configured: metadatasettings.State{Online: true, MusicBrainz: true},
			stored: allOn, found: true, want: metadatasettings.State{Online: true, MusicBrainz: true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := metadatasettings.StartState(test.configured, test.stored, test.found); got != test.want {
				t.Errorf("StartState() = %+v, want %+v", got, test.want)
			}
		})
	}
}
