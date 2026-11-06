package watch_test

import (
	"errors"
	"math"
	"testing"

	appwatch "github.com/magpeek/mivo/internal/app/watch"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/watch"
)

const movieDuration = 6000

func movieFixture(t *testing.T) fixture {
	t.Helper()

	fixture := newFixture(t)
	fixture.catalog.add(appwatch.Target{ItemID: "movie-1", Kind: library.ItemKindMovie, DurationSeconds: movieDuration, AttachKey: "tmdb:movie:603"})

	return fixture
}

func TestReportProgressAppliesTheWatchedThreshold(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		position float64
		want     watch.State
	}{
		{"peek stays not started", 59, watch.State{}},
		{"in progress", 600, watch.State{PositionSeconds: 600, LastPlayed: fixedNow()}},
		{"just below 90 %", 5399, watch.State{PositionSeconds: 5399, LastPlayed: fixedNow()}},
		{"at 90 % becomes watched", 5400, watch.State{Watched: true, PlayCount: 1, LastPlayed: fixedNow()}},
		{"past the end becomes watched", 7000, watch.State{Watched: true, PlayCount: 1, LastPlayed: fixedNow()}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := movieFixture(t)

			if err := fixture.service.ReportProgress(viewerContext(profileID), "movie-1", test.position); err != nil {
				t.Fatalf("ReportProgress: %v", err)
			}

			if got := fixture.store.state(profileID, "movie-1").state; got != test.want {
				t.Fatalf("state = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestReportProgressCostsOneLookupAndOneUpdate(t *testing.T) {
	t.Parallel()

	fixture := movieFixture(t)

	if err := fixture.service.ReportProgress(viewerContext(profileID), "movie-1", 600); err != nil {
		t.Fatalf("ReportProgress: %v", err)
	}

	if fixture.catalog.lookups != 1 || fixture.store.updates != 1 || fixture.store.stateReads != 0 {
		t.Fatalf("lookups %d, updates %d, reads %d; want 1, 1, 0", fixture.catalog.lookups, fixture.store.updates, fixture.store.stateReads)
	}

	row := fixture.store.state(profileID, "movie-1")
	if row.attachKey != "tmdb:movie:603" || !row.updatedAt.Equal(fixedNow()) {
		t.Fatalf("stored attach key %q at %v, want tmdb:movie:603 at %v", row.attachKey, row.updatedAt, fixedNow())
	}

	if other := fixture.store.state(otherProfileID, "movie-1").state; other != (watch.State{}) {
		t.Fatalf("other profile state = %+v, want untouched", other)
	}
}

func TestReportProgressCountsTheCreditsOnce(t *testing.T) {
	t.Parallel()

	fixture := movieFixture(t)
	ctx := viewerContext(profileID)

	for _, position := range []float64{5400, 5500, 5990} {
		if err := fixture.service.ReportProgress(ctx, "movie-1", position); err != nil {
			t.Fatalf("ReportProgress(%v): %v", position, err)
		}
	}

	if got := fixture.store.state(profileID, "movie-1").state.PlayCount; got != 1 {
		t.Fatalf("play count = %d, want 1", got)
	}
}

func TestReportProgressWithUnknownDurationNeverFinishes(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.catalog.add(appwatch.Target{ItemID: "book-1", Kind: library.ItemKindAudiobook})

	if err := fixture.service.ReportProgress(viewerContext(profileID), "book-1", 100000); err != nil {
		t.Fatalf("ReportProgress: %v", err)
	}

	want := watch.State{PositionSeconds: 100000, LastPlayed: fixedNow()}
	if got := fixture.store.state(profileID, "book-1").state; got != want {
		t.Fatalf("state = %+v, want %+v", got, want)
	}
}

func TestReportProgressRefusesNonFinitePositions(t *testing.T) {
	t.Parallel()

	for _, position := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		fixture := movieFixture(t)

		err := fixture.service.ReportProgress(viewerContext(profileID), "movie-1", position)
		if !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("ReportProgress(%v) error = %v, want ErrInvalid", position, err)
		}

		if fixture.catalog.lookups != 0 || fixture.store.updates != 0 {
			t.Fatalf("ReportProgress(%v) reached the catalog or the store", position)
		}
	}
}

func TestReportProgressRefusesKindsWithoutWatchState(t *testing.T) {
	t.Parallel()

	for _, kind := range []library.ItemKind{library.ItemKindPhoto, library.ItemKindTrack, library.ItemKindExtra, library.ItemKindShow} {
		fixture := newFixture(t)
		fixture.catalog.add(appwatch.Target{ItemID: "item-1", Kind: kind, DurationSeconds: 300})

		err := fixture.service.ReportProgress(viewerContext(profileID), "item-1", 100)
		if !errors.Is(err, appwatch.ErrNoWatchState) || !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("ReportProgress on %s: error = %v, want ErrNoWatchState", kind, err)
		}

		if fixture.store.updates != 0 {
			t.Fatalf("ReportProgress on %s wrote to the store", kind)
		}
	}
}

func TestReportProgressOnAHiddenItemIsNotFound(t *testing.T) {
	t.Parallel()

	fixture := movieFixture(t)
	fixture.catalog.hidden["movie-1"] = true

	err := fixture.service.ReportProgress(viewerContext(profileID), "movie-1", 600)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}

	if fixture.store.updates != 0 {
		t.Fatal("progress of a hidden item reached the store")
	}
}

func TestReportProgressWrapsStoreErrors(t *testing.T) {
	t.Parallel()

	fixture := movieFixture(t)
	fixture.store.failWith = domain.ErrUnavailable

	if err := fixture.service.ReportProgress(viewerContext(profileID), "movie-1", 600); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}
