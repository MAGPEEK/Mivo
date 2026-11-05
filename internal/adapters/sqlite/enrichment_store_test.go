package sqlite_test

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// enrichmentStart is the fixed start time of the enrichment store clock.
func enrichmentStart() time.Time {
	return time.Date(2026, time.November, 5, 12, 0, 0, 0, time.UTC)
}

// enrichmentFixture is a migrated database with the catalog fixture rows (item-1 in lib-1,
// show-1 and season-1 in lib-2) and a store whose clock the test moves.
type enrichmentFixture struct {
	store      *sqlite.EnrichmentStore
	db         *sqlite.DB
	path       string
	inspection *sql.DB
	now        *time.Time
}

func newEnrichmentFixture(t *testing.T) enrichmentFixture {
	t.Helper()

	return newLoggingEnrichmentFixture(t, nil)
}

// newLoggingEnrichmentFixture is newEnrichmentFixture with logger behind the database.
func newLoggingEnrichmentFixture(t *testing.T, logger *slog.Logger) enrichmentFixture {
	t.Helper()

	db, path := openTestDB(t, logger)
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate error = %v", err)
	}

	inspection := openForeignKeyInspectionDB(t, path)

	for _, statement := range slices.Concat(constraintFixture(), catalogV2Fixture()) {
		execute(t, inspection, statement)
	}

	now := enrichmentStart()

	return enrichmentFixture{
		store:      sqlite.NewEnrichmentStore(db, func() time.Time { return now }),
		db:         db,
		path:       path,
		inspection: inspection,
		now:        &now,
	}
}

func (fixture enrichmentFixture) advance(duration time.Duration) {
	*fixture.now = fixture.now.Add(duration)
}

func TestItemMatchOfANeverMatchedItemIsUnmatched(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)

	got, err := fixture.store.ItemMatch(t.Context(), "item-1")
	if err != nil || got != (sqlite.ItemMatch{State: match.MatchStateUnmatched}) {
		t.Errorf("ItemMatch = %+v, %v, want unmatched", got, err)
	}

	if _, err := fixture.store.ItemMatch(t.Context(), "item-9"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ItemMatch of an unknown item error = %v, want ErrNotFound", err)
	}
}

func TestSetItemMatchStoresAndReplacesTheMatch(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	matched := sqlite.ItemMatch{
		State: match.MatchStateMatched, Provider: "tmdb", Confidence: 0.92, MatchedAt: enrichmentStart(),
		Ref:       match.ProviderRef{Provider: match.RefMusicBrainz, Kind: match.RefKindRelease, ID: "b84ee12a-09ef-421b-82de-0441a926375b"},
		CheckedAt: enrichmentStart(),
	}
	disabled := sqlite.ItemMatch{State: match.MatchStateDisabled, CheckedAt: enrichmentStart()}

	for _, want := range []sqlite.ItemMatch{matched, disabled, matched} {
		if err := fixture.store.SetItemMatch(t.Context(), "item-1", want); err != nil {
			t.Fatalf("SetItemMatch(%+v) error = %v", want, err)
		}

		got, err := fixture.store.ItemMatch(t.Context(), "item-1")
		if err != nil || got != want {
			t.Errorf("ItemMatch = %+v, %v, want %+v", got, err, want)
		}
	}

	if got := rowCount(t, fixture.inspection, "item_match"); got != 1 {
		t.Errorf("item_match rows = %d, want 1", got)
	}
}

func TestSetItemMatchRejectsWhatItCannotStore(t *testing.T) {
	t.Parallel()

	matched := func(change func(*sqlite.ItemMatch)) sqlite.ItemMatch {
		value := sqlite.ItemMatch{State: match.MatchStateMatched, Provider: "tmdb", Confidence: 1, MatchedAt: enrichmentStart()}
		change(&value)

		return value
	}
	tests := []struct {
		name   string
		itemID string
		match  sqlite.ItemMatch
		want   error
	}{
		{"unknown state", "item-1", sqlite.ItemMatch{State: "maybe"}, domain.ErrInvalid},
		{"negative confidence", "item-1", matched(func(m *sqlite.ItemMatch) { m.Confidence = -0.1 }), domain.ErrInvalid},
		{"confidence above one", "item-1", matched(func(m *sqlite.ItemMatch) { m.Confidence = 1.5 }), domain.ErrInvalid},
		{"confidence not a number", "item-1", matched(func(m *sqlite.ItemMatch) { m.Confidence = math.NaN() }), domain.ErrInvalid},
		{"match without provider", "item-1", matched(func(m *sqlite.ItemMatch) { m.Provider = "" }), domain.ErrInvalid},
		{"match without time", "item-1", matched(func(m *sqlite.ItemMatch) { m.MatchedAt = time.Time{} }), domain.ErrInvalid},
		{"reference of an unknown provider", "item-1", matched(func(m *sqlite.ItemMatch) { m.Ref = match.ProviderRef{Provider: "anidb", ID: "1"} }), domain.ErrInvalid},
		{"reference of an unknown kind", "item-1", matched(func(m *sqlite.ItemMatch) { m.Ref = match.ProviderRef{Provider: match.RefTMDB, Kind: "book", ID: "1"} }), domain.ErrInvalid},
		{"reference without ID", "item-1", matched(func(m *sqlite.ItemMatch) {
			m.Ref = match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie}
		}), domain.ErrInvalid},
		{"ID without provider", "item-1", matched(func(m *sqlite.ItemMatch) { m.Ref = match.ProviderRef{ID: "603"} }), domain.ErrInvalid},
		{"unknown item", "item-9", matched(func(*sqlite.ItemMatch) {}), domain.ErrNotFound},
	}

	fixture := newEnrichmentFixture(t)

	for _, test := range tests {
		err := fixture.store.SetItemMatch(t.Context(), test.itemID, test.match)
		if !errors.Is(err, test.want) {
			t.Errorf("%s: SetItemMatch error = %v, want %v", test.name, err, test.want)
		}
	}
}

