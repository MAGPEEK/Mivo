package identify_test

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/app/identify"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

var errBoom = errors.New("boom")

// fakeStore is the in-memory catalog behind every store port of identify and behind
// enrich.MetadataStore, so a test can run the real Enricher after a use case.
type fakeStore struct {
	items    map[string]enrich.ItemSnapshot
	children map[string][]string
	// libraries maps a library ID to its online switch; matchable to its non-disabled items.
	libraries map[string]bool
	matchable map[string][]string
	artwork   map[string][]string

	saveErr     error
	artworkErr  error
	childrenErr error
	listErr     error

	changes []identify.MatchChange
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		items:     map[string]enrich.ItemSnapshot{},
		children:  map[string][]string{},
		libraries: map[string]bool{},
		matchable: map[string][]string{},
		artwork:   map[string][]string{},
	}
}

func (store *fakeStore) ItemSnapshot(_ context.Context, itemID string) (enrich.ItemSnapshot, error) {
	snapshot, ok := store.items[itemID]
	if !ok {
		return enrich.ItemSnapshot{}, fmt.Errorf("item %s: %w", itemID, domain.ErrNotFound)
	}

	return snapshot, nil
}

func (store *fakeStore) SaveMatchChange(_ context.Context, itemID string, change identify.MatchChange) error {
	if store.saveErr != nil {
		return store.saveErr
	}

	snapshot := store.items[itemID]
	snapshot.Record = change.Record
	snapshot.Locks = change.Locks
	snapshot.MatchState = change.Match.State
	snapshot.LinkedRef = change.Match.Ref
	store.items[itemID] = snapshot
	store.changes = append(store.changes, change)

	return nil
}

func (store *fakeStore) SaveMatchState(_ context.Context, itemID string, result enrich.MatchResult) error {
	snapshot := store.items[itemID]
	snapshot.MatchState = result.State
	store.items[itemID] = snapshot

	return nil
}

func (store *fakeStore) SaveEnrichment(_ context.Context, itemID string, enrichment enrich.Enrichment) error {
	snapshot := store.items[itemID]
	snapshot.Record = enrichment.Record
	snapshot.MatchState = enrichment.Match.State
	snapshot.LinkedRef = enrichment.Match.Ref
	store.items[itemID] = snapshot

	return nil
}

func (store *fakeStore) DeleteProviderArtwork(_ context.Context, itemID string) ([]string, error) {
	if store.artworkErr != nil {
		return nil, store.artworkErr
	}

	paths := store.artwork[itemID]
	delete(store.artwork, itemID)

	return paths, nil
}

func (store *fakeStore) SeasonsAndEpisodes(_ context.Context, showID string) ([]string, error) {
	return store.children[showID], store.childrenErr
}

func (store *fakeStore) LibraryOnlineEnabled(_ context.Context, libraryID string) (bool, error) {
	enabled, ok := store.libraries[libraryID]
	if !ok {
		return false, fmt.Errorf("library %s: %w", libraryID, domain.ErrNotFound)
	}

	return enabled, nil
}

func (store *fakeStore) MatchableItems(_ context.Context, libraryID string) ([]string, error) {
	return store.matchable[libraryID], store.listErr
}

// rescan does what a scan does to an item: it reads the local sources again and merges them
// into the record, respecting locks. A scan never changes the match state.
func (store *fakeStore) rescan(itemID string, local enrich.LocalRecords) {
	snapshot := store.items[itemID]
	snapshot.Local = local
	snapshot.Record = metadata.Merge(snapshot.Record, snapshot.Locks, []metadata.SourcedRecord{
		{Source: metadata.SourceNFO, Record: local.NFO},
		{Source: metadata.SourceFilename, Record: local.Filename},
	})
	store.items[itemID] = snapshot
}

// queued is one Enqueue call.
type queued struct {
	itemID string
	reason enrich.Reason
}

type fakeQueue struct {
	calls []queued
	// missing items answer domain.ErrNotFound; err fails every call.
	missing map[string]bool
	err     error
}

func (queue *fakeQueue) Enqueue(_ context.Context, itemID string, reason enrich.Reason) error {
	if queue.err != nil {
		return queue.err
	}

	if queue.missing[itemID] {
		return fmt.Errorf("item %s: %w", itemID, domain.ErrNotFound)
	}

	queue.calls = append(queue.calls, queued{itemID: itemID, reason: reason})

	return nil
}

func (queue *fakeQueue) itemIDs() []string {
	ids := make([]string, 0, len(queue.calls))
	for _, call := range queue.calls {
		ids = append(ids, call.itemID)
	}

	return ids
}

type fakeFiles struct {
	removed []string
	errs    map[string]error
}

