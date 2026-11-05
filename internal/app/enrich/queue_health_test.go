package enrich_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
)

func TestHealthObserve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		start error // observed first; nil leaves the provider ok
		err   error
		want  enrich.ProviderState
	}{
		{name: "success", err: nil, want: enrich.ProviderStateOK},
		{name: "offline", err: errUnavailable(), want: enrich.ProviderStateOffline},
		{name: "rate limited", err: domain.ErrRateLimited, want: enrich.ProviderStateRateLimited},
		{name: "invalid key", err: errInvalidKey, want: enrich.ProviderStateDisabled},
		{name: "recovers", start: errUnavailable(), err: nil, want: enrich.ProviderStateOK},
		{name: "not found is reachable", start: errUnavailable(), err: domain.ErrNotFound, want: enrich.ProviderStateOK},
		{name: "cancel tells nothing", start: errUnavailable(), err: fmt.Errorf("%w: %w", domain.ErrUnavailable, context.Canceled), want: enrich.ProviderStateOffline},
		{name: "unknown error tells nothing", start: domain.ErrRateLimited, err: errors.New("decode"), want: enrich.ProviderStateRateLimited},
		{name: "disabled stays disabled", start: errInvalidKey, err: nil, want: enrich.ProviderStateDisabled},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			health := enrich.NewHealth(enrich.HealthOptions{IsInvalidKey: func(err error) bool { return errors.Is(err, errInvalidKey) }})
			if test.start != nil {
				health.Observe(enrich.ProviderTMDB, test.start)
			}

			health.Observe(enrich.ProviderTMDB, test.err)

			if got := health.State(enrich.ProviderTMDB); got != test.want {
				t.Fatalf("state = %s, want %s", got, test.want)
			}

			if got := health.State(enrich.ProviderMusicBrainz); got != enrich.ProviderStateOK {
				t.Fatalf("other provider = %s, want ok", got)
			}
		})
	}
}

func TestHealthWithoutKeyCheckNeverDisables(t *testing.T) {
	t.Parallel()

	health := enrich.NewHealth(enrich.HealthOptions{})
	health.Observe(enrich.ProviderTMDB, errInvalidKey)

	if !health.Usable(enrich.ProviderTMDB) || health.State(enrich.ProviderTMDB) != enrich.ProviderStateOffline {
		t.Fatalf("state = %s, want offline and usable", health.State(enrich.ProviderTMDB))
	}
}

// Constructors given no Health use a private one instead of failing.
func TestNilHealthDefaultsToAPrivateOne(t *testing.T) {
	t.Parallel()

	clock := newFakeClock()
	jobs := newFakeJobs(clock)
	queue := enrich.NewQueue(enrich.QueueDeps{Jobs: jobs})
	mustEnqueue(t, queue, "item-a")

	worker := enrich.NewWorker(enrich.WorkerDeps{Jobs: jobs, Enricher: &fakeEnricher{}, Clock: clock.Now}, enrich.WorkerOptions{})
	if worked, err := worker.RunOnce(context.Background()); !worked || err != nil {
		t.Fatalf("RunOnce = %v, %v; want the job done", worked, err)
	}

	if _, err := queue.Status(context.Background()); err != nil {
		t.Fatalf("Status: %v", err)
	}

	movies := enrich.NewCachedMovies(&fakeMovies{}, enrich.CacheDeps{Cache: newFakeCache()})
	if _, err := movies.Search(context.Background(), enrich.SearchQuery{Title: "Heat"}); err != nil {
		t.Fatalf("Search: %v", err)
	}

	observed := enrich.NewObservedArtwork(&fakeDownloader{}, nil)
	if _, err := observed.Fetch(context.Background(), enrich.ArtworkRef{Provider: enrich.ArtworkProviderTMDB, Path: "/p.jpg"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
}

func TestSchedulerRefreshDue(t *testing.T) {
	t.Parallel()

	jobs := newFakeJobs(newFakeClock())
	queue := enrich.NewQueue(enrich.QueueDeps{Jobs: jobs, Health: enrich.NewHealth(enrich.HealthOptions{})})
	source := &fakeRefreshSource{itemIDs: []string{"item-a", "item-b"}}
	now := queueStart()

	queued, err := enrich.NewScheduler(source, queue).RefreshDue(context.Background(), now)
	if err != nil {
		t.Fatalf("RefreshDue: %v", err)
	}

	if queued != 2 || !source.cutoffs[0].Equal(now.Add(-enrich.CacheTTL)) || source.limits[0] <= 0 {
		t.Fatalf("queued %d with cutoff %v limit %d; want 2 with cutoff now - TTL", queued, source.cutoffs[0], source.limits[0])
	}

	job, ok := jobs.job("item-b")
	if !ok || job.Reason != enrich.ReasonPeriodic {
		t.Fatalf("job = %+v, want a periodic job", job)
	}
}

func TestSchedulerRefreshDueErrors(t *testing.T) {
	t.Parallel()

	jobs := newFakeJobs(newFakeClock())
	queue := enrich.NewQueue(enrich.QueueDeps{Jobs: jobs, Health: enrich.NewHealth(enrich.HealthOptions{})})

	source := &fakeRefreshSource{err: errQueueStore}
	if _, err := enrich.NewScheduler(source, queue).RefreshDue(context.Background(), queueStart()); !errors.Is(err, errQueueStore) {
		t.Fatalf("source failure: err = %v, want it wrapped", err)
	}

	jobs.enqueueErr = fmt.Errorf("item: %w", domain.ErrNotFound)
	source = &fakeRefreshSource{itemIDs: []string{"deleted"}}

	queued, err := enrich.NewScheduler(source, queue).RefreshDue(context.Background(), queueStart())
	if err != nil || queued != 0 {
		t.Fatalf("deleted item: RefreshDue = %d, %v; want it skipped", queued, err)
	}

	jobs.enqueueErr = errQueueStore

	if _, err := enrich.NewScheduler(source, queue).RefreshDue(context.Background(), queueStart()); !errors.Is(err, errQueueStore) {
		t.Fatalf("enqueue failure: err = %v, want it wrapped", err)
	}
}
