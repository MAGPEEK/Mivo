package restapi_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
)

// fakeReanalyzer records the calls and answers err.
type fakeReanalyzer struct {
	err   error
	calls [][2]string
}

func (fake *fakeReanalyzer) Reanalyze(_ context.Context, libraryID, itemID string) error {
	fake.calls = append(fake.calls, [2]string{libraryID, itemID})

	return fake.err
}

func newReanalyzeHandler(t *testing.T, items *fakeLibraryService, reanalyzer restapi.ItemReanalyzer) http.Handler {
	t.Helper()

	handler, _ := newTestHandlerWithOptions(t, restapi.Options{Libraries: items, Items: items, Reanalysis: reanalyzer})

	return handler
}

func TestReanalyzeItemAnswers202AndStartsTheRun(t *testing.T) {
	t.Parallel()

	items := &fakeLibraryService{detail: applibrary.ItemDetail{Item: testItem(testItemID, "Up")}}
	reanalyzer := &fakeReanalyzer{}
	handler := newReanalyzeHandler(t, items, reanalyzer)

	got := serveRequest(t, handler, jsonRequest(t, http.MethodPost, itemPath+"/reanalyze", ""))

	if got.status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", got.status)
	}

	if len(reanalyzer.calls) != 1 || reanalyzer.calls[0] != [2]string{testLibraryID, testItemID} {
		t.Errorf("calls = %v, want one for the item in its library", reanalyzer.calls)
	}
}

func TestReanalyzeItemFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		itemErr    error
		runErr     error
		wantStatus int
		wantCode   string
		wantRun    bool
	}{
		{name: "hidden or unknown item", itemErr: domain.ErrNotFound, wantStatus: http.StatusNotFound, wantCode: restapi.CodeNotFound},
		{name: "item lookup fails", itemErr: errors.New("disk"), wantStatus: http.StatusInternalServerError, wantCode: restapi.CodeInternal},
		{name: "already running", runErr: domain.ErrConflict, wantStatus: http.StatusConflict, wantCode: restapi.CodeConflict, wantRun: true},
		{name: "item without files", runErr: domain.ErrNotFound, wantStatus: http.StatusNotFound, wantCode: restapi.CodeNotFound, wantRun: true},
		{name: "kind without video probe", runErr: domain.ErrInvalid, wantStatus: http.StatusBadRequest, wantCode: restapi.CodeInvalid, wantRun: true},
		{name: "service closed", runErr: domain.ErrUnavailable, wantStatus: http.StatusServiceUnavailable, wantCode: restapi.CodeUnavailable, wantRun: true},
		{name: "unexpected", runErr: errors.New("boom"), wantStatus: http.StatusInternalServerError, wantCode: restapi.CodeInternal, wantRun: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			items := &fakeLibraryService{detail: applibrary.ItemDetail{Item: testItem(testItemID, "Up")}, itemErr: test.itemErr}
			reanalyzer := &fakeReanalyzer{err: test.runErr}
			handler := newReanalyzeHandler(t, items, reanalyzer)

			got := serveRequest(t, handler, jsonRequest(t, http.MethodPost, itemPath+"/reanalyze", ""))
			assertProblem(t, got, test.wantStatus, test.wantCode)

			if ran := len(reanalyzer.calls) > 0; ran != test.wantRun {
				t.Errorf("reanalysis started = %v, want %v (a hidden item must not reach the use case)", ran, test.wantRun)
			}
		})
	}
}

func TestReanalyzeItemWithoutServiceIsNotImplemented(t *testing.T) {
	t.Parallel()

	items := &fakeLibraryService{detail: applibrary.ItemDetail{Item: testItem(testItemID, "Up")}}
	handler := newReanalyzeHandler(t, items, nil)

	got := serveRequest(t, handler, jsonRequest(t, http.MethodPost, itemPath+"/reanalyze", ""))
	assertProblem(t, got, http.StatusNotImplemented, restapi.CodeNotImplemented)
}
