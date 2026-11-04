package scan_test

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"sync"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// v2Item is a stored item of the in-memory v2 catalog.
type v2Item struct {
	id, parentID string
	kind         library.ItemKind
	matchKey     string
}

// v2File is a stored file of the in-memory v2 catalog.
type v2File struct {
	id, itemID   string
	placementKey string
	state        library.FileState
}

// memoryCatalogV2 enforces the contract scan.PlacementCatalog states for the sqlite writer:
// chain upsert by (parent, kind, match key), stable item IDs, bottom-up removal of emptied
// ancestors and idempotent sidecar writes.
type memoryCatalogV2 struct {
	mutex    sync.Mutex
	items    map[string]*v2Item
	files    map[string]*v2File
	sidecars map[string]library.FileState
	saves    int
	moves    int
	// removeErr, when set, fails RemovePlacedFiles.
	removeErr error
	// sidecarWrites counts SaveSidecarStates calls.
	sidecarWrites int
}

func newMemoryCatalogV2() *memoryCatalogV2 {
	return &memoryCatalogV2{
		items: map[string]*v2Item{}, files: map[string]*v2File{}, sidecars: map[string]library.FileState{},
	}
}

func (catalog *memoryCatalogV2) PlacedFiles(_ context.Context, _ string) ([]scan.PlacedFileState, error) {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	var result []scan.PlacedFileState
	for _, file := range catalog.files {
		result = append(result, scan.PlacedFileState{
			ID: file.id, PlacementKey: file.placementKey, ItemIDs: catalog.chainOf(file.itemID), State: file.state,
		})
	}

	sort.Slice(result, func(i, j int) bool { return result[i].State.RelativePath < result[j].State.RelativePath })

	return result, nil
}

// chainOf returns the IDs from the top-level item down to itemID.
func (catalog *memoryCatalogV2) chainOf(itemID string) []string {
	var chain []string
	for id := itemID; id != ""; id = catalog.items[id].parentID {
		chain = append([]string{id}, chain...)
	}

	return chain
}

func (catalog *memoryCatalogV2) SavePlacedFile(_ context.Context, record scan.PlacedFile,
	newID func() (string, error),
) ([]string, error) {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	if err := record.Placement.Validate(); err != nil {
		return nil, err
	}

	if record.ProbedAt.IsZero() || record.File.ID == "" {
		return nil, fmt.Errorf("record without probe time or file id: %w", domain.ErrInvalid)
	}

	for _, other := range catalog.files {
		if other.state.RelativePath == record.File.RelativePath && other.id != record.File.ID {
			return nil, fmt.Errorf("path taken: %w", domain.ErrConflict)
		}
	}

	chain, err := catalog.upsertChain(record.Placement, newID)
	if err != nil {
		return nil, err
	}

	catalog.saves++
	catalog.place(record.File.ID, chain[len(chain)-1], record.Placement.Key(), library.FileState{
		RelativePath: record.File.RelativePath, Size: record.File.Size, ModifiedAt: record.File.ModifiedAt,
	})

	return chain, nil
}

// place stores the file and prunes the item it left.
func (catalog *memoryCatalogV2) place(fileID, itemID, key string, state library.FileState) {
	previous := catalog.files[fileID]
	catalog.files[fileID] = &v2File{id: fileID, itemID: itemID, placementKey: key, state: state}

	if previous != nil {
		catalog.prune(previous.itemID)
	}
}

func (catalog *memoryCatalogV2) upsertChain(placement library.Placement, newID func() (string, error)) ([]string, error) {
	var (
		chain    []string
		parentID string
	)

	for _, level := range placement.Levels {
		id := catalog.find(parentID, level)
		if id == "" {
			var err error
			if id, err = newID(); err != nil {
				return nil, err
			}

			catalog.items[id] = &v2Item{id: id, parentID: parentID, kind: level.Kind, matchKey: level.MatchKey}
		}

		chain = append(chain, id)
		parentID = id
	}

	return chain, nil
}

func (catalog *memoryCatalogV2) find(parentID string, level library.Level) string {
	for id, item := range catalog.items {
		if item.parentID == parentID && item.kind == level.Kind && item.matchKey == level.MatchKey {
			return id
		}
	}

	return ""
}

