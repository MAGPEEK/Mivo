package sqlite_test

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/domain"
)

const (
	testLease   = 5 * time.Minute
	testBackoff = time.Hour
)

func formatMillis(moment time.Time) string {
	return strconv.FormatInt(moment.UnixMilli(), 10)
}

func (fixture enrichmentFixture) enqueue(t *testing.T, itemID string, reason sqlite.EnrichmentReason) {
	t.Helper()

	if err := fixture.store.Enqueue(t.Context(), itemID, reason); err != nil {
		t.Fatalf("Enqueue(%s, %s) error = %v", itemID, reason, err)
	}
}

func (fixture enrichmentFixture) claim(t *testing.T) (sqlite.EnrichmentJob, bool) {
	t.Helper()

	job, ok, err := fixture.store.ClaimNextJob(t.Context(), testLease)
	if err != nil {
		t.Fatalf("ClaimNextJob error = %v", err)
	}

	return job, ok
}

func (fixture enrichmentFixture) queueLength(t *testing.T) int {
	t.Helper()

	length, err := fixture.store.QueueLength(t.Context())
	if err != nil {
		t.Fatalf("QueueLength error = %v", err)
	}

	return length
}

func TestEnqueueKeepsOneJobPerItemWithTheNewestReason(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	fixture.enqueue(t, "item-1", sqlite.EnrichmentReasonScan)
	fixture.advance(time.Second)
	fixture.enqueue(t, "show-1", sqlite.EnrichmentReasonPeriodic)
	fixture.enqueue(t, "item-1", sqlite.EnrichmentReasonIdentify)

	if got := fixture.queueLength(t); got != 2 {
		t.Errorf("QueueLength = %d, want 2", got)
	}

	job, ok := fixture.claim(t)
	want := sqlite.EnrichmentJob{
		ItemID: "item-1", Reason: sqlite.EnrichmentReasonIdentify, NotBefore: enrichmentStart(), Revision: 1,
		LeaseUntil: enrichmentStart().Add(time.Second + testLease),
	}

	if !ok || job != want {
		t.Errorf("first claim = %+v, %t, want %+v", job, ok, want)
	}

	if job, ok = fixture.claim(t); !ok || job.ItemID != "show-1" {
		t.Errorf("second claim = %+v, %t, want show-1", job, ok)
	}

	if job, ok = fixture.claim(t); ok {
		t.Errorf("third claim = %+v, want none (both jobs leased)", job)
	}
}

func TestEnqueueRejectsUnknownReasonsAndItems(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)

	if err := fixture.store.Enqueue(t.Context(), "item-1", "manual"); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("Enqueue with an unknown reason error = %v, want ErrInvalid", err)
	}

	err := fixture.store.Enqueue(t.Context(), "item-9", sqlite.EnrichmentReasonScan)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Enqueue of an unknown item error = %v, want ErrNotFound", err)
	}

	// The driver's constraint text stays out of the message (review M6-03).
	if err != nil && strings.Contains(strings.ToUpper(err.Error()), "FOREIGN KEY") {
		t.Errorf("Enqueue of an unknown item error = %q, want no driver detail", err)
	}

	if _, _, err := fixture.store.ClaimNextJob(t.Context(), 0); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("ClaimNextJob without a lease error = %v, want ErrInvalid", err)
	}
}

func TestMarkJobDoneRemovesTheJob(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	fixture.enqueue(t, "item-1", sqlite.EnrichmentReasonScan)

	job, _ := fixture.claim(t)
	if err := fixture.store.MarkJobDone(t.Context(), job); err != nil {
		t.Fatalf("MarkJobDone error = %v", err)
	}

	if got := fixture.queueLength(t); got != 0 {
		t.Errorf("QueueLength after done = %d, want 0", got)
	}
}

