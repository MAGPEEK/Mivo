package restapi_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

const childrenPath = itemPath + "/children"

// TestListItemChildren pins the paging parameters the handler passes on and the answer for a
// page of children.
func TestListItemChildren(t *testing.T) {
	t.Parallel()

	season := applibrary.ItemSummary{
		Item: library.Item{
			ID: testSecondItemID, LibraryID: testLibraryID, ParentID: testItemID,
			Kind: library.ItemKindSeason, Title: "Specials", Number: library.NumberOf(0),
		},
		ChildCount: 3,
	}

	tests := []struct {
		name   string
		target string
		want   applibrary.ChildPageRequest
	}{
		{name: "defaults", target: childrenPath},
		{name: "every parameter", target: childrenPath + "?limit=25&cursor=abc&order=desc", want: applibrary.ChildPageRequest{Page: library.PageRequest{Limit: 25, Cursor: "abc"}, Descending: true}},
		{name: "explicit asc", target: childrenPath + "?order=asc"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			items := &fakeLibraryService{children: applibrary.ItemPage{Items: []applibrary.ItemSummary{season}, NextCursor: "next"}}
			handler, _ := newLibraryHandler(t, items, &fakeScanService{})

			got := serve(t, handler, http.MethodGet, test.target)
			assertJSONResponse(t, got, http.StatusOK)

			page := decodeStrictly[apigen.ItemPage](t, got.body)
			if len(page.Items) != 1 || page.NextCursor == nil || *page.NextCursor != "next" {
				t.Errorf("page = %+v, want one item and cursor next", page)
			}

			assertBodyContains(t, got.body, `"number":0`, `"kind":"season"`, `"childCount":3`, `"parentId":"`+testItemID+`"`)
			assertBodyLacks(t, got.body, `"media"`)

			if requests := items.childRequests(); len(requests) != 1 || requests[0] != test.want {
				t.Errorf("child requests = %+v, want [%+v]", requests, test.want)
			}
		})
	}
}

func TestListItemChildrenRejectsBadParameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		target     string
		wantStatus int
		wantCode   string
	}{
		{name: "limit zero", target: childrenPath + "?limit=0", wantStatus: http.StatusBadRequest, wantCode: restapi.CodeInvalid},
		{name: "limit too large", target: childrenPath + "?limit=501", wantStatus: http.StatusBadRequest, wantCode: restapi.CodeInvalid},
		{name: "empty cursor", target: childrenPath + "?cursor=", wantStatus: http.StatusBadRequest, wantCode: restapi.CodeInvalid},
		{name: "cursor too long", target: childrenPath + "?cursor=" + strings.Repeat("a", maxCursorLength+1), wantStatus: http.StatusBadRequest, wantCode: restapi.CodeInvalid},
		{name: "unknown order", target: childrenPath + "?order=sideways", wantStatus: http.StatusBadRequest, wantCode: restapi.CodeInvalid},
		{name: "malformed item id", target: "/api/v1/items/not-a-uuid/children", wantStatus: http.StatusBadRequest, wantCode: restapi.CodeBadRequest},
		{name: "malformed limit", target: childrenPath + "?limit=many", wantStatus: http.StatusBadRequest, wantCode: restapi.CodeBadRequest},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			items := &fakeLibraryService{}
			handler, _ := newLibraryHandler(t, items, &fakeScanService{})

			got := serve(t, handler, http.MethodGet, test.target)
			assertProblem(t, got, test.wantStatus, test.wantCode)

			if len(items.childRequests()) != 0 {
				t.Errorf("item catalog called with %v, want no call", items.childRequests())
			}
		})
	}
}

func TestListItemChildrenMapsUseCaseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "unknown item", err: fmt.Errorf("list children: %w", domain.ErrNotFound), wantStatus: http.StatusNotFound, wantCode: restapi.CodeNotFound},
		{name: "foreign cursor", err: fmt.Errorf("list children: %w", domain.ErrInvalid), wantStatus: http.StatusBadRequest, wantCode: restapi.CodeInvalid},
		{name: "server fault", err: errors.New("disk on fire"), wantStatus: http.StatusInternalServerError, wantCode: restapi.CodeInternal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, _ := newLibraryHandler(t, &fakeLibraryService{childrenErr: test.err}, &fakeScanService{})

			got := serve(t, handler, http.MethodGet, childrenPath)
			assertProblem(t, got, test.wantStatus, test.wantCode)
		})
	}
}