// prune removes the item and then its ancestors while they have no files and no children.
func (catalog *memoryCatalogV2) prune(itemID string) {
	for itemID != "" {
		if catalog.hasContent(itemID) {
			return
		}

		parent := catalog.items[itemID].parentID
		delete(catalog.items, itemID)
		itemID = parent
	}
}

func (catalog *memoryCatalogV2) hasContent(itemID string) bool {
	for _, file := range catalog.files {
		if file.itemID == itemID {
			return true
		}
	}

	for _, item := range catalog.items {
		if item.parentID == itemID {
			return true
		}
	}

	return false
}

func (catalog *memoryCatalogV2) MoveFile(_ context.Context, request scan.MoveRequest) ([]string, error) {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	file, found := catalog.files[request.FileID]
	if !found {
		return nil, fmt.Errorf("file %s: %w", request.FileID, domain.ErrNotFound)
	}

	if err := request.Placement.Validate(); err != nil {
		return nil, err
	}

	chain, err := catalog.upsertChain(request.Placement, request.NewID)
	if err != nil {
		return nil, err
	}

	catalog.moves++
	catalog.place(request.FileID, chain[len(chain)-1], request.Placement.Key(), file.state)

	return chain, nil
}

func (catalog *memoryCatalogV2) RemovePlacedFiles(_ context.Context, _ string, fileIDs []string) error {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	if catalog.removeErr != nil {
		return catalog.removeErr
	}

	for _, id := range fileIDs {
		if file, found := catalog.files[id]; found {
			delete(catalog.files, id)
			catalog.prune(file.itemID)
		}
	}

	return nil
}

func (catalog *memoryCatalogV2) SidecarStates(context.Context, string) ([]library.FileState, error) {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	result := make([]library.FileState, 0, len(catalog.sidecars))
	for _, state := range catalog.sidecars {
		result = append(result, state)
	}

	return result, nil
}

func (catalog *memoryCatalogV2) SaveSidecarStates(_ context.Context, _ string,
	upserts []library.FileState, removals []string,
) error {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	catalog.sidecarWrites++

	for _, state := range upserts {
		catalog.sidecars[state.RelativePath] = state
	}

	for _, removed := range removals {
		delete(catalog.sidecars, removed)
	}

	return nil
}

func (catalog *memoryCatalogV2) ItemMetadataState(context.Context, string) (scan.MetadataState, error) {
	return scan.MetadataState{}, domain.ErrNotFound
}

func (catalog *memoryCatalogV2) SaveMetadata(context.Context, string, metadata.Record) error {
	return domain.ErrNotFound
}

func (catalog *memoryCatalogV2) SaveArtwork(context.Context, string, []artwork.Candidate,
	map[artwork.Kind]artwork.Candidate,
) error {
	return domain.ErrNotFound
}

// count returns the number of items of kind.
func (catalog *memoryCatalogV2) count(kind library.ItemKind) int {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	total := 0
	for _, item := range catalog.items {
		if item.kind == kind {
			total++
		}
	}

	return total
}

// itemOf returns the leaf item ID of the file at path.
func (catalog *memoryCatalogV2) itemOf(path string) string {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	for _, file := range catalog.files {
		if file.state.RelativePath == path {
			return file.itemID
		}
	}

	return ""
}

// filesPerItem returns how many files each item has, as a sorted list.
func (catalog *memoryCatalogV2) filesPerItem() []int {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	perItem := map[string]int{}
	for _, file := range catalog.files {
		perItem[file.itemID]++
	}

	counts := make([]int, 0, len(perItem))
	for _, n := range perItem {
		counts = append(counts, n)
	}

	slices.Sort(counts)

	return counts
}

// recordingRefresher remembers the last refresh request.
type recordingRefresher struct {
	mutex   sync.Mutex
	last    scan.RefreshRequest
	callNum int
	err     error
}

func (refresher *recordingRefresher) RefreshItems(_ context.Context, request scan.RefreshRequest) error {
	refresher.mutex.Lock()
	defer refresher.mutex.Unlock()

	refresher.last = request
	refresher.callNum++

	return refresher.err
}
