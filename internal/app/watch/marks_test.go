package watch_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/magpeek/mivo/internal/app/access"
	appwatch "github.com/magpeek/mivo/internal/app/watch"
	"github.com/magpeek/mivo/internal/domain"
	domainaccess "github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/watch"
)

// showFixture has show-1 with season-1 (episodes 1 and 2) and season-2 (episode 3).
func showFixture(t *testing.T) fixture {
	t.Helper()

	fixture := newFixture(t)
	fixture.catalog.add(appwatch.Target{ItemID: "show-1", Kind: library.ItemKindShow})
	fixture.catalog.add(appwatch.Target{ItemID: "season-1", Kind: library.ItemKindSeason})
	fixture.catalog.add(appwatch.Target{ItemID: "season-2", Kind: library.ItemKindSeason})
	fixture.catalog.addEpisode("show-1", "season-1", "episode-1")
	fixture.catalog.addEpisode("show-1", "season-1", "episode-2")
	fixture.catalog.addEpisode("show-1", "season-2", "episode-3")

	return fixture
}

func TestMarkWatchedOnALeaf(t *testing.T) {
	t.Parallel()

	for _, kind := range []library.ItemKind{library.ItemKindMovie, library.ItemKindEpisode, library.ItemKindAudiobook} {
		fixture := newFixture(t)
		fixture.catalog.add(appwatch.Target{ItemID: "item-1", Kind: kind, AttachKey: "key-1"})

		if err := fixture.service.MarkWatched(viewerContext(profileID), "item-1"); err != nil {
			t.Fatalf("MarkWatched %s: %v", kind, err)
		}

		row := fixture.store.state(profileID, "item-1")
		want := watch.State{Watched: true, PlayCount: 1, LastPlayed: fixedNow()}

		if row.state != want || row.attachKey != "key-1" {
			t.Fatalf("MarkWatched %s: stored %+v with key %q, want %+v with key-1", kind, row.state, row.attachKey, want)
		}
	}
}

func TestMarkWatchedOnAShowCascadesToItsEpisodes(t *testing.T) {
	t.Parallel()

	fixture := showFixture(t)
	fixture.store.put(profileID, "episode-2", watch.State{PositionSeconds: 600, LastPlayed: fixedNow()})

	if err := fixture.service.MarkWatched(viewerContext(profileID), "show-1"); err != nil {
		t.Fatalf("MarkWatched: %v", err)
	}

	if fixture.store.updates != 1 {
		t.Fatalf("store updates = %d, want one for the whole show", fixture.store.updates)
	}

	for _, episodeID := range []string{"episode-1", "episode-2", "episode-3"} {
		row := fixture.store.state(profileID, episodeID)
		if !row.state.Watched || row.state.InProgress() || row.attachKey != "tmdb:episode:"+episodeID {
			t.Fatalf("%s after marking the show: %+v key %q, want watched without position", episodeID, row.state, row.attachKey)
		}
	}

	if row := fixture.store.state(profileID, "show-1"); row != (storedState{}) {
		t.Fatalf("show got a state of its own: %+v", row)
	}
}

func TestMarkUnwatchedOnASeasonCascadesOnlyToItsEpisodes(t *testing.T) {
	t.Parallel()

	fixture := showFixture(t)
	watched := watch.State{Watched: true, PlayCount: 2, LastPlayed: fixedNow()}

	for _, episodeID := range []string{"episode-1", "episode-2", "episode-3"} {
		fixture.store.put(profileID, episodeID, watched)
	}

	if err := fixture.service.MarkUnwatched(viewerContext(profileID), "season-1"); err != nil {
		t.Fatalf("MarkUnwatched: %v", err)
	}

	unwatched := watch.State{PlayCount: 2, LastPlayed: fixedNow()}
	for episodeID, want := range map[string]watch.State{"episode-1": unwatched, "episode-2": unwatched, "episode-3": watched} {
		if got := fixture.store.state(profileID, episodeID).state; got != want {
			t.Fatalf("%s = %+v, want %+v", episodeID, got, want)
		}
	}
}

func TestMarkingAContainerWithoutEpisodesWritesNothing(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.catalog.add(appwatch.Target{ItemID: "show-1", Kind: library.ItemKindShow})

	if err := fixture.service.MarkWatched(viewerContext(profileID), "show-1"); err != nil {
		t.Fatalf("MarkWatched: %v", err)
	}

	if fixture.store.updates != 0 {
		t.Fatalf("store updates = %d, want 0", fixture.store.updates)
	}
}

