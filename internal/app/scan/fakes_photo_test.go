package scan_test

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/photo"
)

const (
	photosID   = "lib-photos"
	photosRoot = "photos"
)

// infoOutcome is the scripted result of reading the metadata of one absolute path.
type infoOutcome struct {
	info photo.Info
	err  error
}

// scriptedPhotoMetadata returns scripted metadata per absolute path (default: a zero Info),
// records reads and the largest number of concurrent reads. When gate is set, every read
// signals entered (if set) and waits for a token from gate or for the end of its context.
type scriptedPhotoMetadata struct {
	mutex     sync.Mutex
	outcomes  map[string]infoOutcome
	read      []string
	active    int
	maxActive int
	gate      chan struct{}
	entered   chan struct{}
}

func newScriptedPhotoMetadata() *scriptedPhotoMetadata {
	return &scriptedPhotoMetadata{outcomes: map[string]infoOutcome{}}
}

func (reader *scriptedPhotoMetadata) script(relativePath string, outcome infoOutcome) {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()

	reader.outcomes[absolutePath(photosRoot, relativePath)] = outcome
}

func (reader *scriptedPhotoMetadata) ReadInfo(ctx context.Context, path string) (photo.Info, error) {
	reader.mutex.Lock()
	reader.read = append(reader.read, path)
	reader.active++
	reader.maxActive = max(reader.maxActive, reader.active)
	outcome := reader.outcomes[path]
	gate, entered := reader.gate, reader.entered
	reader.mutex.Unlock()

	defer func() {
		reader.mutex.Lock()
		reader.active--
		reader.mutex.Unlock()
	}()

	if err := waitAtGate(ctx, gate, entered); err != nil {
		return photo.Info{}, err
	}

	return outcome.info, outcome.err
}

func waitAtGate(ctx context.Context, gate, entered chan struct{}) error {
	if entered != nil {
		select {
		case entered <- struct{}{}:
		case <-ctx.Done():
		}
	}

	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return fmt.Errorf("read info: %w", ctx.Err())
		}
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("read info: %w", err)
	}

	return nil
}

// takeReads returns the sorted relative paths read so far and forgets them.
func (reader *scriptedPhotoMetadata) takeReads() []string {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()

	paths := make([]string, 0, len(reader.read))
	for _, path := range reader.read {
		paths = append(paths, path[len(fakeRootPrefix+photosRoot+"/"):])
	}

	reader.read = nil

	return slices.Sorted(slices.Values(paths))
}

func (reader *scriptedPhotoMetadata) maxConcurrent() int {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()

	return reader.maxActive
}

// memoryPhotoCatalog keeps the details per item ID; err fails every write while set.
type memoryPhotoCatalog struct {
	mutex   sync.Mutex
	details map[string]scan.PhotoDetails
	writes  int
	err     error
}

func newMemoryPhotoCatalog() *memoryPhotoCatalog {
	return &memoryPhotoCatalog{details: map[string]scan.PhotoDetails{}}
}

func (catalog *memoryPhotoCatalog) SavePhotoDetails(_ context.Context, details scan.PhotoDetails) error {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	if catalog.err != nil {
		return catalog.err
	}

	if err := details.Info.Validate(); err != nil {
		return fmt.Errorf("save photo details: %w", err)
	}

	catalog.writes++
	catalog.details[details.ItemID] = details

	return nil
}

func (catalog *memoryPhotoCatalog) of(itemID string) (scan.PhotoDetails, bool) {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	details, found := catalog.details[itemID]

	return details, found
}

func (catalog *memoryPhotoCatalog) setErr(err error) {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	catalog.err = err
}

func (catalog *memoryPhotoCatalog) writeCount() int {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	return catalog.writes
}

// newPhotoHarness is a harness with a photos library and photo ports.
func newPhotoHarness(t *testing.T) (*harness, *scriptedPhotoMetadata, *memoryPhotoCatalog) {
	t.Helper()

	h := newHarness(t)
	reader, catalog := newScriptedPhotoMetadata(), newMemoryPhotoCatalog()
	h.photos.Metadata, h.photos.Catalog = reader, catalog
	h.libraries.set(oneFolder(photosID, "Photos", photosRoot, library.KindPhotos))

	return h, reader, catalog
}

// scanPhotos scans the photos library with the given files and returns the final status.
func (h *harness) scanPhotos(t *testing.T, files ...library.FileState) scan.Status {
	t.Helper()

	h.walker.set(photosRoot, walkResult{files: files})

	return h.scan(t, photosID)
}

// detailsOf returns the stored details of the photo item of the file at path.
func detailsOf(t *testing.T, h *harness, catalog *memoryPhotoCatalog, path string) scan.PhotoDetails {
	t.Helper()

	itemID := h.catalogV2.itemOf(path)
	if itemID == "" {
		t.Fatalf("no item stored for %s", path)
	}

	details, found := catalog.of(itemID)
	if !found {
		t.Fatalf("no photo details stored for %s (item %s)", path, itemID)
	}

	return details
}
