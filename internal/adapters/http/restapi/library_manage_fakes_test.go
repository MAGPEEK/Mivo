package restapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/library"
)

const (
	testRootID  = "0c1d2e3f-4a5b-4c6d-8e7f-9a0b1c2d3e4f"
	rootsPath   = "/api/v1/folder-roots"
	foldersPath = rootsPath + "/" + testRootID + "/folders"
)

// fakeManager is an in-memory LibraryManager that records what it got.
type fakeManager struct {
	mutex sync.Mutex

	stored    library.ManagedLibrary
	err       error
	created   []applibrary.CreateRequest
	patched   []applibrary.UpdatePatch
	patchedID []string
	removed   []string
}

func (manager *fakeManager) CreateLibrary(_ context.Context, request applibrary.CreateRequest) (library.ManagedLibrary, error) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()

	manager.created = append(manager.created, request)

	return manager.stored, manager.err
}

func (manager *fakeManager) UpdateLibrary(_ context.Context, id string, patch applibrary.UpdatePatch) (library.ManagedLibrary, error) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()

	manager.patchedID = append(manager.patchedID, id)
	manager.patched = append(manager.patched, patch)

	return manager.stored, manager.err
}

func (manager *fakeManager) RemoveLibrary(_ context.Context, id string) error {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()

	manager.removed = append(manager.removed, id)

	return manager.err
}

// fakeBrowsing is an in-memory FolderBrowsing.
type fakeBrowsing struct {
	mutex sync.Mutex

	roots    []applibrary.BrowseRoot
	page     applibrary.FolderPage
	rootsErr error
	err      error
	requests [][2]string
}

func (browsing *fakeBrowsing) BrowseRoots(context.Context) ([]applibrary.BrowseRoot, error) {
	return browsing.roots, browsing.rootsErr
}

func (browsing *fakeBrowsing) BrowseFolders(_ context.Context, rootID, path string) (applibrary.FolderPage, error) {
	browsing.mutex.Lock()
	defer browsing.mutex.Unlock()

	browsing.requests = append(browsing.requests, [2]string{rootID, path})

	return browsing.page, browsing.err
}

// newManagementHandler serves the API with the management ports wired.
func newManagementHandler(t *testing.T, options restapi.Options) (http.Handler, *syncBuffer) {
	t.Helper()

	return newTestHandlerWithOptions(t, options)
}

// jsonRequest builds a request with a JSON body.
func jsonRequest(t *testing.T, method, target, body string) *http.Request {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	return request
}
