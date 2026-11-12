package restapi_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	appaccess "github.com/magpeek/mivo/internal/app/access"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	approws "github.com/magpeek/mivo/internal/app/rows"
	appsearch "github.com/magpeek/mivo/internal/app/search"
	appwatch "github.com/magpeek/mivo/internal/app/watch"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/rows"
	"github.com/magpeek/mivo/internal/domain/watch"
)

const watchItemPath = "/api/v1/items/" + testItemID

// fakeWatch is an in-memory WatchService.
type fakeWatch struct {
	mutex sync.Mutex

	calls       []string
	positions   []float64
	err         error
	states      map[string]appwatch.ItemState
	statesCalls [][]appwatch.ItemRef
	statesErr   error
}

func (service *fakeWatch) record(call string) error {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	service.calls = append(service.calls, call)

	return service.err
}

func (service *fakeWatch) ReportProgress(_ context.Context, itemID string, position float64) error {
	service.mutex.Lock()
	service.positions = append(service.positions, position)
	service.mutex.Unlock()

	return service.record("progress " + itemID)
}

func (service *fakeWatch) MarkWatched(_ context.Context, itemID string) error {
	return service.record("watched " + itemID)
}

func (service *fakeWatch) MarkUnwatched(_ context.Context, itemID string) error {
	return service.record("unwatched " + itemID)
}

func (service *fakeWatch) HideFromContinueWatching(_ context.Context, itemID string) error {
	return service.record("hide " + itemID)
}

func (service *fakeWatch) States(_ context.Context, items []appwatch.ItemRef) (map[string]appwatch.ItemState, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	service.statesCalls = append(service.statesCalls, append([]appwatch.ItemRef(nil), items...))

	return service.states, service.statesErr
}

func (service *fakeWatch) stateCallCount() int {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	return len(service.statesCalls)
}

func (service *fakeWatch) callList() []string {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	return append([]string(nil), service.calls...)
}

// watchCall is a request to a watch endpoint; a body is sent as JSON.
type watchCall struct{ method, target, body string }

func serveBody(t *testing.T, handler http.Handler, call watchCall) response {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), call.method, call.target, strings.NewReader(call.body))
	if call.body != "" {
		request.Header.Set("Content-Type", "application/json")
	}

	return serveRequest(t, handler, request)
}

// TestWatchOperationsCallTheService: every watch operation of the contract reaches its use
// case with the item ID and answers 204.
func TestWatchOperationsCallTheService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		method string
		suffix string
		body   string
		want   string
	}{
		{http.MethodPut, "/progress", `{"positionSeconds":2530.5,"playbackSessionId":"5d4c3b2a-1f0e-4d9c-8b7a-6f5e4d3c2b1a"}`, "progress " + testItemID},
		{http.MethodPut, "/watched", "", "watched " + testItemID},
		{http.MethodDelete, "/watched", "", "unwatched " + testItemID},
		{http.MethodDelete, "/continue-watching", "", "hide " + testItemID},
	}

	for _, test := range tests {
		t.Run(test.method+" "+test.suffix, func(t *testing.T) {
			t.Parallel()

			service := &fakeWatch{}
			handler, _ := newTestHandlerWithOptions(t, restapi.Options{Watch: service})

			got := serveBody(t, handler, watchCall{test.method, watchItemPath + test.suffix, test.body})
			if got.status != http.StatusNoContent || len(got.body) != 0 {
				t.Fatalf("answer = %d %q, want 204 without a body", got.status, got.body)
			}

			if calls := service.callList(); len(calls) != 1 || calls[0] != test.want {
				t.Errorf("calls = %v, want [%s]", calls, test.want)
			}
		})
	}

	service := &fakeWatch{}
	handler, _ := newTestHandlerWithOptions(t, restapi.Options{Watch: service})
	serveBody(t, handler, watchCall{http.MethodPut, watchItemPath + "/progress", `{"positionSeconds":12.5}`})

	if len(service.positions) != 1 || service.positions[0] != 12.5 {
		t.Errorf("positions = %v, want [12.5]", service.positions)
	}
}

