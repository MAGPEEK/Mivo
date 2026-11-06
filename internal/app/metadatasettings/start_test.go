package metadatasettings_test

import (
	"context"
	"testing"

	"github.com/magpeek/mivo/internal/app/metadatasettings"
)

func TestStartState(t *testing.T) {
	t.Parallel()

	stored := metadatasettings.State{Online: true, Language: "de", OwnTMDBKey: "ui-key"}

	if got := metadatasettings.StartState("file-key", stored, true); got != stored {
		t.Errorf("StartState(stored) = %+v, want the stored state", got)
	}

	want := metadatasettings.State{Online: true, TMDB: true, MusicBrainz: true, OwnTMDBKey: "file-key"}
	if got := metadatasettings.StartState("file-key", metadatasettings.State{}, false); got != want {
		t.Errorf("StartState(nothing stored) = %+v, want %+v", got, want)
	}
}

// recordingStore keeps the last saved state.
type recordingStore struct {
	saved metadatasettings.State
}

func (store *recordingStore) SaveMetadataSettings(_ context.Context, state metadatasettings.State) error {
	store.saved = state

	return nil
}

func TestForcedOffSwitchesStayOffAndAreNeverStored(t *testing.T) {
	t.Parallel()

	store := &recordingStore{}
	service := metadatasettings.NewService(metadatasettings.Options{
		Initial:   metadatasettings.StartState("", metadatasettings.State{}, false),
		ForcedOff: metadatasettings.ForcedOff{Online: true, MusicBrainz: true},
		Store:     store,
	})

	settings, err := service.Settings(t.Context())
	if err != nil || settings.Online || settings.MusicBrainz || !settings.TMDB {
		t.Fatalf("Settings() = %+v, %v; want online and musicbrainz forced off", settings, err)
	}

	language := "de"
	if _, err := service.Update(t.Context(), metadatasettings.Change{Language: &language}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if !store.saved.Online || !store.saved.MusicBrainz || store.saved.Language != "de" {
		t.Errorf("stored %+v, want the user's switches (on) with the new language", store.saved)
	}

	online, _ := service.OnlineSettings(t.Context())
	if online.Enabled || online.MusicBrainz {
		t.Errorf("OnlineSettings() = %+v, want the forced switches still off", online)
	}
}

// readingListener reads the settings while it is told about a change, which deadlocks if
// the service still holds its lock.
type readingListener struct {
	service *metadatasettings.Service
	read    metadatasettings.Settings
}

func (listener *readingListener) SettingsChanged() {
	listener.read, _ = listener.service.Settings(context.Background())
}

func TestListenerIsCalledOutsideTheLock(t *testing.T) {
	t.Parallel()

	listener := &readingListener{}
	service := metadatasettings.NewService(metadatasettings.Options{
		Initial: metadatasettings.State{Online: true}, Listener: listener,
	})
	listener.service = service

	off := false
	if _, err := service.Update(t.Context(), metadatasettings.Change{Online: &off}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if listener.read.Online {
		t.Errorf("listener read %+v, want the new state", listener.read)
	}
}
