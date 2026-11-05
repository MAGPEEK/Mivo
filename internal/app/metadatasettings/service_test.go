package metadatasettings_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/app/metadatasettings"
	"github.com/magpeek/mivo/internal/domain"
)

const (
	ownKey     = "own-secret-key"
	bundledKey = "bundled-secret-key"
)

// fakeStore records saved states and fails on demand.
type fakeStore struct {
	mutex  sync.Mutex
	saved  []metadatasettings.State
	failed error
}

func (store *fakeStore) SaveMetadataSettings(_ context.Context, state metadatasettings.State) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if store.failed != nil {
		return store.failed
	}

	store.saved = append(store.saved, state)

	return nil
}

type countingListener struct{ calls int }

func (listener *countingListener) SettingsChanged() { listener.calls++ }

func initialState() metadatasettings.State {
	return metadatasettings.State{Online: true, TMDB: true, MusicBrainz: true}
}

func newService(store metadatasettings.Store, listener metadatasettings.ChangeListener, bundled string) *metadatasettings.Service {
	return metadatasettings.NewService(metadatasettings.Options{
		Initial: initialState(), DefaultLanguage: "en", BundledTMDBKey: bundled, Store: store, Listener: listener,
	})
}

func pointer[T any](value T) *T { return &value }

func TestSettingsShowTheKeyStatusOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		bundled string
		own     *string
		want    metadatasettings.KeyStatus
		wantKey string
	}{
		{name: "no key", want: metadatasettings.KeyNone},
		{name: "bundled", bundled: bundledKey, want: metadatasettings.KeyBundled, wantKey: bundledKey},
		{name: "own wins", bundled: bundledKey, own: pointer(ownKey), want: metadatasettings.KeyOwn, wantKey: ownKey},
		{name: "own without bundled", own: pointer(ownKey), want: metadatasettings.KeyOwn, wantKey: ownKey},
		{name: "removed own falls back", bundled: bundledKey, own: pointer(""), want: metadatasettings.KeyBundled, wantKey: bundledKey},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			service := serviceWithOwnKey(t, test.bundled, test.own)

			settings, err := service.Settings(t.Context())
			if err != nil || settings.TMDBKey != test.want {
				t.Errorf("Settings = %+v, %v, want key status %q", settings, err, test.want)
			}

			if key, err := service.APIKey(t.Context()); err != nil || key != test.wantKey {
				t.Errorf("APIKey = %q, %v, want %q", key, err, test.wantKey)
			}
		})
	}
}

// serviceWithOwnKey returns a service with the bundled key and, unless own is nil, the own
// key set through Update.
func serviceWithOwnKey(t *testing.T, bundled string, own *string) *metadatasettings.Service {
	t.Helper()

	service := newService(nil, nil, bundled)
	if own == nil {
		return service
	}

	if _, err := service.Update(t.Context(), metadatasettings.Change{TMDBKey: own}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	return service
}

func TestUpdateIsEffectiveForTheNextReader(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	listener := &countingListener{}
	service := newService(store, listener, "")

	got, err := service.Update(t.Context(), metadatasettings.Change{
		Online: pointer(false), MusicBrainz: pointer(false), Language: pointer("de-de"),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	want := metadatasettings.Settings{Online: false, TMDB: true, MusicBrainz: false, Language: "de-DE", TMDBKey: metadatasettings.KeyNone}
	if got != want {
		t.Errorf("Update = %+v, want %+v", got, want)
	}

	online, err := service.OnlineSettings(t.Context())
	if err != nil || online != (enrich.OnlineSettings{Enabled: false, TMDB: true, MusicBrainz: false, Language: "de-DE"}) {
		t.Errorf("OnlineSettings = %+v, %v", online, err)
	}

	if len(store.saved) != 1 || store.saved[0].Language != "de-DE" || store.saved[0].Online || listener.calls != 1 {
		t.Errorf("saved = %+v, listener calls = %d", store.saved, listener.calls)
	}
}

func TestUpdateWithoutEffectStoresNothing(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	listener := &countingListener{}
	service := newService(store, listener, "")

	if _, err := service.Update(t.Context(), metadatasettings.Change{Online: pointer(true), Language: pointer("")}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	if len(store.saved) != 0 || listener.calls != 0 {
		t.Errorf("saved = %+v, listener calls = %d, want none", store.saved, listener.calls)
	}

	settings, _ := service.Settings(t.Context())
	if settings.Language != "en" {
		t.Errorf("language = %q, want the default en", settings.Language)
	}
}

func TestUpdateRejectsMalformedValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		change metadatasettings.Change
		field  metadatasettings.Field
	}{
		{"language", metadatasettings.Change{Language: pointer("german")}, metadatasettings.FieldLanguage},
		{"key with a space", metadatasettings.Change{TMDBKey: pointer("abc def")}, metadatasettings.FieldTMDBKey},
		{"key with a newline", metadatasettings.Change{TMDBKey: pointer("abc\nX-Header: 1")}, metadatasettings.FieldTMDBKey},
		{"key too long", metadatasettings.Change{TMDBKey: pointer(strings.Repeat("k", metadatasettings.MaxKeyLength+1))}, metadatasettings.FieldTMDBKey},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			store := &fakeStore{}
			service := newService(store, nil, "")

			_, err := service.Update(t.Context(), test.change)

			var fieldErr *metadatasettings.FieldError
			if !errors.Is(err, domain.ErrInvalid) || !errors.As(err, &fieldErr) || fieldErr.Field != test.field {
				t.Fatalf("Update error = %v, want a %s FieldError", err, test.field)
			}

			if strings.Contains(err.Error(), "abc") {
				t.Errorf("error %q quotes the key", err)
			}

			if len(store.saved) != 0 {
				t.Errorf("saved = %+v, want nothing", store.saved)
			}
		})
	}
}

func TestUpdateKeepsTheSettingsWhenTheStoreFails(t *testing.T) {
	t.Parallel()

	store := &fakeStore{failed: errors.New("disk full")}
	listener := &countingListener{}
	service := newService(store, listener, "")

	if _, err := service.Update(t.Context(), metadatasettings.Change{Online: pointer(false)}); err == nil {
		t.Fatal("Update succeeded, want the store error")
	}

	online, _ := service.OnlineSettings(t.Context())
	if !online.Enabled || listener.calls != 0 {
		t.Errorf("OnlineSettings = %+v, listener calls = %d; want unchanged", online, listener.calls)
	}
}

func TestServiceIsSafeForConcurrentUse(t *testing.T) {
	t.Parallel()

	service := newService(&fakeStore{}, nil, bundledKey)

	var group sync.WaitGroup
	for index := range 20 {
		group.Go(func() {
			_, _ = service.Update(context.Background(), metadatasettings.Change{Online: pointer(index%2 == 0)})
			_, _ = service.OnlineSettings(context.Background())
			_, _ = service.APIKey(context.Background())
		})
	}

	group.Wait()
}
