package watch_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/access"
	appwatch "github.com/magpeek/mivo/internal/app/watch"
	"github.com/magpeek/mivo/internal/domain"
	domainaccess "github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/watch"
)

const (
	profileID      = "profile-1"
	otherProfileID = "profile-2"
)

// fixedNow is the time the fixture clock reports.
func fixedNow() time.Time {
	return time.Date(2026, 11, 6, 20, 0, 0, 0, time.UTC)
}

// fakeCatalog is the viewer-scoped catalog: hidden items answer ErrNotFound like unknown ones.
type fakeCatalog struct {
	targets  map[string]appwatch.Target
	episodes map[string][]string
	hidden   map[string]bool
	lookups  int
	listErr  error
	// listRestrictions records the restriction of every ListEpisodes call.
	listRestrictions []domainaccess.Restriction
}

func newFakeCatalog() *fakeCatalog {
	return &fakeCatalog{targets: map[string]appwatch.Target{}, episodes: map[string][]string{}, hidden: map[string]bool{}}
}

func (catalog *fakeCatalog) add(target appwatch.Target) {
	catalog.targets[target.ItemID] = target
}

// addEpisode adds an episode below its season and show.
func (catalog *fakeCatalog) addEpisode(showID, seasonID, episodeID string) {
	catalog.add(appwatch.Target{ItemID: episodeID, Kind: library.ItemKindEpisode, DurationSeconds: 2400, AttachKey: "tmdb:episode:" + episodeID})
	catalog.episodes[showID] = append(catalog.episodes[showID], episodeID)
	catalog.episodes[seasonID] = append(catalog.episodes[seasonID], episodeID)
}

func (catalog *fakeCatalog) LookupWatchTarget(_ context.Context, itemID string) (appwatch.Target, error) {
	catalog.lookups++

	target, ok := catalog.targets[itemID]
	if !ok || catalog.hidden[itemID] {
		return appwatch.Target{}, fmt.Errorf("item %q: %w", itemID, domain.ErrNotFound)
	}

	return target, nil
}

func (catalog *fakeCatalog) ListEpisodes(_ context.Context, containerID string, restriction domainaccess.Restriction) ([]appwatch.Target, error) {
	catalog.listRestrictions = append(catalog.listRestrictions, restriction)

	if catalog.listErr != nil {
		return nil, catalog.listErr
	}

	episodes := make([]appwatch.Target, 0, len(catalog.episodes[containerID]))
	for _, episodeID := range catalog.episodes[containerID] {
		episode := catalog.targets[episodeID]
		episode.DurationSeconds = 0
		episodes = append(episodes, episode)
	}

	return episodes, nil
}

type storedState struct {
	state     watch.State
	attachKey string
	updatedAt time.Time
}

// fakeStore keeps states per profile and item in memory and counts its calls.
type fakeStore struct {
	mutex             sync.Mutex
	rows              map[string]map[string]storedState
	episodes          map[string][]string
	updates           int
	stateReads        int
	countReads        int
	readIDs           [][]string
	countIDs          [][]string
	countRestrictions []domainaccess.Restriction
	failWith          error
	lastUpdateAt      time.Time
}

func newFakeStore(catalog *fakeCatalog) *fakeStore {
	return &fakeStore{rows: map[string]map[string]storedState{}, episodes: catalog.episodes}
}

func (store *fakeStore) UpdateStates(_ context.Context, update appwatch.StateUpdate) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	store.updates++
	store.lastUpdateAt = update.Now

	if store.failWith != nil {
		return store.failWith
	}

	if store.rows[update.ProfileID] == nil {
		store.rows[update.ProfileID] = map[string]storedState{}
	}

	for _, key := range update.Keys {
		previous := store.rows[update.ProfileID][key.ItemID]
		store.rows[update.ProfileID][key.ItemID] = storedState{state: update.Change(previous.state), attachKey: key.AttachKey, updatedAt: update.Now}
	}

	return nil
}

func (store *fakeStore) States(_ context.Context, profile string, itemIDs []string) (map[string]watch.State, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	store.stateReads++
	store.readIDs = append(store.readIDs, itemIDs)

	if store.failWith != nil {
		return nil, store.failWith
	}

	states := map[string]watch.State{}

	for _, itemID := range itemIDs {
		if row, ok := store.rows[profile][itemID]; ok {
			states[itemID] = row.state
		}
	}

	return states, nil
}

func (store *fakeStore) EpisodeCounts(_ context.Context, profile string, containerIDs []string, restriction domainaccess.Restriction) (map[string]appwatch.EpisodeCount, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	store.countRestrictions = append(store.countRestrictions, restriction)

	store.countReads++
	store.countIDs = append(store.countIDs, containerIDs)

	if store.failWith != nil {
		return nil, store.failWith
	}

	counts := map[string]appwatch.EpisodeCount{}

	for _, containerID := range containerIDs {
		episodes := store.episodes[containerID]
		if len(episodes) == 0 {
			continue
		}

		count := appwatch.EpisodeCount{Total: len(episodes)}
		for _, episodeID := range episodes {
			if !store.rows[profile][episodeID].state.Watched {
				count.Unwatched++
			}
		}

		counts[containerID] = count
	}

	return counts, nil
}

func (store *fakeStore) state(profile, itemID string) storedState {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	return store.rows[profile][itemID]
}

func (store *fakeStore) put(profile, itemID string, state watch.State) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if store.rows[profile] == nil {
		store.rows[profile] = map[string]storedState{}
	}

	store.rows[profile][itemID] = storedState{state: state}
}

type fixture struct {
	catalog *fakeCatalog
	store   *fakeStore
	service *appwatch.Service
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	catalog := newFakeCatalog()
	store := newFakeStore(catalog)

	service, err := appwatch.NewService(appwatch.Dependencies{
		Items:    catalog,
		Episodes: catalog,
		Store:    store,
		Clock:    fixedNow,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	return fixture{catalog: catalog, store: store, service: service}
}

func viewerContext(profile string) context.Context {
	return access.WithViewer(context.Background(), domainaccess.Viewer{AccountID: "account-1", ProfileID: profile})
}
