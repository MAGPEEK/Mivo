package restapi_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	appsearch "github.com/magpeek/mivo/internal/app/search"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
)

const catalogSearchPath = "/api/v1/search"

// fakeSearch answers a fixed page and records the requests it got.
type fakeSearch struct {
	mutex    sync.Mutex
	requests []appsearch.Request
	page     appsearch.Page
	err      error
}

func (service *fakeSearch) Search(_ context.Context, request appsearch.Request) (appsearch.Page, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	service.requests = append(service.requests, request)

	return service.page, service.err
}

func (service *fakeSearch) received() []appsearch.Request {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	return append([]appsearch.Request(nil), service.requests...)
}

func newSearchHandler(t *testing.T, service *fakeSearch) http.Handler {
	t.Helper()

	handler, _ := newTestHandlerWithOptions(t, restapi.Options{Search: service})

	return handler
}

func searchTarget(values url.Values) string {
	return catalogSearchPath + "?" + values.Encode()
}

func searchFixture() appsearch.Page {
	return appsearch.Page{
		Items: []applibrary.ItemSummary{
			{
				Item:   library.Item{ID: rowShowID, LibraryID: testLibraryID, Kind: library.ItemKindMovie, Title: "The Matrix", Year: library.NumberOf(1999)},
				Poster: &artwork.Ref{ID: rowImageID, Tag: "tag"},
			},
			{
				Item:  library.Item{ID: rowEpisodeID, LibraryID: testLibraryID, ParentID: rowSeasonID, Kind: library.ItemKindEpisode, Title: "Matrix Reboot"},
				Thumb: &artwork.Ref{ID: rowImageID, Tag: "still"},
			},
		},
		NextCursor: "next-page",
	}
}

func TestSearchAnswersHits(t *testing.T) {
	t.Parallel()

	service := &fakeSearch{page: searchFixture()}

	target := searchTarget(url.Values{"q": {"  matr "}, "type": {"movies"}, "limit": {"2"}, "cursor": {"page-2"}})
	got := serve(t, newSearchHandler(t, service), http.MethodGet, target)
	assertJSONResponse(t, got, http.StatusOK)

	results := decodeStrictly[apigen.SearchResults](t, got.body)
	if len(results.Items) != 2 || results.NextCursor == nil || *results.NextCursor != "next-page" {
		t.Fatalf("results = %+v, want two hits and the next cursor", results)
	}

	movie := results.Items[0]
	if movie.ID.String() != rowShowID || movie.LibraryID.String() != testLibraryID || movie.Kind != apigen.ItemKindMovie ||
		movie.Year == nil || *movie.Year != 1999 || movie.Poster == nil || movie.Poster.ID.String() != rowImageID {
		t.Errorf("movie hit = %+v, want id, library, kind, year and poster", movie)
	}

	if episode := results.Items[1]; episode.Kind != apigen.ItemKindEpisode || episode.Thumb == nil || episode.Year != nil {
		t.Errorf("episode hit = %+v, want a thumb and no year", episode)
	}

	requests := service.received()
	if len(requests) != 1 {
		t.Fatalf("service got %d requests, want 1", len(requests))
	}

	request := requests[0]
	if request.Query != "matr" || request.Type != appsearch.TypeMovies || request.Limit != 2 || request.Cursor != "page-2" {
		t.Errorf("request = %+v", request)
	}
}

func TestSearchDefaultsToTopWithoutLimit(t *testing.T) {
	t.Parallel()

	service := &fakeSearch{page: appsearch.Page{Items: []applibrary.ItemSummary{}}}

	got := serve(t, newSearchHandler(t, service), http.MethodGet, searchTarget(url.Values{"q": {"é!"}}))
	assertJSONResponse(t, got, http.StatusOK)

	results := decodeStrictly[apigen.SearchResults](t, got.body)
	if results.Items == nil || len(results.Items) != 0 || results.NextCursor != nil {
		t.Errorf("results = %+v, want an empty list without cursor", results)
	}

	if !strings.Contains(string(got.body), `"items":[]`) {
		t.Errorf("body = %s, want an items array", got.body)
	}

	requests := service.received()
	if len(requests) != 1 || requests[0].Type != appsearch.TypeTop || requests[0].Limit != 0 || requests[0].Cursor != "" {
		t.Errorf("requests = %+v, want one top request with the default limit", requests)
	}
}

