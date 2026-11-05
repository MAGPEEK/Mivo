package sqlite_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// The store implements the queue, cache and refresh ports of the enrichment queue (M6-11).
var (
	_ enrich.JobEnqueuer   = (*sqlite.EnrichmentStore)(nil)
	_ enrich.JobStore      = (*sqlite.EnrichmentStore)(nil)
	_ enrich.ProviderCache = (*sqlite.EnrichmentStore)(nil)
	_ enrich.RefreshSource = (*sqlite.EnrichmentStore)(nil)
)

const (
	matchVersion  = 7
	refreshWindow = 7 * 24 * time.Hour
	refreshLimit  = 10
)

func TestClaimNextJobRejectsLeasesBelowAMillisecond(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	fixture.enqueue(t, "item-1", sqlite.EnrichmentReasonScan)

	if _, _, err := fixture.store.ClaimNextJob(t.Context(), 500*time.Microsecond); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("ClaimNextJob with a sub-millisecond lease error = %v, want ErrInvalid", err)
	}

	if _, ok, err := fixture.store.ClaimNextJob(t.Context(), time.Millisecond); err != nil || !ok {
		t.Fatalf("ClaimNextJob with a one-millisecond lease = %t, %v, want the job", ok, err)
	}
}

func TestReleaseJobMakesTheJobClaimableWithoutAnAttempt(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	fixture.enqueue(t, "item-1", sqlite.EnrichmentReasonScan)

	job, _ := fixture.claim(t)
	if err := fixture.store.ReleaseJob(t.Context(), job); err != nil {
		t.Fatalf("ReleaseJob error = %v", err)
	}

	again, ok := fixture.claim(t)
	if !ok || again.ItemID != "item-1" || again.Attempts != 0 {
		t.Fatalf("claim after release = %+v, %t, want item-1 without attempts", again, ok)
	}
}

func TestReleaseJobReportsALostOrVanishedJob(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	fixture.enqueue(t, "item-1", sqlite.EnrichmentReasonScan)

	stale, _ := fixture.claim(t)
	fixture.advance(testLease)
	fixture.claim(t) // another worker takes the orphaned job

	if err := fixture.store.ReleaseJob(t.Context(), stale); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("ReleaseJob after losing the lease error = %v, want ErrConflict", err)
	}

	execute(t, fixture.inspection, "DELETE FROM items WHERE id = 'item-1'")

	if err := fixture.store.ReleaseJob(t.Context(), stale); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ReleaseJob of a deleted item error = %v, want ErrNotFound", err)
	}
}

func TestItemsDueForRefresh(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	setMatch(t, fixture, "item-1", sqlite.ItemMatch{State: match.MatchStateUnmatched})
	setMatch(t, fixture, "show-1", sqlite.ItemMatch{State: match.MatchStateDisabled})
	fixture.advance(refreshWindow + time.Hour)
	setMatch(t, fixture, "season-1", sqlite.ItemMatch{State: match.MatchStateUnmatched})

	cutoff := fixture.now.Add(-refreshWindow)
	if got := dueItems(t, fixture, cutoff); !slices.Equal(got, []string{"item-1"}) {
		t.Errorf("due items = %q, want [item-1] (show-1 disabled, season-1 fresh)", got)
	}

	// A row from before checked_at existed is due first.
	execute(t, fixture.inspection, "UPDATE item_match SET checked_at = NULL WHERE item_id = 'season-1'")

	if got := dueItems(t, fixture, cutoff); !slices.Equal(got, []string{"season-1", "item-1"}) {
		t.Errorf("due items = %q, want [season-1 item-1]", got)
	}

	if err := fixture.store.SetLibraryOnlineMetadata(t.Context(), "lib-1", false); err != nil {
		t.Fatalf("SetLibraryOnlineMetadata error = %v", err)
	}

	if got := dueItems(t, fixture, cutoff); !slices.Equal(got, []string{"season-1"}) {
		t.Errorf("due items = %q, want [season-1] (lib-1 offline)", got)
	}
}

// An item whose job is backing off must not be listed: re-enqueueing it would make it due now.
func TestItemsDueForRefreshSkipsQueuedItemsSoBackoffHolds(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	setMatch(t, fixture, "item-1", sqlite.ItemMatch{State: match.MatchStateUnmatched})
	fixture.advance(refreshWindow + time.Hour)

	fixture.enqueue(t, "item-1", sqlite.EnrichmentReasonScan)
	job, _ := fixture.claim(t)
	backoffUntil := fixture.now.Add(testBackoff)

	if err := fixture.store.MarkJobFailed(t.Context(), job, "tmdb unavailable", backoffUntil); err != nil {
		t.Fatalf("MarkJobFailed error = %v", err)
	}

	if got := dueItems(t, fixture, fixture.now.Add(-refreshWindow)); len(got) != 0 {
		t.Fatalf("due items = %q, want none while item-1 is queued", got)
	}

	if due := queryStrings(t, fixture.inspection, "SELECT not_before FROM enrichment_queue"); !slices.Equal(due, []string{formatMillis(backoffUntil)}) {
		t.Fatalf("not_before = %q, want the backoff %s kept", due, formatMillis(backoffUntil))
	}
}

func TestItemsDueForRefreshReportsDatabaseErrors(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	if err := fixture.db.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}

	if _, err := fixture.store.ItemsDueForRefresh(t.Context(), enrichmentStart(), refreshLimit); err == nil {
		t.Fatal("ItemsDueForRefresh on a closed database error = nil")
	}
}

// goose down from 00007 drops the reference columns and keeps the matches.
func TestMigrationDownTo00006KeepsMatches(t *testing.T) {
	t.Parallel()

	inspection := openCatalogV2Fixture(t)
	execute(t, inspection, `INSERT INTO item_match (item_id, state, provider, confidence, matched_at, ref_provider, ref_kind, ref_id)
		VALUES ('item-1', 'matched', 'musicbrainz', 1, 5, 'musicbrainz', 'release', 'b84ee12a')`)

	provider := newSourceMigrationProvider(t, inspection)
	if _, err := provider.DownTo(t.Context(), matchVersion-1); err != nil {
		t.Fatalf("DownTo(%d) error = %v", matchVersion-1, err)
	}

	if got := queryStrings(t, inspection, "SELECT item_id || '|' || provider FROM item_match"); !slices.Equal(got, []string{"item-1|musicbrainz"}) {
		t.Errorf("matches after down = %q, want the row kept", got)
	}

	if _, err := provider.Up(t.Context()); err != nil {
		t.Fatalf("Up error = %v", err)
	}

	if got := queryStrings(t, inspection, "SELECT ref_provider || '|' || coalesce(checked_at, 'none') FROM item_match"); !slices.Equal(got, []string{"|none"}) {
		t.Errorf("reapplied columns = %q, want defaults", got)
	}
}

func setMatch(t *testing.T, fixture enrichmentFixture, itemID string, itemMatch sqlite.ItemMatch) {
	t.Helper()

	if err := fixture.store.SetItemMatch(t.Context(), itemID, itemMatch); err != nil {
		t.Fatalf("SetItemMatch(%s) error = %v", itemID, err)
	}
}

func dueItems(t *testing.T, fixture enrichmentFixture, cutoff time.Time) []string {
	t.Helper()

	itemIDs, err := fixture.store.ItemsDueForRefresh(t.Context(), cutoff, refreshLimit)
	if err != nil {
		t.Fatalf("ItemsDueForRefresh error = %v", err)
	}

	return itemIDs
}
