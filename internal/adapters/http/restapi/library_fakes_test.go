package restapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	appscan "github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/library"
)

// The use cases cmd/mivo wires in must keep satisfying the ports the handlers declare.
var (
	_ restapi.LibraryService = (*applibrary.Service)(nil)
	_ restapi.ScanService    = (*appscan.Service)(nil)
)

// fakeLibraryService is an in-memory LibraryService that records the arguments it got.
type fakeLibraryService struct {
	mutex sync.Mutex

	overviews []applibrary.Overview
	// found, when set, is what GetLibrary returns for every ID.
	found   *applibrary.Overview
	listErr error
	getErr  error
	page    applibrary.ItemPage
	pageErr error
	detail  applibrary.ItemDetail
	itemErr error

	requestedLibraries []string
	requestedPages     []library.PageRequest
	requestedItems     []string
}

func (service *fakeLibraryService) ListLibraries(context.Context) ([]applibrary.Overview, error) {
	return service.overviews, service.listErr
}

func (service *fakeLibraryService) GetLibrary(_ context.Context, id string) (applibrary.Overview, error) {
	service.record(&service.requestedLibraries, id)

	if service.getErr != nil {
		return applibrary.Overview{}, service.getErr
	}

	if service.found != nil {
		return *service.found, nil
	}

	for _, overview := range service.overviews {
		if overview.Library.ID == id {
			return overview, nil
		}
	}

	return applibrary.Overview{}, errLibraryNotFound(id)
}

func (service *fakeLibraryService) ListItems(_ context.Context, libraryID string, page library.PageRequest) (applibrary.ItemPage, error) {
	service.record(&service.requestedLibraries, libraryID)

	service.mutex.Lock()
	service.requestedPages = append(service.requestedPages, page)
	service.mutex.Unlock()

	return service.page, service.pageErr
}

func (service *fakeLibraryService) GetItem(_ context.Context, itemID string) (applibrary.ItemDetail, error) {
	service.record(&service.requestedItems, itemID)

	return service.detail, service.itemErr
}

func (service *fakeLibraryService) record(target *[]string, id string) {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	*target = append(*target, id)
}

func (service *fakeLibraryService) pages() []library.PageRequest {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	return append([]library.PageRequest(nil), service.requestedPages...)
}

// fakeScanService is an in-memory ScanService. Status answers from statuses and falls back
// to the idle zero status, like app/scan for a library that was never scanned.
type fakeScanService struct {
	mutex sync.Mutex

	statuses    map[string]appscan.Status
	startStatus appscan.Status
	startErr    error
	started     []string
}

func (service *fakeScanService) Start(_ context.Context, libraryID string) (appscan.Status, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	service.started = append(service.started, libraryID)

	return service.startStatus, service.startErr
}

func (service *fakeScanService) Status(libraryID string) appscan.Status {
	status, found := service.statuses[libraryID]
	if !found {
		return appscan.Status{State: appscan.StateIdle}
	}

	return status
}

func (service *fakeScanService) startedIDs() []string {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	return append([]string(nil), service.started...)
}

// newLibraryHandler serves the API with both catalog services wired.
func newLibraryHandler(t *testing.T, libraries *fakeLibraryService, scans *fakeScanService) (http.Handler, *syncBuffer) {
	t.Helper()

	return newTestHandlerWithOptions(t, restapi.Options{Library: libraries, Scan: scans})
}

// decodeStrictly decodes body into T and fails on fields T does not declare, so the JSON
// shape is checked against the generated contract types.
func decodeStrictly[T any](t *testing.T, body []byte) T {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()

	var value T
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("decode %s strictly: %v", body, err)
	}

	return value
}

// assertJSONResponse checks the status and the JSON content type of a success response.
func assertJSONResponse(t *testing.T, got response, wantStatus int) {
	t.Helper()

	if got.status != wantStatus {
		t.Fatalf("status = %d, want %d (body %s)", got.status, wantStatus, got.body)
	}

	if contentType := got.header.Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
}

// assertBodyContains checks raw JSON fragments the decoded types cannot tell apart, for
// example an empty array versus null or an omitted field versus its zero value.
func assertBodyContains(t *testing.T, body []byte, fragments ...string) {
	t.Helper()

	for _, fragment := range fragments {
		if !strings.Contains(string(body), fragment) {
			t.Errorf("body %s does not contain %s", body, fragment)
		}
	}
}

func assertBodyLacks(t *testing.T, body []byte, fragments ...string) {
	t.Helper()

	for _, fragment := range fragments {
		if strings.Contains(string(body), fragment) {
			t.Errorf("body %s contains %s", body, fragment)
		}
	}
}
