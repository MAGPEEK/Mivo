package rows_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	appaccess "github.com/magpeek/mivo/internal/app/access"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	approws "github.com/magpeek/mivo/internal/app/rows"
	appwatch "github.com/magpeek/mivo/internal/app/watch"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/rows"
	"github.com/magpeek/mivo/internal/domain/watch"
)

// fakeWatch implements WatchRows and ItemStates in memory.
type fakeWatch struct {
	continueIDs []string
	upNextIDs   []string
	failWith    error
	requests    []approws.WatchRowRequest
	stateCalls  int
}

func (fake *fakeWatch) ContinueWatching(_ context.Context, request approws.WatchRowRequest) ([]string, error) {
	fake.requests = append(fake.requests, request)

	return fake.continueIDs, fake.failWith
}

func (fake *fakeWatch) UpNext(_ context.Context, request approws.WatchRowRequest) ([]string, error) {
	fake.requests = append(fake.requests, request)

	return fake.upNextIDs, fake.failWith
}

func (fake *fakeWatch) States(_ context.Context, items []appwatch.ItemRef) (map[string]appwatch.ItemState, error) {
	fake.stateCalls++

	states := map[string]appwatch.ItemState{}
	for _, item := range items {
		states[item.ID] = appwatch.ItemState{State: watch.State{PositionSeconds: 42}}
	}

	return states, nil
}

func watchService(t *testing.T, catalog *fakeCatalog, fake *fakeWatch) *approws.Service {
	t.Helper()

	service, err := approws.NewService(approws.Dependencies{
		Libraries: appaccess.NewScopedLibraries(catalog), Rows: catalog, Summaries: catalog, Catalog: catalog, Scans: catalog,
		Clock: fixedNow, Watch: fake, States: fake,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	return service
}

func rowOfKind(list []approws.Row, kind rows.Kind) (approws.Row, bool) {
	index := slices.IndexFunc(list, func(row approws.Row) bool { return row.Kind == kind })
	if index < 0 {
		return approws.Row{}, false
	}

	return list[index], true
}

func kinds(list []approws.Row) []rows.Kind {
	result := make([]rows.Kind, 0, len(list))
	for _, row := range list {
		result = append(result, row.Kind)
	}

	return result
}

func TestHomeWatchRowsComeFirst(t *testing.T) {
	t.Parallel()

	fake := &fakeWatch{continueIDs: []string{"m-old", "b1e2"}, upNextIDs: []string{"a1e2"}}

	got, err := watchService(t, newFixtureCatalog(), fake).Home(adminContext(), 0)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}

	want := []rows.Kind{rows.KindContinueWatching, rows.KindUpNext, rows.KindRecentlyAdded, rows.KindRecentlyAdded}
	if !slices.Equal(kinds(got.Rows), want) {
		t.Fatalf("kinds = %v, want %v", kinds(got.Rows), want)
	}

	resume := got.Rows[0]
	if resume.TitleKey != rows.TitleContinueWatching || resume.CardFormat != rows.CardPoster || resume.LibraryID != "" ||
		resume.ID != "continueWatching" || !slices.Equal(itemIDs(resume), []string{"m-old", "b1e2"}) {
		t.Errorf("continue watching row = %+v", resume)
	}

	if got.Rows[1].TitleKey != rows.TitleUpNext || got.Rows[1].ID != "upNext" {
		t.Errorf("up next row = %+v", got.Rows[1])
	}

	// An episode card keeps its show title and season number and shows the show's poster.
	card := resume.Items[1]
	if card.ShowTitle != "Show B" || card.SeasonNumber != library.NumberOf(3) || card.Summary.Poster == nil || card.Summary.Poster.ID != "poster-show-b" {
		t.Errorf("episode card = %+v", card)
	}

	if card.ViewerState == nil || card.ViewerState.State.PositionSeconds != 42 {
		t.Errorf("viewer state = %+v, want the decorated state", card.ViewerState)
	}

	if fake.requests[0].Limit != rows.DefaultItemLimit || fake.requests[0].ProfileID != "admin" || fake.requests[0].LibraryID != "" {
		t.Errorf("request = %+v", fake.requests[0])
	}
}

func TestHomeWatchRowsAreOmittedWhenEmpty(t *testing.T) {
	t.Parallel()

	got, err := watchService(t, newFixtureCatalog(), &fakeWatch{upNextIDs: []string{"a1e2"}}).Home(adminContext(), 0)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}

	if want := []rows.Kind{rows.KindUpNext, rows.KindRecentlyAdded, rows.KindRecentlyAdded}; !slices.Equal(kinds(got.Rows), want) {
		t.Errorf("kinds = %v, want %v", kinds(got.Rows), want)
	}
}

func TestWatchRowsDropHiddenItems(t *testing.T) {
	t.Parallel()

	catalog := newFixtureCatalog()
	fake := &fakeWatch{continueIDs: []string{"m-old", "gone", "m-new"}}

	got, err := watchService(t, catalog, fake).Home(adminContext(), 0)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}

	if ids := itemIDs(got.Rows[0]); !slices.Equal(ids, []string{"m-old", "m-new"}) {
		t.Errorf("ids = %v, want the unknown one dropped", ids)
	}

	if len(catalog.restrictions["ItemSummaries"]) == 0 {
		t.Error("summaries were read without the viewer's restriction")
	}
}

