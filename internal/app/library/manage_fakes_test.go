package library_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	domainlibrary "github.com/magpeek/mivo/internal/domain/library"
)

// callLog records the order of side effects across the fakes.
type callLog struct {
	mutex sync.Mutex
	calls []string
}

func (log *callLog) add(call string) {
	log.mutex.Lock()
	defer log.mutex.Unlock()

	log.calls = append(log.calls, call)
}

func (log *callLog) all() []string {
	log.mutex.Lock()
	defer log.mutex.Unlock()

	return slices.Clone(log.calls)
}

// memoryManaged implements ManagedWriter, ManagedReader and ImportLedger in memory.
type memoryManaged struct {
	log       *callLog
	libraries []domainlibrary.ManagedLibrary
	imported  map[string]bool
	nextID    int
	createErr error
	updateErr error
}

func newMemoryManaged(log *callLog, libraries ...domainlibrary.ManagedLibrary) *memoryManaged {
	return &memoryManaged{log: log, libraries: libraries, imported: map[string]bool{}}
}

func (store *memoryManaged) CreateLibrary(_ context.Context, candidate domainlibrary.ManagedLibrary,
) (domainlibrary.ManagedLibrary, error) {
	store.log.add("store.create")

	if store.createErr != nil {
		return domainlibrary.ManagedLibrary{}, store.createErr
	}

	store.nextID++
	candidate.ID = fmt.Sprintf("new-%d", store.nextID)
	candidate.Folders = slices.Clone(candidate.Folders)

	for index := range candidate.Folders {
		candidate.Folders[index].ID = fmt.Sprintf("%s-f%d", candidate.ID, index)
	}

	store.libraries = append(store.libraries, candidate)

	return candidate, nil
}

func (store *memoryManaged) UpdateLibrary(_ context.Context, updated domainlibrary.ManagedLibrary) error {
	store.log.add("store.update")

	if store.updateErr != nil {
		return store.updateErr
	}

	for index, stored := range store.libraries {
		if stored.ID == updated.ID {
			store.libraries[index] = updated

			return nil
		}
	}

	return domain.ErrNotFound
}

func (store *memoryManaged) DeleteLibrary(_ context.Context, id string) error {
	store.log.add("store.delete")

	for index, stored := range store.libraries {
		if stored.ID == id {
			store.libraries = slices.Delete(store.libraries, index, index+1)

			return nil
		}
	}

	return domain.ErrNotFound
}

func (store *memoryManaged) ListManaged(context.Context) ([]domainlibrary.ManagedLibrary, error) {
	return slices.Clone(store.libraries), nil
}

func (store *memoryManaged) GetManaged(_ context.Context, id string) (domainlibrary.ManagedLibrary, error) {
	for _, stored := range store.libraries {
		if stored.ID == id {
			return stored, nil
		}
	}

	return domainlibrary.ManagedLibrary{}, fmt.Errorf("get %s: %w", id, domain.ErrNotFound)
}

func (store *memoryManaged) NeverImported(_ context.Context, roots []string) ([]string, error) {
	var pending []string

	for _, root := range roots {
		if !store.imported[root] {
			pending = append(pending, root)
		}
	}

	return pending, nil
}

func (store *memoryManaged) RecordImported(_ context.Context, roots []string) error {
	for _, root := range roots {
		store.imported[root] = true
	}

	return nil
}

// fakeBrowser maps "rootID:path" to a result; unknown pairs are invalid.
type fakeBrowser struct {
	roots   []library.BrowseRoot
	checks  map[string]error
	listing library.FolderListing
}

func (browser *fakeBrowser) Roots(context.Context) ([]library.BrowseRoot, error) {
	return browser.roots, nil
}

func (browser *fakeBrowser) List(context.Context, string, string) (library.FolderListing, error) {
	return browser.listing, nil
}

func (browser *fakeBrowser) Check(_ context.Context, rootID, relativePath string) (string, error) {
	for _, root := range browser.roots {
		if root.ID != rootID {
			continue
		}

		key := rootID + ":" + relativePath
		if err, found := browser.checks[key]; found && err != nil {
			return "", err
		}

		return strings.TrimSuffix(root.Path, "/") + "/" + relativePath, nil
	}

	return "", domain.ErrInvalid
}

// fakeScans records scan control calls into the shared log.
type fakeScans struct {
	log      *callLog
	startErr error
	waitErr  error
}

func (scans *fakeScans) Start(_ context.Context, id string) error {
	scans.log.add("scan.start " + id)

	return scans.startErr
}

func (scans *fakeScans) Cancel(id string) { scans.log.add("scan.cancel " + id) }

func (scans *fakeScans) WaitIdle(_ context.Context, id string) error {
	scans.log.add("scan.wait " + id)

	return scans.waitErr
}

// fakeEvents records library events into the shared log.
type fakeEvents struct{ log *callLog }

func (events fakeEvents) Added(id string)   { events.log.add("event.added " + id) }
func (events fakeEvents) Changed(id string) { events.log.add("event.changed " + id) }
func (events fakeEvents) Removed(id string) { events.log.add("event.removed " + id) }
