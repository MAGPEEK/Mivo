package enrich_test

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// fakeStore is an in-memory MetadataStore for one item.
type fakeStore struct {
	snapshots   map[string]enrich.ItemSnapshot
	snapshotErr error
	saveErr     error

	snapshotCalls int
	matchStates   []enrich.MatchResult
	enrichments   []enrich.Enrichment
}

func (store *fakeStore) ItemSnapshot(_ context.Context, itemID string) (enrich.ItemSnapshot, error) {
	store.snapshotCalls++

	if store.snapshotErr != nil {
		return enrich.ItemSnapshot{}, store.snapshotErr
	}

	snapshot, ok := store.snapshots[itemID]
	if !ok {
		return enrich.ItemSnapshot{}, fmt.Errorf("item %s: %w", itemID, domain.ErrNotFound)
	}

	return snapshot, nil
}

func (store *fakeStore) SaveMatchState(_ context.Context, _ string, result enrich.MatchResult) error {
	if store.saveErr != nil {
		return store.saveErr
	}

	store.matchStates = append(store.matchStates, result)

	return nil
}

func (store *fakeStore) SaveEnrichment(_ context.Context, _ string, enrichment enrich.Enrichment) error {
	if store.saveErr != nil {
		return store.saveErr
	}

	store.enrichments = append(store.enrichments, enrichment)

	return nil
}

func (store *fakeStore) writes() int {
	return len(store.matchStates) + len(store.enrichments)
}

type fakeSettings struct {
	settings enrich.OnlineSettings
	err      error
}

func (settings fakeSettings) OnlineSettings(context.Context) (enrich.OnlineSettings, error) {
	return settings.settings, settings.err
}

// fakeTitles is the shared part of the movie, show and music fakes: canned candidates and
// details keyed by reference ID, an error per call kind (the offline fake sets
// domain.ErrUnavailable) and a record of every request.
type fakeTitles struct {
	candidates []match.Candidate
	details    map[string]enrich.Details
	searchErr  error
	detailsErr error

	detailsRequests []enrich.DetailsRequest
	searchCalls     int
}

func (titles *fakeTitles) search() ([]match.Candidate, error) {
	titles.searchCalls++

	return titles.candidates, titles.searchErr
}

func (titles *fakeTitles) lookUp(request enrich.DetailsRequest) (enrich.Details, error) {
	titles.detailsRequests = append(titles.detailsRequests, request)

	if titles.detailsErr != nil {
		return enrich.Details{}, titles.detailsErr
	}

	details, ok := titles.details[request.Ref.ID]
	if !ok {
		return enrich.Details{}, fmt.Errorf("details %s: %w", request.Ref.ID, domain.ErrNotFound)
	}

	return details, nil
}

func (titles *fakeTitles) calls() int {
	return titles.searchCalls + len(titles.detailsRequests)
}

type fakeMovies struct {
	fakeTitles
	queries []enrich.SearchQuery
}

func (movies *fakeMovies) Search(_ context.Context, query enrich.SearchQuery) ([]match.Candidate, error) {
	movies.queries = append(movies.queries, query)

	return movies.search()
}

func (movies *fakeMovies) Details(_ context.Context, request enrich.DetailsRequest) (enrich.Details, error) {
	return movies.lookUp(request)
}

type fakeShows struct {
	fakeTitles
	queries        []enrich.SearchQuery
	seasons        map[int]enrich.SeasonDetails
	seasonErr      error
	seasonRequests []enrich.SeasonRequest
}

func (shows *fakeShows) Search(_ context.Context, query enrich.SearchQuery) ([]match.Candidate, error) {
	shows.queries = append(shows.queries, query)

	return shows.search()
}

func (shows *fakeShows) Details(_ context.Context, request enrich.DetailsRequest) (enrich.Details, error) {
	return shows.lookUp(request)
}

func (shows *fakeShows) SeasonDetails(_ context.Context, request enrich.SeasonRequest) (enrich.SeasonDetails, error) {
	shows.seasonRequests = append(shows.seasonRequests, request)

	if shows.seasonErr != nil {
		return enrich.SeasonDetails{}, shows.seasonErr
	}

	season, ok := shows.seasons[request.Season]
	if !ok {
		return enrich.SeasonDetails{}, fmt.Errorf("season %d: %w", request.Season, domain.ErrNotFound)
	}

	return season, nil
}

func (shows *fakeShows) calls() int {
	return shows.fakeTitles.calls() + len(shows.seasonRequests)
}

type fakeMusic struct {
	fakeTitles
	queries []enrich.MusicQuery
}

func (music *fakeMusic) Search(_ context.Context, query enrich.MusicQuery) ([]match.Candidate, error) {
	music.queries = append(music.queries, query)

	return music.search()
}

func (music *fakeMusic) Details(_ context.Context, request enrich.DetailsRequest) (enrich.Details, error) {
	return music.lookUp(request)
}

// fakeDownloader stores every picture whose path is not in failures; cancel, when set, is
// called on the first fetch to simulate a shutdown mid-download.
type fakeDownloader struct {
	failures map[string]bool
	cancel   context.CancelFunc
	fetched  []enrich.ArtworkRef
}

func (downloader *fakeDownloader) Fetch(_ context.Context, ref enrich.ArtworkRef) (enrich.Stored, error) {
	downloader.fetched = append(downloader.fetched, ref)

	if downloader.cancel != nil {
		downloader.cancel()
	}

	if downloader.failures[ref.Path] {
		return enrich.Stored{}, fmt.Errorf("fetch %s: %w", ref.Path, domain.ErrUnavailable)
	}

	return enrich.Stored{DataPath: "artwork" + ref.Path, Size: 1}, nil
}
