package restapi_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	approws "github.com/magpeek/mivo/internal/app/rows"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/rows"
)

// homeRowsPath is the Home rows route (task M5-12).
const homeRowsPath = "/api/v1/home/rows"

// IDs of the rows fixture.
const (
	rowShowID    = "4b5c6d7e-8f9a-4b0c-9d1e-2f3a4b5c6d7e"
	rowSeasonID  = "5c6d7e8f-9a0b-4c1d-8e2f-3a4b5c6d7e8f"
	rowEpisodeID = "8a9b0c1d-2e3f-4a5b-8c6d-7e8f9a0b1c2d"
	rowImageID   = "6d7e8f9a-0b1c-4d2e-9f3a-4b5c6d7e8f9a"
)

// fakeRows answers fixed rows and records the requested limits.
type fakeRows struct {
	mutex  sync.Mutex
	limits []int
	home   approws.Home
	rows   approws.LibraryRows
	err    error
}

func (service *fakeRows) Home(_ context.Context, limit int) (approws.Home, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	service.limits = append(service.limits, limit)

	return service.home, service.err
}

func (service *fakeRows) LibraryRows(_ context.Context, libraryID string, limit int) (approws.LibraryRows, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	service.limits = append(service.limits, limit)

	if libraryID != testLibraryID {
		return approws.LibraryRows{}, fmt.Errorf("library %q: %w", libraryID, domain.ErrNotFound)
	}

	return service.rows, service.err
}

func (service *fakeRows) requestedLimits() []int {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	return append([]int(nil), service.limits...)
}

// TestRowsRoutesWithoutService runs the rows parameters through the generated router of an
// API without a rows service: invalid limits answer 400 with the field, valid requests 501.
func TestRowsRoutesWithoutService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		target     string
		wantStatus int
		wantCode   string
		wantField  string
	}{
		{target: homeRowsPath, wantStatus: http.StatusNotImplemented, wantCode: "not_implemented"},
		{target: homeRowsPath + "?limit=50", wantStatus: http.StatusNotImplemented, wantCode: "not_implemented"},
		{target: homeRowsPath + "?limit=0", wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "limit"},
		{target: homeRowsPath + "?limit=51", wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "limit"},
		{target: homeRowsPath + "?limit=abc", wantStatus: http.StatusBadRequest, wantCode: "bad_request"},
		{target: libraryPath + "/rows?limit=1", wantStatus: http.StatusNotImplemented, wantCode: "not_implemented"},
		{target: libraryPath + "/rows?limit=-1", wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "limit"},
		{target: "/api/v1/libraries/not-a-uuid/rows", wantStatus: http.StatusBadRequest, wantCode: "bad_request"},
	}

	for _, test := range tests {
		t.Run(test.target, func(t *testing.T) {
			t.Parallel()

			handler, _ := newLibraryHandler(t, &fakeLibraryService{}, &fakeScanService{})

			problem := assertProblem(t, serve(t, handler, http.MethodGet, test.target), test.wantStatus, test.wantCode)

			field := ""
			if problem.Field != nil {
				field = *problem.Field
			}

			if field != test.wantField {
				t.Errorf("field = %q, want %q", field, test.wantField)
			}
		})
	}
}

func newRowsHandler(t *testing.T, service *fakeRows) http.Handler {
	t.Helper()

	handler, _ := newTestHandlerWithOptions(t, restapi.Options{Rows: service})

	return handler
}

// rowsFixture is a season card of a "recentlyAdded" row and an episode card of a
// "recentEpisodes" row.
func rowsFixture() []approws.Row {
	poster := &artwork.Ref{ID: rowImageID, Tag: "tag"}
	season := applibrary.ItemSummary{
		Item:   library.Item{ID: rowSeasonID, LibraryID: testLibraryID, ParentID: rowShowID, Kind: library.ItemKindSeason, Title: "Season 3", Number: library.NumberOf(3)},
		Poster: poster,
	}
	episode := applibrary.ItemSummary{
		Item:   library.Item{ID: rowEpisodeID, LibraryID: testLibraryID, ParentID: rowSeasonID, Kind: library.ItemKindEpisode, Title: "Pilot", Number: library.NumberOf(7)},
		Poster: poster,
	}

	return []approws.Row{
		{
			ID: rows.RowID(rows.KindRecentlyAdded, testLibraryID), Kind: rows.KindRecentlyAdded, TitleKey: rows.TitleRecentlyAddedIn,
			TitleParams: map[string]string{rows.ParamLibrary: "Shows"}, LibraryID: testLibraryID, CardFormat: rows.CardPoster,
			SeeAll: library.ListingQuery{Filter: library.ListingFilter{Kind: library.ItemKindShow}, Order: library.ListingOrder{Sort: library.SortByDateAdded, Descending: true}},
			Items:  []approws.Item{{Summary: season, ShowTitle: "Elephants Dream", NewEpisodeCount: 2}},
		},
		{
			ID: rows.GenreRowID(testLibraryID, "Drama"), Kind: rows.KindMoreGenre, TitleKey: rows.TitleMoreGenre, Genre: "Drama",
			LibraryID: testLibraryID, CardFormat: rows.CardEpisode,
			SeeAll: library.ListingQuery{Filter: library.ListingFilter{Genres: []string{"Drama"}}, Order: library.ListingOrder{Sort: library.SortByRandom, Seed: 20261105, HasSeed: true}},
			Items:  []approws.Item{{Summary: episode, SeasonNumber: library.NumberOf(3), ShowTitle: "Elephants Dream"}},
		},
	}
}