// TestWatchOperationErrors maps the use case errors to the published problems.
func TestWatchOperationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"unknown or hidden item", fmt.Errorf("look up: %w", domain.ErrNotFound), http.StatusNotFound, "not_found"},
		{"kind without watch state", appwatch.ErrNoWatchState, http.StatusBadRequest, "invalid"},
		{"no profile selected", appaccess.ErrProfileRequired, http.StatusForbidden, "profile_required"},
		{"not signed in", fmt.Errorf("viewer: %w", domain.ErrUnauthenticated), http.StatusUnauthorized, "unauthenticated"},
		{"store failure", errors.New("disk full"), http.StatusInternalServerError, "internal_error"},
	}

	for _, test := range tests {
		for _, call := range []struct{ method, suffix, body string }{
			{http.MethodPut, "/progress", `{"positionSeconds":1}`},
			{http.MethodPut, "/watched", ""},
			{http.MethodDelete, "/watched", ""},
			{http.MethodDelete, "/continue-watching", ""},
		} {
			t.Run(test.name+" "+call.method+call.suffix, func(t *testing.T) {
				t.Parallel()

				handler, _ := newTestHandlerWithOptions(t, restapi.Options{Watch: &fakeWatch{err: test.err}})

				assertProblem(t, serveBody(t, handler, watchCall{call.method, watchItemPath + call.suffix, call.body}), test.wantStatus, test.wantCode)
			})
		}
	}
}

// TestWatchOperationsWithoutServiceAnswerNotImplemented keeps test servers that wire no
// watch service working.
func TestWatchOperationsWithoutServiceAnswerNotImplemented(t *testing.T) {
	t.Parallel()

	handler, _ := newTestHandlerWithOptions(t, restapi.Options{})

	for _, call := range []struct{ method, suffix, body string }{
		{http.MethodPut, "/progress", `{"positionSeconds":1}`},
		{http.MethodPut, "/watched", ""},
		{http.MethodDelete, "/watched", ""},
		{http.MethodDelete, "/continue-watching", ""},
	} {
		assertProblem(t, serveBody(t, handler, watchCall{call.method, watchItemPath + call.suffix, call.body}), http.StatusNotImplemented, "not_implemented")
	}
}

// TestProgressReportOutsideTheContractIsRejected: a negative position answers 400 "invalid"
// with the field before the use case runs; JSON has no NaN or Infinity, and a number beyond
// the double range fails to bind with 400 "bad_request".
func TestProgressReportOutsideTheContractIsRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     string
		wantCode string
	}{
		{`{"positionSeconds":-1}`, "invalid"},
		{`{"positionSeconds":1e400}`, "bad_request"},
		{`{"positionSeconds":NaN}`, "bad_request"},
		{`{"positionSeconds":"Infinity"}`, "bad_request"},
	}

	for _, test := range tests {
		t.Run(test.body, func(t *testing.T) {
			t.Parallel()

			service := &fakeWatch{}
			handler, _ := newTestHandlerWithOptions(t, restapi.Options{Watch: service})

			got := serveBody(t, handler, watchCall{http.MethodPut, watchItemPath + "/progress", test.body})
			problem := assertProblem(t, got, http.StatusBadRequest, test.wantCode)

			if test.wantCode == "invalid" {
				assertProblemField(t, problem, "positionSeconds")
			}

			if len(service.callList()) != 0 {
				t.Errorf("calls = %v, want none", service.callList())
			}
		})
	}
}

// TestViewerStateOmitsUnknownFields pins the generated type: lastPlayedAt and unwatchedCount
// are optional, the rest is always present.
func TestViewerStateOmitsUnknownFields(t *testing.T) {
	t.Parallel()

	state := decode[apigen.ViewerState](t, []byte(`{"watched":false,"positionSeconds":0,"playCount":0}`))
	if state.LastPlayedAt != nil || state.UnwatchedCount != nil {
		t.Errorf("state = %+v, want no lastPlayedAt and no unwatchedCount", state)
	}
}

func watchedAt() time.Time { return time.Date(2026, 11, 5, 20, 14, 3, 0, time.UTC) }

