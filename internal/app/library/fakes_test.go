package library_test

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// memoryLibraryReader is an in-memory ManagedReader for the item use cases.
type memoryLibraryReader struct {
	mutex     sync.Mutex
	libraries []library.ManagedLibrary
	// getErr, when set, is returned by GetManaged.
	getErr error
}

func (store *memoryLibraryReader) ListManaged(context.Context) ([]library.ManagedLibrary, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	return slices.Clone(store.libraries), nil
}

func (store *memoryLibraryReader) GetManaged(_ context.Context, id string) (library.ManagedLibrary, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if store.getErr != nil {
		return library.ManagedLibrary{}, store.getErr
	}

	for _, stored := range store.libraries {
		if stored.ID == id {
			return stored, nil
		}
	}

	return library.ManagedLibrary{}, fmt.Errorf("library %q: %w", id, domain.ErrNotFound)
}

// put stores libraries directly.
func (store *memoryLibraryReader) put(libraries ...library.ManagedLibrary) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	store.libraries = append(store.libraries, libraries...)
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

// memoryChildReader is an in-memory ChildReader over a flat list of catalog items. It mirrors
// the behaviour the sqlite reader (adapters/sqlite, M3-05c) must show, so service tests and
// adapter tests agree on one contract: an unknown item wraps domain.ErrNotFound, a leaf and a
// container without children yield an empty page, extras are never children, and the order
// is the one compareChildren defines. Its cursor is the direction and the index of the next
// child ("asc:2"), so a cursor of the other direction is rejected like a foreign one.
type memoryChildReader struct {
	mutex sync.Mutex
	items []applibrary.ItemSummary

	// listErr, when set, is returned by ListChildren.
	listErr error
	// requests records every ListChildren call.
	requests []childRequest
}

// childRequest is one recorded ListChildren call.
type childRequest struct {
	itemID  string
	request applibrary.ChildPageRequest
}

// Cursor prefixes of the fake, one per direction.
const (
	ascendingCursorPrefix  = "asc:"
	descendingCursorPrefix = "desc:"
)

func (reader *memoryChildReader) put(summaries ...applibrary.ItemSummary) {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()

	reader.items = append(reader.items, summaries...)
}

func (reader *memoryChildReader) ListChildren(_ context.Context, itemID string,
	request applibrary.ChildPageRequest,
) (applibrary.ItemPage, error) {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()

	reader.requests = append(reader.requests, childRequest{itemID: itemID, request: request})

	if reader.listErr != nil {
		return applibrary.ItemPage{}, reader.listErr
	}

	known := slices.ContainsFunc(reader.items, func(summary applibrary.ItemSummary) bool {
		return summary.Item.ID == itemID
	})
	if !known {
		return applibrary.ItemPage{}, fmt.Errorf("item %q: %w", itemID, domain.ErrNotFound)
	}

	children := reader.orderedChildren(itemID, request.Descending)

	prefix := ascendingCursorPrefix
	if request.Descending {
		prefix = descendingCursorPrefix
	}

	start, err := parseDirectedCursor(request.Page.Cursor, prefix, len(children))
	if err != nil {
		return applibrary.ItemPage{}, err
	}

	end := min(start+request.Page.Limit, len(children))
	page := applibrary.ItemPage{Items: slices.Clone(children[start:end])}

	if end < len(children) {
		page.NextCursor = prefix + strconv.Itoa(end)
	}

	return page, nil
}

// orderedChildren returns the children of itemID without extras in the contract order.
func (reader *memoryChildReader) orderedChildren(itemID string, descending bool) []applibrary.ItemSummary {
	children := []applibrary.ItemSummary{}

	for _, summary := range reader.items {
		if summary.Item.ParentID == itemID && summary.Item.Kind != library.ItemKindExtra {
			children = append(children, summary)
		}
	}

	slices.SortFunc(children, compareChildren)

	if descending {
		slices.Reverse(children)
	}

	return children
}

// Number groups of the child order: regular numbers first, then the specials season, then
// items without a number.
const (
	numberGroupRegular = iota
	numberGroupSpecials
	numberGroupUnnumbered
)

// compareChildren is the single place of the fake's child order (ChildReader doc, ADR 0027
// §2): by number group, then number, then sort title, then ID.
func compareChildren(first, second applibrary.ItemSummary) int {
	firstGroup, firstNumber := numberGroup(first.Item)
	secondGroup, secondNumber := numberGroup(second.Item)

	return cmp.Or(
		cmp.Compare(firstGroup, secondGroup),
		cmp.Compare(firstNumber, secondNumber),
		strings.Compare(first.Item.SortTitle, second.Item.SortTitle),
		strings.Compare(first.Item.ID, second.Item.ID),
	)
}

func numberGroup(item library.Item) (group, number int) {
	value, ok := item.Number.Get()

	switch {
	case !ok:
		return numberGroupUnnumbered, 0
	case item.Kind == library.ItemKindSeason && value == 0:
		return numberGroupSpecials, 0
	default:
		return numberGroupRegular, value
	}
}

// parseDirectedCursor reads the fake's cursor; text without the prefix of the requested
// direction or with an index outside the list wraps domain.ErrInvalid.
func parseDirectedCursor(cursor, prefix string, childCount int) (int, error) {
	if cursor == "" {
		return 0, nil
	}

	index, found := strings.CutPrefix(cursor, prefix)
	if !found {
		return 0, fmt.Errorf("cursor %q: %w", cursor, domain.ErrInvalid)
	}

	return parseIndexCursor(index, childCount)
}

func (reader *memoryChildReader) recordedRequests() []childRequest {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()

	return slices.Clone(reader.requests)
}