func TestGetHomeRows(t *testing.T) {
	t.Parallel()

	service := &fakeRows{home: approws.Home{
		Rows: rowsFixture(),
		Hero: []approws.HeroItem{{
			ItemID: rowShowID, LibraryID: testLibraryID, Kind: library.ItemKindShow, Title: "Elephants Dream",
			Year: library.NumberOf(2006), SummaryLine: "Two men.", Backdrop: artwork.Ref{ID: rowImageID, Tag: "b"},
		}},
	}}

	got := serve(t, newRowsHandler(t, service), http.MethodGet, homeRowsPath+"?limit=7")
	assertJSONResponse(t, got, http.StatusOK)

	home := decodeStrictly[apigen.HomeRows](t, got.body)
	if len(home.Rows) != 2 || len(home.Hero) != 1 || home.Hint != nil {
		t.Fatalf("home = %+v, want two rows, one hero, no hint", home)
	}

	assertSeasonCardRow(t, home.Rows[0])
	assertGenreRow(t, home.Rows[1])

	if hero := home.Hero[0]; hero.ItemID.String() != rowShowID || hero.Backdrop.Tag != "b" || hero.Logo != nil || *hero.SummaryLine != "Two men." || *hero.Year != 2006 {
		t.Errorf("hero = %+v", hero)
	}

	if limits := service.requestedLimits(); len(limits) != 1 || limits[0] != 7 {
		t.Errorf("limits = %v, want [7]", limits)
	}
}

func assertSeasonCardRow(t *testing.T, row apigen.Row) {
	t.Helper()

	query := row.SeeAllQuery
	if row.Kind != apigen.RowKindRecentlyAdded || (*row.TitleParams)["library"] != "Shows" || row.LibraryID.String() != testLibraryID ||
		*query.Sort != apigen.ItemSortDateAdded || *query.Order != apigen.SortOrderDesc || *query.Type != apigen.ItemKindShow || query.Seed != nil {
		t.Errorf("recentlyAdded row = %+v, seeAll %+v", row, query)
	}

	card := row.Items[0]
	if *card.ShowTitle != "Elephants Dream" || *card.NewEpisodeCount != 2 || card.SeasonNumber != nil || *card.Number != 3 {
		t.Errorf("season card = %+v", card)
	}
}

func assertGenreRow(t *testing.T, row apigen.Row) {
	t.Helper()

	query := row.SeeAllQuery
	if row.Kind != apigen.RowKindMoreGenre || *row.Genre != "Drama" || row.CardFormat != apigen.CardFormatEpisode || row.TitleParams != nil ||
		*query.Sort != apigen.ItemSortRandom || query.Order != nil || *query.Seed != 20261105 || (*query.Genre)[0] != "Drama" || query.Type != nil {
		t.Errorf("genre row = %+v, seeAll %+v", row, query)
	}

	card := row.Items[0]
	if *card.SeasonNumber != 3 || *card.ShowTitle != "Elephants Dream" || card.NewEpisodeCount != nil {
		t.Errorf("episode card = %+v", card)
	}
}

func TestListLibraryRows(t *testing.T) {
	t.Parallel()

	service := &fakeRows{rows: approws.LibraryRows{LibraryID: testLibraryID, Rows: []approws.Row{}, Hint: rows.HintScanning}}

	got := serve(t, newRowsHandler(t, service), http.MethodGet, libraryPath+"/rows")
	assertJSONResponse(t, got, http.StatusOK)

	libraryRows := decodeStrictly[apigen.LibraryRows](t, got.body)
	if libraryRows.LibraryID.String() != testLibraryID || len(libraryRows.Rows) != 0 || libraryRows.Rows == nil ||
		*libraryRows.Hint != apigen.RowsHintScanning {
		t.Errorf("library rows = %+v", libraryRows)
	}

	if limits := service.requestedLimits(); len(limits) != 1 || limits[0] != 0 {
		t.Errorf("limits = %v, want [0] (the service default)", limits)
	}
}

func TestRowsFailures(t *testing.T) {
	t.Parallel()

	broken := rowsFixture()
	broken[0].Kind = "unknownKind"

	tests := []struct {
		name       string
		service    *fakeRows
		target     string
		wantStatus int
		wantCode   string
	}{
		{name: "unknown library", service: &fakeRows{}, target: librariesPath + "/" + rowShowID + "/rows", wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{name: "library rows failure", service: &fakeRows{err: errors.New("boom")}, target: libraryPath + "/rows", wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
		{name: "home failure", service: &fakeRows{err: fmt.Errorf("wrapped: %w", domain.ErrNotFound)}, target: homeRowsPath, wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
		{name: "kind outside the contract", service: &fakeRows{home: approws.Home{Rows: broken}}, target: homeRowsPath, wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assertProblem(t, serve(t, newRowsHandler(t, test.service), http.MethodGet, test.target), test.wantStatus, test.wantCode)
		})
	}
}