// TestItemListingsCarryViewerState: a page of 100 items asks States once, a leaf shows its
// state, a show its unwatchedCount, and an item without state none.
func TestItemListingsCarryViewerState(t *testing.T) {
	t.Parallel()

	const count = 100

	summaries := make([]applibrary.ItemSummary, 0, count)

	for index := range count {
		id := fmt.Sprintf("00000000-0000-4000-8000-%012d", index)
		item := testItem(id, fmt.Sprintf("Item %03d", index))

		if index == 1 {
			item.Kind = library.ItemKindShow
		}

		summaries = append(summaries, applibrary.ItemSummary{Item: item})
	}

	service := &fakeWatch{states: map[string]appwatch.ItemState{
		summaries[0].Item.ID: {State: watch.State{PositionSeconds: 30, PlayCount: 2, LastPlayed: watchedAt()}},
		summaries[1].Item.ID: {State: watch.State{}, Episodes: appwatch.EpisodeCount{Total: 10, Unwatched: 4}},
	}}
	libraries := &fakeLibraryService{page: applibrary.ItemPage{Items: summaries}, children: applibrary.ItemPage{Items: summaries}}

	handler, _ := newTestHandlerWithOptions(t, restapi.Options{Items: libraries, Libraries: libraries, Watch: service})

	for _, target := range []string{itemsPath, watchItemPath + "/children"} {
		got := serve(t, handler, http.MethodGet, target)
		assertJSONResponse(t, got, http.StatusOK)

		page := decode[apigen.ItemPage](t, got.body)

		first, show, third := page.Items[0].ViewerState, page.Items[1].ViewerState, page.Items[2].ViewerState
		if first == nil || first.PositionSeconds != 30 || first.PlayCount != 2 || first.LastPlayedAt == nil || !first.LastPlayedAt.Equal(watchedAt()) {
			t.Errorf("%s: leaf state = %+v", target, first)
		}

		if show == nil || show.UnwatchedCount == nil || *show.UnwatchedCount != 4 || show.Watched {
			t.Errorf("%s: show state = %+v", target, show)
		}

		if third != nil {
			t.Errorf("%s: item without state = %+v, want none", target, third)
		}
	}

	if service.stateCallCount() != 2 {
		t.Fatalf("States called %d times for two responses, want 2", service.stateCallCount())
	}

	if refs := service.statesCalls[0]; len(refs) != count || refs[1].Kind != library.ItemKindShow {
		t.Errorf("first batch has %d refs, want %d with the show kind", len(refs), count)
	}
}

// TestFullyWatchedShowIsWatched: every episode watched sends watched true and unwatchedCount 0.
func TestFullyWatchedShowIsWatched(t *testing.T) {
	t.Parallel()

	item := testItem(testItemID, "Show")
	item.Kind = library.ItemKindShow
	libraries := &fakeLibraryService{detail: applibrary.ItemDetail{Item: item}}
	service := &fakeWatch{states: map[string]appwatch.ItemState{
		testItemID: {State: watch.State{Watched: true}, Episodes: appwatch.EpisodeCount{Total: 3}},
	}}

	handler, _ := newTestHandlerWithOptions(t, restapi.Options{Items: libraries, Libraries: libraries, Watch: service})

	got := serve(t, handler, http.MethodGet, watchItemPath)
	assertJSONResponse(t, got, http.StatusOK)

	state := decode[apigen.ItemDetail](t, got.body).ViewerState
	if state == nil || !state.Watched || state.UnwatchedCount == nil || *state.UnwatchedCount != 0 {
		t.Errorf("state = %+v, want watched with unwatchedCount 0", state)
	}
}

// TestItemDetailAndSearchCarryViewerState covers the detail and the search results.
func TestItemDetailAndSearchCarryViewerState(t *testing.T) {
	t.Parallel()

	item := testItem(testItemID, "Big Buck Bunny")
	service := &fakeWatch{states: map[string]appwatch.ItemState{
		testItemID: {State: watch.State{Watched: true, PlayCount: 1}},
	}}
	libraries := &fakeLibraryService{detail: applibrary.ItemDetail{Item: item}}
	searching := &fakeSearch{page: appsearch.Page{Items: []applibrary.ItemSummary{{Item: item}}}}

	handler, _ := newTestHandlerWithOptions(t, restapi.Options{Items: libraries, Libraries: libraries, Search: searching, Watch: service})

	detail := serve(t, handler, http.MethodGet, watchItemPath)
	assertJSONResponse(t, detail, http.StatusOK)

	if state := decode[apigen.ItemDetail](t, detail.body).ViewerState; state == nil || !state.Watched || state.PlayCount != 1 {
		t.Errorf("detail state = %+v", state)
	}

	results := serve(t, handler, http.MethodGet, "/api/v1/search?q=bunny")
	assertJSONResponse(t, results, http.StatusOK)

	if state := decode[apigen.SearchResults](t, results.body).Items[0].ViewerState; state == nil || !state.Watched {
		t.Errorf("search state = %+v", state)
	}
}