func TestDeleteProviderArtworkKeepsOtherSourcesAndReturnsStoredFiles(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	execute(t, fixture.inspection, newArtwork(func(row *artworkRow) {
		provider(row)
		row.id, row.dataPath, row.selected, row.kind = "'art-3'", "'artwork/abc.jpg'", "1", "'backdrop'"
	}))
	execute(t, fixture.inspection, newArtwork(func(row *artworkRow) { provider(row); row.id = "'art-4'" }))
	execute(t, fixture.inspection, newArtwork(func(row *artworkRow) {
		provider(row)
		row.id, row.itemID = "'art-5'", "'show-1'"
	}))

	files, err := fixture.store.DeleteProviderArtwork(t.Context(), "item-1")
	if err != nil || !slices.Equal(files, []string{"artwork/abc.jpg"}) {
		t.Errorf("DeleteProviderArtwork = %q, %v, want [artwork/abc.jpg]", files, err)
	}

	rows := queryStrings(t, fixture.inspection, "SELECT id FROM artwork ORDER BY id")
	if want := []string{"art-1", "art-5"}; !slices.Equal(rows, want) {
		t.Errorf("artwork after the delete = %q, want %q", rows, want)
	}

	files, err = fixture.store.DeleteProviderArtwork(t.Context(), "item-1")
	if err != nil || len(files) != 0 {
		t.Errorf("second DeleteProviderArtwork = %q, %v, want nothing", files, err)
	}
}

// A tampered data_path must not make the caller delete a file outside the data directory.
func TestDeleteProviderArtworkSkipsPathsOutsideTheDataDirectory(t *testing.T) {
	t.Parallel()

	logger, records := newRecordingLogger()
	fixture := newLoggingEnrichmentFixture(t, logger)
	tampered := []string{"../escape.jpg", "/etc/passwd", `artwork\..\..\escape.jpg`, "artwork/../../escape.jpg"}

	for index, dataPath := range tampered {
		execute(t, fixture.inspection, newArtwork(func(row *artworkRow) {
			provider(row)
			row.id, row.dataPath = fmt.Sprintf("'bad-%d'", index), "'"+dataPath+"'"
		}))
	}

	execute(t, fixture.inspection, newArtwork(func(row *artworkRow) {
		provider(row)
		row.id, row.dataPath = "'good'", "'artwork/ok.jpg'"
	}))

	files, err := fixture.store.DeleteProviderArtwork(t.Context(), "item-1")
	if err != nil || !slices.Equal(files, []string{"artwork/ok.jpg"}) {
		t.Errorf("DeleteProviderArtwork = %q, %v, want only [artwork/ok.jpg]", files, err)
	}

	warnings := 0

	for _, record := range records() {
		if record.level != slog.LevelWarn {
			continue
		}

		warnings++

		if text := fmt.Sprint(record.message, record.attrs); strings.Contains(text, "escape") || strings.Contains(text, "passwd") {
			t.Errorf("warning %q names the path", text)
		}
	}

	if warnings != len(tampered) {
		t.Errorf("warnings = %d, want %d", warnings, len(tampered))
	}
}

func TestOnlineMetadataSwitchDefaultsOnAndFollowsTheLibrary(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)

	if enabled, err := fixture.store.ItemOnlineMetadata(t.Context(), "item-1"); err != nil || !enabled {
		t.Errorf("ItemOnlineMetadata of a new library = %t, %v, want true", enabled, err)
	}

	fixture.advance(time.Minute)

	if err := fixture.store.SetLibraryOnlineMetadata(t.Context(), "lib-1", false); err != nil {
		t.Fatalf("SetLibraryOnlineMetadata error = %v", err)
	}

	if enabled, err := fixture.store.ItemOnlineMetadata(t.Context(), "item-1"); err != nil || enabled {
		t.Errorf("ItemOnlineMetadata after switching off = %t, %v, want false", enabled, err)
	}

	if enabled, err := fixture.store.ItemOnlineMetadata(t.Context(), "show-1"); err != nil || !enabled {
		t.Errorf("ItemOnlineMetadata of the other library = %t, %v, want true", enabled, err)
	}

	updated := queryStrings(t, fixture.inspection, "SELECT updated_at FROM libraries WHERE id = 'lib-1'")
	if want := []string{formatMillis(enrichmentStart().Add(time.Minute))}; !slices.Equal(updated, want) {
		t.Errorf("updated_at = %q, want %q", updated, want)
	}
}