func TestMarkWrapsEpisodeListErrors(t *testing.T) {
	t.Parallel()

	fixture := showFixture(t)
	fixture.catalog.listErr = domain.ErrUnavailable

	if err := fixture.service.MarkWatched(viewerContext(profileID), "show-1"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestMarkWrapsStoreErrors(t *testing.T) {
	t.Parallel()

	fixture := showFixture(t)
	fixture.store.failWith = domain.ErrUnavailable

	if err := fixture.service.MarkUnwatched(viewerContext(profileID), "episode-1"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestMarksRefuseKindsWithoutWatchState(t *testing.T) {
	t.Parallel()

	for _, kind := range []library.ItemKind{library.ItemKindPhoto, library.ItemKindTrack, library.ItemKindAlbum, library.ItemKindExtra} {
		fixture := newFixture(t)
		fixture.catalog.add(appwatch.Target{ItemID: "item-1", Kind: kind})

		if err := fixture.service.MarkWatched(viewerContext(profileID), "item-1"); !errors.Is(err, appwatch.ErrNoWatchState) {
			t.Fatalf("MarkWatched on %s: error = %v, want ErrNoWatchState", kind, err)
		}
	}
}

func TestMarksOnAHiddenItemAreNotFound(t *testing.T) {
	t.Parallel()

	fixture := showFixture(t)
	fixture.catalog.hidden["show-1"] = true

	if err := fixture.service.MarkWatched(viewerContext(profileID), "show-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}

	if fixture.store.updates != 0 {
		t.Fatal("marking a hidden show reached the store")
	}
}

func TestHideFromContinueWatchingUntilTheNextReport(t *testing.T) {
	t.Parallel()

	fixture := movieFixture(t)
	ctx := viewerContext(profileID)

	if err := fixture.service.ReportProgress(ctx, "movie-1", 600); err != nil {
		t.Fatalf("ReportProgress: %v", err)
	}

	if err := fixture.service.HideFromContinueWatching(ctx, "movie-1"); err != nil {
		t.Fatalf("HideFromContinueWatching: %v", err)
	}

	if state := fixture.store.state(profileID, "movie-1").state; !state.Hidden || watch.InContinueWatching(state, fixedNow()) {
		t.Fatalf("state after hide = %+v, want hidden", state)
	}

	if err := fixture.service.ReportProgress(ctx, "movie-1", 700); err != nil {
		t.Fatalf("ReportProgress: %v", err)
	}

	if state := fixture.store.state(profileID, "movie-1").state; state.Hidden {
		t.Fatalf("state after a new report = %+v, want not hidden", state)
	}
}

func TestHideLeavesItemsThatAreNotInProgressUnchanged(t *testing.T) {
	t.Parallel()

	fixture := showFixture(t)
	finished := watch.State{Watched: true, PlayCount: 1, LastPlayed: fixedNow()}
	fixture.store.put(profileID, "episode-1", finished)
	fixture.store.put(profileID, "episode-2", watch.State{PositionSeconds: 300, LastPlayed: fixedNow()})

	if err := fixture.service.HideFromContinueWatching(viewerContext(profileID), "season-1"); err != nil {
		t.Fatalf("HideFromContinueWatching: %v", err)
	}

	if got := fixture.store.state(profileID, "episode-1").state; got != finished {
		t.Fatalf("finished episode = %+v, want unchanged %+v", got, finished)
	}

	if got := fixture.store.state(profileID, "episode-2").state; !got.Hidden {
		t.Fatalf("in-progress episode = %+v, want hidden", got)
	}
}

func TestEpisodeReadsCarryTheViewersRestriction(t *testing.T) {
	t.Parallel()

	limit, err := domainaccess.MaxAgeLimit(domainaccess.Age6, true)
	if err != nil {
		t.Fatal(err)
	}

	viewer := domainaccess.Viewer{
		AccountID: "account-1", ProfileID: profileID, Libraries: domainaccess.OnlyLibraries("lib-1"), Limit: limit,
	}
	ctx := access.WithViewer(context.Background(), viewer)
	fixture := showFixture(t)

	if err := fixture.service.MarkWatched(ctx, "show-1"); err != nil {
		t.Fatalf("MarkWatched: %v", err)
	}

	if _, err := fixture.service.States(ctx, []appwatch.ItemRef{{ID: "show-1", Kind: library.ItemKindShow}}); err != nil {
		t.Fatalf("States: %v", err)
	}

	want := viewer.Restriction()
	for what, got := range map[string][]domainaccess.Restriction{
		"ListEpisodes": fixture.catalog.listRestrictions, "EpisodeCounts": fixture.store.countRestrictions,
	} {
		if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
			t.Errorf("%s restrictions = %+v, want the viewer's %+v", what, got, want)
		}
	}
}