func (files *fakeFiles) RemoveDataFile(_ context.Context, dataPath string) error {
	if err := files.errs[dataPath]; err != nil {
		return err
	}

	files.removed = append(files.removed, dataPath)

	return nil
}

type fakeSettings struct {
	settings enrich.OnlineSettings
	err      error
}

func (settings *fakeSettings) OnlineSettings(context.Context) (enrich.OnlineSettings, error) {
	return settings.settings, settings.err
}

// fakeLookup implements the movie, show and music lookups: canned candidates and details by
// reference ID, one error per call kind (the offline fake sets domain.ErrUnavailable) and a
// record of every request.
type fakeLookup struct {
	candidates []match.Candidate
	details    map[string]enrich.Details
	searchErr  error
	detailsErr error

	searches        []enrich.SearchQuery
	musicSearches   []enrich.MusicQuery
	detailsRequests []enrich.DetailsRequest
}

func (lookup *fakeLookup) Search(_ context.Context, query enrich.SearchQuery) ([]match.Candidate, error) {
	lookup.searches = append(lookup.searches, query)

	return slices.Clone(lookup.candidates), lookup.searchErr
}

func (lookup *fakeLookup) Details(_ context.Context, request enrich.DetailsRequest) (enrich.Details, error) {
	lookup.detailsRequests = append(lookup.detailsRequests, request)

	if lookup.detailsErr != nil {
		return enrich.Details{}, lookup.detailsErr
	}

	details, ok := lookup.details[request.Ref.ID]
	if !ok {
		return enrich.Details{}, fmt.Errorf("entry %s: %w", request.Ref.ID, domain.ErrNotFound)
	}

	return details, nil
}

func (lookup *fakeLookup) SeasonDetails(context.Context, enrich.SeasonRequest) (enrich.SeasonDetails, error) {
	return enrich.SeasonDetails{}, fmt.Errorf("no seasons: %w", domain.ErrNotFound)
}

func (lookup *fakeLookup) calls() int {
	return len(lookup.searches) + len(lookup.musicSearches) + len(lookup.detailsRequests)
}

// fakeMusic adapts fakeLookup to enrich.MusicLookup.
type fakeMusic struct {
	*fakeLookup
}

func (music fakeMusic) Search(_ context.Context, query enrich.MusicQuery) ([]match.Candidate, error) {
	music.musicSearches = append(music.musicSearches, query)

	return slices.Clone(music.candidates), music.searchErr
}

// harness wires a Service on fakes with every provider switched on.
type harness struct {
	store    *fakeStore
	queue    *fakeQueue
	files    *fakeFiles
	settings *fakeSettings
	movies   *fakeLookup
	shows    *fakeLookup
	music    *fakeLookup
}

func newHarness() *harness {
	return &harness{
		store:    newFakeStore(),
		queue:    &fakeQueue{missing: map[string]bool{}},
		files:    &fakeFiles{errs: map[string]error{}},
		settings: &fakeSettings{settings: enrich.OnlineSettings{Enabled: true, TMDB: true, MusicBrainz: true, Language: "en"}},
		movies:   &fakeLookup{details: map[string]enrich.Details{}},
		shows:    &fakeLookup{details: map[string]enrich.Details{}},
		music:    &fakeLookup{details: map[string]enrich.Details{}},
	}
}

func (h *harness) deps() identify.Deps {
	return identify.Deps{
		Items:     h.store,
		Matches:   h.store,
		Artwork:   h.store,
		Files:     h.files,
		Children:  h.store,
		Libraries: h.store,
		Queue:     h.queue,
		Settings:  h.settings,
		Movies:    h.movies,
		Shows:     h.shows,
		Music:     fakeMusic{h.music},
	}
}

func (h *harness) service() *identify.Service {
	return identify.NewService(h.deps())
}

// enricher returns the real Enricher on the same fakes.
func (h *harness) enricher() *enrich.Enricher {
	return enrich.NewEnricher(enrich.Deps{
		Store:    h.store,
		Settings: h.settings,
		Movies:   h.movies,
		Shows:    h.shows,
		Music:    fakeMusic{h.music},
	})
}

// addItem stores an item of an online library.
func (h *harness) addItem(itemID string, kind library.ItemKind, snapshot enrich.ItemSnapshot) {
	snapshot.Kind = kind
	snapshot.OnlineEnabled = true

	if snapshot.MatchState == "" {
		snapshot.MatchState = match.MatchStateUnmatched
	}

	h.store.items[itemID] = snapshot
}

func tmdbID(value string) metadata.ProviderID {
	return metadata.ProviderID{Provider: metadata.ProviderTMDB, Value: value}
}