func TestOnlineMetadataSwitchReportsUnknownRows(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)

	if _, err := fixture.store.ItemOnlineMetadata(t.Context(), "item-9"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ItemOnlineMetadata of an unknown item error = %v, want ErrNotFound", err)
	}

	if err := fixture.store.SetLibraryOnlineMetadata(t.Context(), "lib-9", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetLibraryOnlineMetadata of an unknown library error = %v, want ErrNotFound", err)
	}
}

// Regression for M6-03: provider data written for a matched item still cannot change a
// locked field (ADR 0027 §6, ADR 0031 §8).
func TestProviderDataForAMatchedItemKeepsLockedFields(t *testing.T) {
	t.Parallel()

	fixture := newMetadataFixture(t)
	store := sqlite.NewEnrichmentStore(fixture.db, nil)

	if err := fixture.catalog.SaveMetadata(t.Context(), fixture.movieID, fullRecord(t)); err != nil {
		t.Fatalf("SaveMetadata error = %v", err)
	}

	fixture.lock(t, fixture.movieID, metadata.FieldTitle)
	fixture.lock(t, fixture.movieID, metadata.FieldProviderIDs)

	matched := sqlite.ItemMatch{State: match.MatchStateMatched, Provider: "tmdb", Confidence: 0.9, MatchedAt: enrichmentStart()}
	if err := store.SetItemMatch(t.Context(), fixture.movieID, matched); err != nil {
		t.Fatalf("SetItemMatch error = %v", err)
	}

	if err := fixture.catalog.SaveMetadata(t.Context(), fixture.movieID, otherRecord(t)); err != nil {
		t.Fatalf("SaveMetadata of provider data error = %v", err)
	}

	state, err := fixture.catalog.ItemMetadataState(t.Context(), fixture.movieID)
	if err != nil {
		t.Fatalf("ItemMetadataState error = %v", err)
	}

	stored := storedFullRecord(t)
	if state.Record.Title != stored.Title || !slices.Equal(state.Record.ProviderIDs, stored.ProviderIDs) {
		t.Errorf("title, provider IDs = %q, %v, want the locked %q, %v",
			state.Record.Title, state.Record.ProviderIDs, stored.Title, stored.ProviderIDs)
	}

	if state.Record.Summary != otherRecord(t).Summary {
		t.Errorf("summary = %q, want the provider's %q", state.Record.Summary, otherRecord(t).Summary)
	}
}

// Every store method reports a failing database instead of a result.
func TestEnrichmentStoreMethodsReportDatabaseErrors(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	ctx := cancelledContext(t)
	store := fixture.store
	cacheEntry := sqlite.CacheEntry{Payload: []byte(`{}`)}
	job := sqlite.EnrichmentJob{ItemID: "item-1"}
	disabled := sqlite.ItemMatch{State: match.MatchStateDisabled}

	calls := map[string]func() error{
		"ItemMatch":                func() error { _, err := store.ItemMatch(ctx, "item-1"); return err },
		"SetItemMatch":             func() error { return store.SetItemMatch(ctx, "item-1", disabled) },
		"DeleteProviderArtwork":    func() error { _, err := store.DeleteProviderArtwork(ctx, "item-1"); return err },
		"ItemOnlineMetadata":       func() error { _, err := store.ItemOnlineMetadata(ctx, "item-1"); return err },
		"SetLibraryOnlineMetadata": func() error { return store.SetLibraryOnlineMetadata(ctx, "lib-1", true) },
		"ReadProviderCache":        func() error { _, err := store.ReadProviderCache(ctx, "tmdb", "k"); return err },
		"WriteProviderCache":       func() error { return store.WriteProviderCache(ctx, "tmdb", "k", cacheEntry) },
		"Enqueue":                  func() error { return store.Enqueue(ctx, "item-1", sqlite.EnrichmentReasonScan) },
		"ClaimNextJob":             func() error { _, _, err := store.ClaimNextJob(ctx, time.Minute); return err },
		"MarkJobDone":              func() error { return store.MarkJobDone(ctx, job) },
		"MarkJobFailed":            func() error { return store.MarkJobFailed(ctx, job, "x", enrichmentStart()) },
		"QueueLength":              func() error { _, err := store.QueueLength(ctx); return err },
	}

	for name, call := range calls {
		if err := call(); err == nil || errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s with a cancelled context error = %v, want a database error", name, err)
		}
	}
}