// A request that comes in while the job runs is not swallowed by finishing the old one.
func TestMarkJobDoneKeepsARequestThatCameInWhileTheJobRan(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	fixture.enqueue(t, "item-1", sqlite.EnrichmentReasonScan)

	job, _ := fixture.claim(t)
	fixture.enqueue(t, "item-1", sqlite.EnrichmentReasonIdentify)

	if err := fixture.store.MarkJobDone(t.Context(), job); err != nil {
		t.Fatalf("MarkJobDone error = %v", err)
	}

	next, ok := fixture.claim(t)
	if !ok || next.Reason != sqlite.EnrichmentReasonIdentify || next.Revision != job.Revision+1 {
		t.Errorf("claim after done = %+v, %t, want the identify request", next, ok)
	}
}

func TestMarkJobFailedBacksOffAndCountsAttempts(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	fixture.enqueue(t, "item-1", sqlite.EnrichmentReasonScan)

	job, _ := fixture.claim(t)
	if err := fixture.store.MarkJobFailed(t.Context(), job, "tmdb unavailable", enrichmentStart().Add(testBackoff)); err != nil {
		t.Fatalf("MarkJobFailed error = %v", err)
	}

	if job, ok := fixture.claim(t); ok {
		t.Errorf("claim during the backoff = %+v, want none", job)
	}

	fixture.advance(testBackoff)

	retry, ok := fixture.claim(t)
	want := sqlite.EnrichmentJob{
		ItemID: "item-1", Reason: sqlite.EnrichmentReasonScan, NotBefore: enrichmentStart().Add(testBackoff),
		Attempts: 1, LastError: "tmdb unavailable", LeaseUntil: enrichmentStart().Add(testBackoff + testLease),
	}

	if !ok || retry != want {
		t.Errorf("claim after the backoff = %+v, %t, want %+v", retry, ok, want)
	}
}

func TestMarkJobFailedKeepsTheDueTimeOfANewerRequest(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	fixture.enqueue(t, "item-1", sqlite.EnrichmentReasonScan)

	job, _ := fixture.claim(t)
	fixture.enqueue(t, "item-1", sqlite.EnrichmentReasonRefresh)

	if err := fixture.store.MarkJobFailed(t.Context(), job, "offline", enrichmentStart().Add(testBackoff)); err != nil {
		t.Fatalf("MarkJobFailed error = %v", err)
	}

	if next, ok := fixture.claim(t); !ok || next.Reason != sqlite.EnrichmentReasonRefresh || next.Attempts != 1 {
		t.Errorf("claim after the failure = %+v, %t, want the refresh request due now", next, ok)
	}
}

func TestMarkJobFailedReportsAVanishedJob(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	fixture.enqueue(t, "item-1", sqlite.EnrichmentReasonScan)

	job, _ := fixture.claim(t)
	execute(t, fixture.inspection, "DELETE FROM items WHERE id = 'item-1'")

	if err := fixture.store.MarkJobFailed(t.Context(), job, "x", enrichmentStart()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("MarkJobFailed of a deleted item error = %v, want ErrNotFound", err)
	}

	if err := fixture.store.MarkJobDone(t.Context(), job); err != nil {
		t.Errorf("MarkJobDone of a deleted item error = %v, want nil", err)
	}
}

// The queue lives in the database: jobs survive a restart, and a job whose worker died is
// claimable again once the lease ran out.
func TestQueueSurvivesReopeningAndExpiredLeasesAreReclaimable(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	fixture.enqueue(t, "item-1", sqlite.EnrichmentReasonScan)
	fixture.enqueue(t, "show-1", sqlite.EnrichmentReasonScan)

	claimed, _ := fixture.claim(t)

	if err := fixture.db.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}

	reopened, err := sqlite.Open(t.Context(), sqlite.Options{Path: fixture.path})
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}

	t.Cleanup(func() { _ = reopened.Close() })

	now := enrichmentStart()
	store := sqlite.NewEnrichmentStore(reopened, func() time.Time { return now })

	var claimedItems []string

	for _, at := range []time.Time{enrichmentStart(), enrichmentStart(), enrichmentStart().Add(testLease)} {
		now = at

		job, ok, err := store.ClaimNextJob(t.Context(), testLease)
		if err != nil {
			t.Fatalf("ClaimNextJob error = %v", err)
		}

		if ok {
			claimedItems = append(claimedItems, job.ItemID)
		}
	}

	if want := []string{"show-1", claimed.ItemID}; !slices.Equal(claimedItems, want) {
		t.Errorf("claims after reopening = %q, want %q (the orphan only after its lease)", claimedItems, want)
	}
}

