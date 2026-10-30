package library_test

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// memoryLibraryStore is an in-memory Store. Like the real adapter it identifies a
// library by its root, creates IDs only for new roots, deletes unconfigured roots and reports
// them with their item counts, and lists libraries ordered by name.
type memoryLibraryStore struct {
	mutex     sync.Mutex
	libraries []library.Summary

	// syncErr, listErr and getErr, when set, are returned by the matching method.
	syncErr error
	listErr error
	getErr  error
	// syncCalls counts SyncLibraries calls, so tests can prove invalid input never arrives.
	syncCalls int
}

func (store *memoryLibraryStore) SyncLibraries(_ context.Context, definitions []library.Definition,
	newID func() (string, error),
) (library.SyncResult, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	store.syncCalls++

	if store.syncErr != nil {
		return library.SyncResult{}, store.syncErr
	}

	synced := make([]library.Summary, 0, len(definitions))

	for _, definition := range definitions {
		summary, err := store.syncOne(definition, newID)
		if err != nil {
			return library.SyncResult{}, err
		}

		synced = append(synced, summary)
	}

	removed := store.unconfigured(definitions)
	store.libraries = sortedByName(synced)

	return library.SyncResult{Libraries: librariesOf(store.libraries), Removed: removed}, nil
}

func (store *memoryLibraryStore) syncOne(definition library.Definition,
	newID func() (string, error),
) (library.Summary, error) {
	for _, stored := range store.libraries {
		if stored.Library.Root == definition.Root {
			stored.Library.Name, stored.Library.Kind = definition.Name, definition.Kind

			return stored, nil
		}
	}

	id, err := newID()
	if err != nil {
		return library.Summary{}, fmt.Errorf("new id for library %q: %w", definition.Name, err)
	}

	return library.Summary{Library: library.Library{
		ID: id, Name: definition.Name, Root: definition.Root, Kind: definition.Kind,
	}}, nil
}

func (store *memoryLibraryStore) unconfigured(definitions []library.Definition) []library.Summary {
	removed := []library.Summary{}

	for _, stored := range store.libraries {
		configured := slices.ContainsFunc(definitions, func(definition library.Definition) bool {
			return definition.Root == stored.Library.Root
		})
		if !configured {
			removed = append(removed, stored)
		}
	}

	return removed
}

func (store *memoryLibraryStore) ListLibraries(context.Context) ([]library.Summary, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if store.listErr != nil {
		return nil, store.listErr
	}

	return slices.Clone(store.libraries), nil
}

func (store *memoryLibraryStore) GetLibrary(_ context.Context, id string) (library.Summary, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if store.getErr != nil {
		return library.Summary{}, store.getErr
	}

	for _, stored := range store.libraries {
		if stored.Library.ID == id {
			return stored, nil
		}
	}

	return library.Summary{}, fmt.Errorf("library %q: %w", id, domain.ErrNotFound)
}

// put stores summaries directly, as an earlier sync would have.
func (store *memoryLibraryStore) put(summaries ...library.Summary) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	store.libraries = sortedByName(append(store.libraries, summaries...))
}

func (store *memoryLibraryStore) syncs() int {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	return store.syncCalls
}

func sortedByName(summaries []library.Summary) []library.Summary {
	slices.SortFunc(summaries, func(first, second library.Summary) int {
		return strings.Compare(first.Library.Name, second.Library.Name)
	})

	return summaries
}

func librariesOf(summaries []library.Summary) []library.Library {
	libraries := make([]library.Library, 0, len(summaries))
	for _, summary := range summaries {
		libraries = append(libraries, summary.Library)
	}

	return libraries
}

// memoryItemReader is an in-memory ItemReader holding the items of each library in sort
// order. It pages by a cursor that is the index of the next item as text, which is enough
// to exercise the service; the real cursor format belongs to the adapter.
type memoryItemReader struct {
	mutex   sync.Mutex
	items   map[string][]applibrary.ItemSummary
	details map[string]applibrary.ItemDetail

	// listErr and getErr, when set, are returned by the matching method.
	listErr error
	getErr  error
	// requests records every ListItems call.
	requests []listRequest
}

// listRequest is one recorded ListItems call.
type listRequest struct {
	libraryID string
	page      library.PageRequest
}

func newMemoryItemReader() *memoryItemReader {
	return &memoryItemReader{
		items:   map[string][]applibrary.ItemSummary{},
		details: map[string]applibrary.ItemDetail{},
	}
}

func (reader *memoryItemReader) ListItems(_ context.Context, libraryID string,
	page library.PageRequest,
) (applibrary.ItemPage, error) {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()

	reader.requests = append(reader.requests, listRequest{libraryID: libraryID, page: page})

	if reader.listErr != nil {
		return applibrary.ItemPage{}, reader.listErr
	}

	all := reader.items[libraryID]

	start, err := parseIndexCursor(page.Cursor, len(all))
	if err != nil {
		return applibrary.ItemPage{}, err
	}

	end := min(start+page.Limit, len(all))
	result := applibrary.ItemPage{Items: slices.Clone(all[start:end])}

	if end < len(all) {
		result.NextCursor = strconv.Itoa(end)
	}

	return result, nil
}