func TestWatchRowsUseTheRestrictedSummaryReader(t *testing.T) {
	t.Parallel()

	catalog := newFixtureCatalog()
	hiding := &hidingSummaries{fakeCatalog: catalog, hidden: "m-new"}
	fake := &fakeWatch{continueIDs: []string{"m-new", "m-old"}}

	service, err := approws.NewService(approws.Dependencies{
		Libraries: appaccess.NewScopedLibraries(catalog), Rows: catalog, Summaries: hiding, Catalog: catalog, Scans: catalog,
		Clock: fixedNow, Watch: fake, States: fake,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := service.Home(adminContext(), 0)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}

	if ids := itemIDs(got.Rows[0]); !slices.Equal(ids, []string{"m-old"}) {
		t.Errorf("ids = %v, want the hidden item gone", ids)
	}
}

// hidingSummaries drops one item from the summaries, as the restriction of a viewer would.
type hidingSummaries struct {
	*fakeCatalog

	hidden string
}

func (reader *hidingSummaries) ItemSummaries(ctx context.Context, ids []string, restriction access.Restriction) ([]applibrary.ItemSummary, error) {
	summaries, err := reader.fakeCatalog.ItemSummaries(ctx, ids, restriction)
	if err != nil {
		return nil, err
	}

	return slices.DeleteFunc(summaries, func(summary applibrary.ItemSummary) bool { return summary.Item.ID == reader.hidden }), nil
}

func TestWatchRowsNeedAProfile(t *testing.T) {
	t.Parallel()

	fake := &fakeWatch{continueIDs: []string{"m-old"}, upNextIDs: []string{"a1e2"}}
	service := watchService(t, newFixtureCatalog(), fake)

	// A viewer without a selected profile has a restriction but no profile.
	ctx := appaccess.WithViewer(context.Background(), access.Viewer{
		AccountID: "admin", Admin: true, Libraries: access.AllLibraries(), Limit: access.NoLimit(),
	})

	got, err := service.Home(ctx, 0)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}

	if want := []rows.Kind{rows.KindRecentlyAdded, rows.KindRecentlyAdded}; !slices.Equal(kinds(got.Rows), want) {
		t.Errorf("kinds = %v, want %v", kinds(got.Rows), want)
	}

	overview, err := service.LibraryRows(ctx, moviesID, 0)
	if err != nil {
		t.Fatalf("LibraryRows: %v", err)
	}

	if _, found := rowOfKind(overview.Rows, rows.KindContinueWatching); found {
		t.Error("overview has a watch row without a profile")
	}

	if len(fake.requests) != 0 {
		t.Errorf("the store was asked %d times without a profile", len(fake.requests))
	}
}

func TestWatchRowErrorsFailTheRequest(t *testing.T) {
	t.Parallel()

	boom := errors.New("store down")

	_, err := watchService(t, newFixtureCatalog(), &fakeWatch{failWith: boom}).Home(adminContext(), 0)
	if !errors.Is(err, boom) {
		t.Errorf("Home error = %v, want the store error", err)
	}
}

func TestLibraryOverviewStartsWithItsContinueWatching(t *testing.T) {
	t.Parallel()

	fake := &fakeWatch{continueIDs: []string{"m-old"}, upNextIDs: []string{"a1e2"}}

	got, err := watchService(t, newFixtureCatalog(), fake).LibraryRows(adminContext(), moviesID, 0)
	if err != nil {
		t.Fatalf("LibraryRows: %v", err)
	}

	if len(got.Rows) < 2 || got.Rows[0].Kind != rows.KindContinueWatching || got.Rows[1].Kind != rows.KindRecentlyAdded {
		t.Fatalf("kinds = %v, want continueWatching first and no upNext", kinds(got.Rows))
	}

	if got.Rows[0].ID != "continueWatching:"+moviesID || got.Rows[0].LibraryID != moviesID {
		t.Errorf("row = %+v", got.Rows[0])
	}

	if len(fake.requests) != 1 || fake.requests[0].LibraryID != moviesID {
		t.Errorf("requests = %+v, want one scoped to the library", fake.requests)
	}
}

func TestEmptyLibraryOverviewHasNoWatchRow(t *testing.T) {
	t.Parallel()

	fake := &fakeWatch{continueIDs: []string{"m-old"}}

	got, err := watchService(t, newFixtureCatalog(), fake).LibraryRows(adminContext(), emptyID, 0)
	if err != nil {
		t.Fatalf("LibraryRows: %v", err)
	}

	if len(got.Rows) != 0 || got.Hint != rows.HintNoItems {
		t.Errorf("rows = %v hint %q, want none and noItems", kinds(got.Rows), got.Hint)
	}
}

func TestHomeHeroPrefersContinueWatching(t *testing.T) {
	t.Parallel()

	catalog := newFixtureCatalog()

	for index := range catalog.items {
		switch catalog.items[index].summary.Item.ID {
		case "m-new":
			catalog.items[index].backdrop = true
		case "m-old": // in progress, but without a backdrop
		}
	}

	oldest := catalog.items[0]
	oldest.summary.Item.ID, oldest.summary.Item.CreatedAt, oldest.backdrop = "m-resume", day(1), true
	catalog.items = append(catalog.items, oldest)

	fake := &fakeWatch{continueIDs: []string{"m-old", "m-resume"}}

	got, err := watchService(t, catalog, fake).Home(adminContext(), 0)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}

	var ids []string
	for _, hero := range got.Hero {
		ids = append(ids, hero.ItemID)
	}

	if want := []string{"m-resume", "m-new"}; !slices.Equal(ids, want) {
		t.Errorf("hero = %v, want the resumable title with a backdrop first, then the newest %v", ids, want)
	}
}

func TestRowsWithoutWatchPortsStayAsBefore(t *testing.T) {
	t.Parallel()

	got, err := newTestService(t, newFixtureCatalog()).Home(adminContext(), 0)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}

	if _, found := rowOfKind(got.Rows, rows.KindContinueWatching); found {
		t.Error("a watch row without a watch store")
	}
}