// A worker whose lease ran out and whose job another worker claimed must not finish or
// reset that job (review M6-03).
func TestStaleWorkerCannotFinishAJobAnotherWorkerClaimed(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	fixture.enqueue(t, "item-1", sqlite.EnrichmentReasonScan)

	stale, _ := fixture.claim(t)
	fixture.advance(testLease)

	owner, ok := fixture.claim(t)
	if !ok || owner.ItemID != stale.ItemID {
		t.Fatalf("claim after the lease = %+v, %t, want the orphaned job", owner, ok)
	}

	if err := fixture.store.MarkJobDone(t.Context(), stale); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("stale MarkJobDone error = %v, want ErrConflict", err)
	}

	if err := fixture.store.MarkJobFailed(t.Context(), stale, "x", enrichmentStart()); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("stale MarkJobFailed error = %v, want ErrConflict", err)
	}

	leases := queryStrings(t, fixture.inspection, "SELECT claimed_until || '|' || attempts FROM enrichment_queue")
	if want := []string{formatMillis(owner.LeaseUntil) + "|0"}; !slices.Equal(leases, want) {
		t.Errorf("job after the stale calls = %q, want %q (owned and untouched)", leases, want)
	}

	if err := fixture.store.MarkJobDone(t.Context(), owner); err != nil {
		t.Errorf("owner MarkJobDone error = %v", err)
	}

	if got := fixture.queueLength(t); got != 0 {
		t.Errorf("QueueLength after the owner finished = %d, want 0", got)
	}
}

// claimUntilEmpty claims jobs until none is due and returns their items.
func claimUntilEmpty(t *testing.T, store *sqlite.EnrichmentStore) []string {
	t.Helper()

	var itemIDs []string

	for {
		job, ok, err := store.ClaimNextJob(t.Context(), testLease)
		if err != nil {
			t.Errorf("ClaimNextJob error = %v", err)
		}

		if !ok {
			return itemIDs
		}

		itemIDs = append(itemIDs, job.ItemID)
	}
}

// Workers claiming at the same time get every job exactly once.
func TestConcurrentClaimersGetEveryJobOnce(t *testing.T) {
	t.Parallel()

	const (
		jobCount    = 30
		workerCount = 8
	)

	fixture := newEnrichmentFixture(t)

	for index := range jobCount {
		id := fmt.Sprintf("item-c%02d", index)
		execute(t, fixture.inspection, newItem(func(row *itemRow) {
			row.id, row.matchKey = "'"+id+"'", "'"+id+"'"
		}))
		fixture.enqueue(t, id, sqlite.EnrichmentReasonScan)
	}

	var (
		mutex   sync.Mutex
		claimed = map[string]int{}
		group   sync.WaitGroup
	)

	for range workerCount {
		group.Go(func() {
			for _, itemID := range claimUntilEmpty(t, fixture.store) {
				mutex.Lock()
				claimed[itemID]++
				mutex.Unlock()
			}
		})
	}

	group.Wait()

	if len(claimed) != jobCount {
		t.Errorf("claimed jobs = %d, want %d", len(claimed), jobCount)
	}

	for itemID, count := range claimed {
		if count != 1 {
			t.Errorf("job %s claimed %d times, want once", itemID, count)
		}
	}
}
