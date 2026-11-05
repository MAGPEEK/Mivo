package enrich_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// TestOfflineLibraryKeepsLocalData runs the real Enricher behind the caching lookups against a
// TMDB fake that is always unreachable: every item keeps its local data, every job is
// rescheduled with backoff, the status reports TMDB offline and no call errors to the caller.
func TestOfflineLibraryKeepsLocalData(t *testing.T) {
	t.Parallel()

	const libraryItems = 20

	h := newWorkerHarness()
	cache := newFakeCache()
	store := &fakeStore{snapshots: map[string]enrich.ItemSnapshot{}}
	movies := &fakeMovies{fakeTitles: fakeTitles{searchErr: errUnavailable(), detailsErr: errUnavailable()}}
	settings := fakeSettings{settings: enrich.OnlineSettings{Enabled: true, TMDB: true, MusicBrainz: true, Language: "en"}}
	deps := enrich.CacheDeps{Cache: cache, Health: h.health, Clock: h.clock.Now}

	enricher := enrich.NewEnricher(enrich.Deps{
		Store: store, Settings: settings, Movies: enrich.NewCachedMovies(movies, deps),
	})

	for index := range libraryItems {
		itemID := fmt.Sprintf("movie-%d", index)
		snapshot := movieSnapshot()
		snapshot.Record = metadata.Record{Title: fmt.Sprintf("Local Title %d", index), Year: 2000 + index}
		store.snapshots[itemID] = snapshot
		mustEnqueue(t, h.queue, itemID)
	}

	worker := enrich.NewWorker(enrich.WorkerDeps{Jobs: h.jobs, Enricher: enricher, Health: h.health, Clock: h.clock.Now},
		enrich.WorkerOptions{})
	runUntilIdle(t, worker)

	if store.writes() != 0 || cache.writes != 0 {
		t.Fatalf("store writes = %d, cache writes = %d; want the local data untouched", store.writes(), cache.writes)
	}

	if h.jobs.length() != libraryItems || len(h.jobs.failures) != libraryItems {
		t.Fatalf("queued %d, failures %d; want every job kept for a retry", h.jobs.length(), len(h.jobs.failures))
	}

	assertRetriedLater(t, h.jobs.failures)
	assertTMDBOffline(t, h, settings)
}

func runUntilIdle(t *testing.T, worker *enrich.Worker) {
	t.Helper()

	for {
		worked, err := worker.RunOnce(context.Background())
		if err != nil {
			t.Fatalf("RunOnce: %v", err)
		}

		if !worked {
			return
		}
	}
}

func assertRetriedLater(t *testing.T, failures []failure) {
	t.Helper()

	for _, failed := range failures {
		if !failed.notBefore.Equal(queueStart().Add(enrich.BaseBackoff)) {
			t.Fatalf("%s due %v, want after the first backoff", failed.itemID, failed.notBefore)
		}
	}
}

func assertTMDBOffline(t *testing.T, h *workerHarness, settings fakeSettings) {
	t.Helper()

	queue := enrich.NewQueue(enrich.QueueDeps{Jobs: h.jobs, Health: h.health, Settings: settings})

	status, err := queue.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	if status.Providers[0] != (enrich.ProviderStatus{Provider: enrich.ProviderTMDB, State: enrich.ProviderStateOffline}) {
		t.Fatalf("providers = %+v, want TMDB offline", status.Providers)
	}
}
