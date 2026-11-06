package enrich_test

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
)

// queueStart is the fake clock's start.
func queueStart() time.Time {
	return time.Date(2026, time.November, 5, 12, 0, 0, 0, time.UTC)
}

// fakeClock is a settable clock, safe for concurrent use.
type fakeClock struct {
	mutex sync.Mutex
	now   time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: queueStart()}
}

func (clock *fakeClock) Now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	return clock.now
}

func (clock *fakeClock) Advance(duration time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	clock.now = clock.now.Add(duration)
}

// failure is one MarkJobFailed call.
type failure struct {
	itemID    string
	message   string
	notBefore time.Time
}

// fakeJobs is an in-memory persisted queue with the semantics of adapters/sqlite: one job per
// item, leases, revisions. Errors set on it are returned by the matching method.
type fakeJobs struct {
	mutex sync.Mutex
	clock *fakeClock
	jobs  map[string]*enrich.Job
	order []string

	enqueueErr, lengthErr, claimErr, doneErr, failErr, releaseErr error

	done           []string
	failures       []failure
	released       []string
	releaseCtxErrs []error
	finishCtxErrs  []error // context errors seen by MarkJobDone and MarkJobFailed
	claimCalls     int
}

func newFakeJobs(clock *fakeClock) *fakeJobs {
	return &fakeJobs{clock: clock, jobs: map[string]*enrich.Job{}}
}

func (jobs *fakeJobs) Enqueue(_ context.Context, itemID string, reason enrich.Reason) error {
	jobs.mutex.Lock()
	defer jobs.mutex.Unlock()

	if jobs.enqueueErr != nil {
		return jobs.enqueueErr
	}

	now := jobs.clock.Now()

	if job, ok := jobs.jobs[itemID]; ok {
		job.Reason, job.Revision = reason, job.Revision+1
		if now.Before(job.NotBefore) {
			job.NotBefore = now
		}

		return nil
	}

	jobs.jobs[itemID] = &enrich.Job{ItemID: itemID, Reason: reason, NotBefore: now}
	jobs.order = append(jobs.order, itemID)

	return nil
}

func (jobs *fakeJobs) QueueLength(context.Context) (int, error) {
	jobs.mutex.Lock()
	defer jobs.mutex.Unlock()

	return len(jobs.jobs), jobs.lengthErr
}

func (jobs *fakeJobs) ClaimNextJob(_ context.Context, lease time.Duration) (enrich.Job, bool, error) {
	jobs.mutex.Lock()
	defer jobs.mutex.Unlock()

	jobs.claimCalls++

	if jobs.claimErr != nil {
		return enrich.Job{}, false, jobs.claimErr
	}

	now := jobs.clock.Now()

	for _, itemID := range jobs.order {
		job, ok := jobs.jobs[itemID]
		if !ok || job.NotBefore.After(now) || job.LeaseUntil.After(now) {
			continue
		}

		job.LeaseUntil = now.Add(lease)

		return *job, true, nil
	}

	return enrich.Job{}, false, nil
}

func (jobs *fakeJobs) MarkJobDone(ctx context.Context, claimed enrich.Job) error {
	jobs.mutex.Lock()
	defer jobs.mutex.Unlock()

	jobs.finishCtxErrs = append(jobs.finishCtxErrs, ctx.Err())

	if jobs.doneErr != nil {
		return jobs.doneErr
	}

	jobs.done = append(jobs.done, claimed.ItemID)

	job, ok := jobs.jobs[claimed.ItemID]
	if !ok {
		return nil
	}

	if job.Revision == claimed.Revision {
		jobs.remove(claimed.ItemID)

		return nil
	}

	job.LeaseUntil, job.Attempts = time.Time{}, 0

	return nil
}

func (jobs *fakeJobs) MarkJobFailed(ctx context.Context, claimed enrich.Job, message string, notBefore time.Time) error {
	jobs.mutex.Lock()
	defer jobs.mutex.Unlock()

	jobs.finishCtxErrs = append(jobs.finishCtxErrs, ctx.Err())

	if jobs.failErr != nil {
		return jobs.failErr
	}

	jobs.failures = append(jobs.failures, failure{itemID: claimed.ItemID, message: message, notBefore: notBefore})

	job, ok := jobs.jobs[claimed.ItemID]
	if !ok {
		return fmt.Errorf("job %s: %w", claimed.ItemID, domain.ErrNotFound)
	}

	job.LeaseUntil, job.Attempts, job.LastError = time.Time{}, job.Attempts+1, message
	if job.Revision == claimed.Revision {
		job.NotBefore = notBefore
	}

	return nil
}