// parseIndexCursor reads the fake's cursor; like a tampered cursor of the real adapter, text
// that is not an index into the list wraps domain.ErrInvalid.
func parseIndexCursor(cursor string, itemCount int) (int, error) {
	if cursor == "" {
		return 0, nil
	}

	start, err := strconv.Atoi(cursor)
	if err != nil || start < 0 || start > itemCount {
		return 0, fmt.Errorf("cursor %q: %w", cursor, domain.ErrInvalid)
	}

	return start, nil
}

func (reader *memoryItemReader) GetItem(_ context.Context, itemID string) (applibrary.ItemDetail, error) {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()

	if reader.getErr != nil {
		return applibrary.ItemDetail{}, reader.getErr
	}

	detail, found := reader.details[itemID]
	if !found {
		return applibrary.ItemDetail{}, fmt.Errorf("item %q: %w", itemID, domain.ErrNotFound)
	}

	return detail, nil
}

func (reader *memoryItemReader) recordedRequests() []listRequest {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()

	return slices.Clone(reader.requests)
}

// sequenceIDGenerator returns the configured IDs in order and then fails.
type sequenceIDGenerator struct {
	mutex sync.Mutex
	ids   []string
	err   error
}

func (generator *sequenceIDGenerator) NewID() (string, error) {
	generator.mutex.Lock()
	defer generator.mutex.Unlock()

	if generator.err != nil {
		return "", generator.err
	}

	if len(generator.ids) == 0 {
		return "", errIDsExhausted
	}

	next := generator.ids[0]
	generator.ids = generator.ids[1:]

	return next, nil
}

// rootCheckerFunc adapts a function to the RootChecker port.
type rootCheckerFunc func(ctx context.Context, root string) bool

func (check rootCheckerFunc) RootAvailable(ctx context.Context, root string) bool {
	return check(ctx, root)
}

// availableRoots reports exactly the given roots as available.
func availableRoots(roots ...string) rootCheckerFunc {
	return func(_ context.Context, root string) bool {
		return slices.Contains(roots, root)
	}
}

// hungRootChecker simulates a stat on a network mount that hangs and ignores cancellation:
// a check of hungRoot blocks until release is closed, then reports availableAfterRelease.
// Every other root is available. It counts the checks per root.
type hungRootChecker struct {
	hungRoot              string
	release               chan struct{}
	availableAfterRelease bool

	releaseOnce sync.Once
	mutex       sync.Mutex
	checks      map[string]int
}

// releaseChecks lets every hung check return; safe to call more than once (test and cleanup).
func (checker *hungRootChecker) releaseChecks() {
	checker.releaseOnce.Do(func() { close(checker.release) })
}

func newHungRootChecker(hungRoot string) *hungRootChecker {
	return &hungRootChecker{hungRoot: hungRoot, release: make(chan struct{}), checks: map[string]int{}}
}

func (checker *hungRootChecker) RootAvailable(_ context.Context, root string) bool {
	checker.mutex.Lock()
	checker.checks[root]++
	checker.mutex.Unlock()

	if root != checker.hungRoot {
		return true
	}

	<-checker.release

	return checker.availableAfterRelease
}

func (checker *hungRootChecker) checkCount(root string) int {
	checker.mutex.Lock()
	defer checker.mutex.Unlock()

	return checker.checks[root]
}

// recordingHandler is a slog.Handler that keeps every record for assertions.
type recordingHandler struct {
	mutex   *sync.Mutex
	records *[]slog.Record
}

func newRecordingLogger() (*slog.Logger, *recordingHandler) {
	handler := &recordingHandler{mutex: &sync.Mutex{}, records: &[]slog.Record{}}

	return slog.New(handler), handler
}

func (handler *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (handler *recordingHandler) Handle(_ context.Context, record slog.Record) error {
	handler.mutex.Lock()
	defer handler.mutex.Unlock()

	*handler.records = append(*handler.records, record.Clone())

	return nil
}

func (handler *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return handler }

func (handler *recordingHandler) WithGroup(string) slog.Handler { return handler }

func (handler *recordingHandler) recorded() []slog.Record {
	handler.mutex.Lock()
	defer handler.mutex.Unlock()

	return slices.Clone(*handler.records)
}

// attributes returns the attributes of record as text keyed by name.
func attributes(record slog.Record) map[string]string {
	values := map[string]string{}

	record.Attrs(func(attribute slog.Attr) bool {
		values[attribute.Key] = attribute.Value.String()

		return true
	})

	return values
}