// TestViewerStateLookupFailures: a missing profile stays a 403, anything else a 500.
func TestViewerStateLookupFailures(t *testing.T) {
	t.Parallel()

	libraries := &fakeLibraryService{detail: applibrary.ItemDetail{Item: testItem(testItemID, "x")}}

	tests := []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{appaccess.ErrProfileRequired, http.StatusForbidden, "profile_required"},
		{errors.New("database locked"), http.StatusInternalServerError, "internal_error"},
	}

	for _, test := range tests {
		handler, _ := newTestHandlerWithOptions(t, restapi.Options{
			Items: libraries, Libraries: libraries, Watch: &fakeWatch{statesErr: test.err},
		})

		assertProblem(t, serve(t, handler, http.MethodGet, watchItemPath), test.wantStatus, test.wantCode)
	}
}

// TestRowsCarryViewerStateInOneBatch: watch rows keep the state the rows service set, other
// rows are decorated with a single States call, and the new row kinds are sent.
func TestRowsCarryViewerStateInOneBatch(t *testing.T) {
	t.Parallel()

	episode := testItem(testSecondItemID, "Episode")
	episode.Kind = library.ItemKindEpisode
	resumed := appwatch.ItemState{State: watch.State{PositionSeconds: 90, LastPlayed: watchedAt()}}

	home := approws.Home{Rows: []approws.Row{
		{
			ID: "continue", Kind: rows.KindContinueWatching, TitleKey: rows.TitleContinueWatching, CardFormat: rows.CardEpisode,
			Items: []approws.Item{{Summary: applibrary.ItemSummary{Item: episode}, ViewerState: &resumed}},
		},
		{
			ID: "next", Kind: rows.KindUpNext, TitleKey: rows.TitleUpNext, CardFormat: rows.CardEpisode,
			Items: []approws.Item{{Summary: applibrary.ItemSummary{Item: episode}, ViewerState: &resumed}},
		},
		{
			ID: "recent", Kind: rows.KindRecentlyAdded, TitleKey: "rows.recentlyAdded", CardFormat: rows.CardPoster,
			Items: []approws.Item{{Summary: applibrary.ItemSummary{Item: testItem(testItemID, "Movie")}}},
		},
	}}

	service := &fakeWatch{states: map[string]appwatch.ItemState{testItemID: {State: watch.State{Watched: true}}}}
	handler, _ := newTestHandlerWithOptions(t, restapi.Options{Rows: &fakeRows{home: home}, Watch: service})

	got := serve(t, handler, http.MethodGet, homeRowsPath)
	assertJSONResponse(t, got, http.StatusOK)

	body := decode[apigen.HomeRows](t, got.body)
	if body.Rows[0].Kind != apigen.RowKindContinueWatching || body.Rows[1].Kind != apigen.RowKindUpNext {
		t.Errorf("kinds = %s, %s", body.Rows[0].Kind, body.Rows[1].Kind)
	}

	if state := body.Rows[0].Items[0].ViewerState; state == nil || state.PositionSeconds != 90 {
		t.Errorf("continue watching state = %+v", state)
	}

	if state := body.Rows[2].Items[0].ViewerState; state == nil || !state.Watched {
		t.Errorf("recently added state = %+v", state)
	}

	if service.stateCallCount() != 1 || len(service.statesCalls[0]) != 1 {
		t.Errorf("States calls = %v, want one call for the one undecorated card", service.statesCalls)
	}
}

// TestRowKindsMatchTheContract pins the enum parity of the row kinds of the domain and the
// contract.
func TestRowKindsMatchTheContract(t *testing.T) {
	t.Parallel()

	for _, kind := range []rows.Kind{
		rows.KindRecentlyAdded, rows.KindMoreGenre, rows.KindContinueWatching, rows.KindUpNext,
	} {
		if !apigen.RowKind(kind).Valid() {
			t.Errorf("row kind %q is not in the contract", kind)
		}
	}
}