func (jobs *fakeJobs) ReleaseJob(ctx context.Context, claimed enrich.Job) error {
	jobs.mutex.Lock()
	defer jobs.mutex.Unlock()

	jobs.releaseCtxErrs = append(jobs.releaseCtxErrs, ctx.Err())

	if jobs.releaseErr != nil {
		return jobs.releaseErr
	}

	jobs.released = append(jobs.released, claimed.ItemID)

	if job, ok := jobs.jobs[claimed.ItemID]; ok {
		job.LeaseUntil = time.Time{}
	}

	return nil
}

func (jobs *fakeJobs) remove(itemID string) {
	delete(jobs.jobs, itemID)
	jobs.order = slices.DeleteFunc(jobs.order, func(queued string) bool { return queued == itemID })
}

func (jobs *fakeJobs) job(itemID string) (enrich.Job, bool) {
	jobs.mutex.Lock()
	defer jobs.mutex.Unlock()

	job, ok := jobs.jobs[itemID]
	if !ok {
		return enrich.Job{}, false
	}

	return *job, true
}

func (jobs *fakeJobs) length() int {
	jobs.mutex.Lock()
	defer jobs.mutex.Unlock()

	return len(jobs.jobs)
}

// fakeEnricher answers every item with err; block, when set, makes EnrichItem wait for the
// context and report the running count, so tests can watch concurrency.
type fakeEnricher struct {
	mutex sync.Mutex
	err   error
	errs  map[string]error
	// outcome is what a successful EnrichItem returns; empty means matched.
	outcome    enrich.Outcome
	block      bool
	running    int
	maxRunning int
	started    chan string
	proceed    chan struct{}
	onEnrich   func()
	enriched   []string
}

func (enricher *fakeEnricher) EnrichItem(ctx context.Context, itemID string) (enrich.Outcome, error) {
	enricher.mutex.Lock()
	enricher.running++
	enricher.maxRunning = max(enricher.maxRunning, enricher.running)
	enricher.enriched = append(enricher.enriched, itemID)
	err, ok := enricher.errs[itemID]
	if !ok {
		err = enricher.err
	}
	enricher.mutex.Unlock()

	defer func() {
		enricher.mutex.Lock()
		enricher.running--
		enricher.mutex.Unlock()
	}()

	if enricher.started != nil {
		enricher.started <- itemID
	}

	if enricher.onEnrich != nil {
		enricher.onEnrich()
	}

	if enricher.proceed != nil {
		<-enricher.proceed
	}

	if enricher.block {
		<-ctx.Done()

		return enrich.OutcomeSkipped, fmt.Errorf("enrich %s: %w", itemID, ctx.Err())
	}

	return cmp.Or(enricher.outcome, enrich.OutcomeMatched), err
}

func (enricher *fakeEnricher) peak() int {
	enricher.mutex.Lock()
	defer enricher.mutex.Unlock()

	return enricher.maxRunning
}

// fakeCache is an in-memory ProviderCache.
type fakeCache struct {
	entries  map[string]enrich.CacheEntry
	readErr  error
	writeErr error
	writes   int
}

func newFakeCache() *fakeCache {
	return &fakeCache{entries: map[string]enrich.CacheEntry{}}
}

func (cache *fakeCache) ReadProviderCache(_ context.Context, provider, key string) (enrich.CacheEntry, error) {
	if cache.readErr != nil {
		return enrich.CacheEntry{}, cache.readErr
	}

	entry, ok := cache.entries[provider+"/"+key]
	if !ok {
		return enrich.CacheEntry{}, fmt.Errorf("cache %s: %w", key, domain.ErrNotFound)
	}

	return entry, nil
}

func (cache *fakeCache) WriteProviderCache(_ context.Context, provider, key string, entry enrich.CacheEntry) error {
	cache.writes++

	if cache.writeErr != nil {
		return cache.writeErr
	}

	cache.entries[provider+"/"+key] = entry

	return nil
}

// only returns the single cached entry.
func (cache *fakeCache) only() (string, enrich.CacheEntry, bool) {
	if len(cache.entries) != 1 {
		return "", enrich.CacheEntry{}, false
	}

	for key, entry := range cache.entries {
		return key, entry, true
	}

	return "", enrich.CacheEntry{}, false
}

// fakeRefreshSource returns canned due items and records the cutoff it was asked for.
type fakeRefreshSource struct {
	itemIDs []string
	err     error
	cutoffs []time.Time
	limits  []int
}

func (source *fakeRefreshSource) ItemsDueForRefresh(_ context.Context, cutoff time.Time, limit int) ([]string, error) {
	source.cutoffs = append(source.cutoffs, cutoff)
	source.limits = append(source.limits, limit)

	return source.itemIDs, source.err
}

// retryAfterErr is a rate-limit error that carries the provider's Retry-After.
type retryAfterErr struct{ after time.Duration }

func (err retryAfterErr) Error() string             { return "rate limited, retry later" }
func (err retryAfterErr) Unwrap() error             { return domain.ErrRateLimited }
func (err retryAfterErr) RetryAfter() time.Duration { return err.after }