// TestSearchRejectsParameters checks every parameter rule of the contract: the problem is 400
// with the parameter as field, and the service is never asked.
func TestSearchRejectsParameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		values    url.Values
		wantCode  string
		wantField string
	}{
		{name: "missing q", values: url.Values{"type": {"top"}}, wantCode: "bad_request"},
		{name: "empty q", values: url.Values{"q": {""}}, wantCode: "invalid", wantField: "q"},
		{name: "one character", values: url.Values{"q": {"m"}}, wantCode: "invalid", wantField: "q"},
		{name: "one character in spaces", values: url.Values{"q": {"  m  "}}, wantCode: "invalid", wantField: "q"},
		{name: "one multi-byte character", values: url.Values{"q": {"é"}}, wantCode: "invalid", wantField: "q"},
		{name: "q too long", values: url.Values{"q": {strings.Repeat("a", 201)}}, wantCode: "invalid", wantField: "q"},
		{name: "unknown type", values: url.Values{"q": {"matr"}, "type": {"people"}}, wantCode: "invalid", wantField: "type"},
		{name: "limit zero", values: url.Values{"q": {"matr"}, "limit": {"0"}}, wantCode: "invalid", wantField: "limit"},
		{name: "limit too large", values: url.Values{"q": {"matr"}, "limit": {"51"}}, wantCode: "invalid", wantField: "limit"},
		{name: "limit not a number", values: url.Values{"q": {"matr"}, "limit": {"many"}}, wantCode: "bad_request"},
		{name: "empty cursor", values: url.Values{"q": {"matr"}, "type": {"movies"}, "cursor": {""}}, wantCode: "invalid", wantField: "cursor"},
		{name: "cursor too long", values: url.Values{"q": {"matr"}, "type": {"movies"}, "cursor": {strings.Repeat("a", 2049)}}, wantCode: "invalid", wantField: "cursor"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			service := &fakeSearch{}

			got := serve(t, newSearchHandler(t, service), http.MethodGet, searchTarget(test.values))
			assertProblemField(t, assertProblem(t, got, http.StatusBadRequest, test.wantCode), test.wantField)

			if requests := service.received(); len(requests) != 0 {
				t.Errorf("service got %+v for a rejected request", requests)
			}
		})
	}
}

// TestSearchMapsServiceFailures checks the problems of what the use case rejects and of
// server faults; a fault never leaks its text.
func TestSearchMapsServiceFailures(t *testing.T) {
	t.Parallel()

	brokenKind := searchFixture()
	brokenKind.Items[0].Item.Kind = "unknownKind"

	tests := []struct {
		name       string
		service    *fakeSearch
		wantStatus int
		wantCode   string
		wantField  string
	}{
		{name: "query too short", service: &fakeSearch{err: fmt.Errorf("search: %w", appsearch.ErrQueryTooShort)}, wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "q"},
		{name: "unknown type", service: &fakeSearch{err: fmt.Errorf("search: %w", appsearch.ErrUnknownType)}, wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "type"},
		{name: "invalid cursor", service: &fakeSearch{err: fmt.Errorf("search: %w", appsearch.ErrInvalidCursor)}, wantStatus: http.StatusBadRequest, wantCode: "invalid", wantField: "cursor"},
		{name: "index failure", service: &fakeSearch{err: errors.New("secret-detail")}, wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
		{name: "stored data failure", service: &fakeSearch{err: fmt.Errorf("secret-detail: %w", domain.ErrInvalid)}, wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
		{name: "kind outside the contract", service: &fakeSearch{page: brokenKind}, wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			target := searchTarget(url.Values{"q": {"matr"}, "type": {"movies"}, "cursor": {"page-2"}})
			got := serve(t, newSearchHandler(t, test.service), http.MethodGet, target)

			assertProblemField(t, assertProblem(t, got, test.wantStatus, test.wantCode), test.wantField)
			assertNoInternalText(t, got, "secret-detail")
		})
	}
}

// TestSearchWithoutService runs the parameters through an API without a search service:
// invalid ones still answer 400, a valid request 501.
func TestSearchWithoutService(t *testing.T) {
	t.Parallel()

	handler, _ := newTestHandlerWithOptions(t, restapi.Options{})

	assertProblem(t, serve(t, handler, http.MethodGet, searchTarget(url.Values{"q": {"matr"}})), http.StatusNotImplemented, "not_implemented")

	got := serve(t, handler, http.MethodGet, searchTarget(url.Values{"q": {"m"}}))
	assertProblemField(t, assertProblem(t, got, http.StatusBadRequest, "invalid"), "q")
}
