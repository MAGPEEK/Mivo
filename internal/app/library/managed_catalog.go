package library

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/domain/library"
)

// DefaultRootCheckTimeout is how long listing libraries waits for a folder check. It is short
// because the library list is the first request of every client and a hung network mount
// must not block it; a folder that does not answer in time is reported as unavailable.
const DefaultRootCheckTimeout = time.Second

// ItemCountReader counts the catalog items of the libraries. Implemented by adapters/sqlite
// (ManagedLibraryStore.TopLevelItemCounts).
type ItemCountReader interface {
	// TopLevelItemCounts returns, per library ID, the number of top-level items (movies and
	// shows, the items listing a library returns). A library without items has no entry.
	TopLevelItemCounts(ctx context.Context) (map[string]int64, error)
}

// FolderOverview is a library folder with whether it is reachable now.
type FolderOverview struct {
	Folder    library.Folder
	Available bool
}

// ManagedOverview is a managed library as clients see it. Scan status is added by the HTTP
// adapter from the scan service.
type ManagedOverview struct {
	Library library.ManagedLibrary
	// Folders are the folders of Library in their stored order.
	Folders []FolderOverview
	// Available is true when every folder is reachable: a library with an offline share
	// plays only part of its items, which clients show as a warning; Folders say which.
	Available bool
	ItemCount int64
}

// ManagedCatalogDependencies lists everything ManagedCatalog needs; all ports are required.
type ManagedCatalogDependencies struct {
	Libraries ManagedReader
	Counts    ItemCountReader
	Roots     RootChecker
	// RootCheckTimeout bounds every folder check; zero means DefaultRootCheckTimeout.
	RootCheckTimeout time.Duration
}

// ManagedCatalog reads the managed libraries with their availability and item counts. It is
// safe for concurrent use.
type ManagedCatalog struct {
	libraries ManagedReader
	counts    ItemCountReader
	roots     *rootChecks
}

// NewManagedCatalog validates the dependencies; a missing one wraps domain.ErrInvalid.
func NewManagedCatalog(deps ManagedCatalogDependencies) (*ManagedCatalog, error) {
	var problems []error

	if deps.Libraries == nil {
		problems = append(problems, missingDependency("library reader"))
	}

	if deps.Counts == nil {
		problems = append(problems, missingDependency("item count reader"))
	}

	if deps.Roots == nil {
		problems = append(problems, missingDependency("root checker"))
	}

	if err := errors.Join(problems...); err != nil {
		return nil, fmt.Errorf("new managed catalog: %w", err)
	}

	timeout := deps.RootCheckTimeout
	if timeout == 0 {
		timeout = DefaultRootCheckTimeout
	}

	return &ManagedCatalog{
		libraries: deps.Libraries, counts: deps.Counts, roots: newRootChecks(deps.Roots, timeout),
	}, nil
}

// ListLibraries returns every library ordered by name. The list is never nil.
func (catalog *ManagedCatalog) ListLibraries(ctx context.Context) ([]ManagedOverview, error) {
	libraries, err := catalog.libraries.ListManaged(ctx)
	if err != nil {
		return nil, fmt.Errorf("list libraries: %w", err)
	}

	return catalog.describeAll(ctx, libraries)
}

// GetLibrary returns one library; an unknown ID wraps domain.ErrNotFound.
func (catalog *ManagedCatalog) GetLibrary(ctx context.Context, id string) (ManagedOverview, error) {
	managed, err := catalog.libraries.GetManaged(ctx, id)
	if err != nil {
		return ManagedOverview{}, fmt.Errorf("get library %q: %w", id, err)
	}

	return catalog.Describe(ctx, managed)
}

// Describe adds availability and the item count to a library the caller holds already, for
// example the one a create or update just returned.
func (catalog *ManagedCatalog) Describe(ctx context.Context, managed library.ManagedLibrary) (ManagedOverview, error) {
	overviews, err := catalog.describeAll(ctx, []library.ManagedLibrary{managed})
	if err != nil {
		return ManagedOverview{}, err
	}

	return overviews[0], nil
}

// describeAll checks the folders of all libraries at once, so several hung mounts cost one
// timeout rather than one each.
func (catalog *ManagedCatalog) describeAll(ctx context.Context,
	libraries []library.ManagedLibrary,
) ([]ManagedOverview, error) {
	var paths []string

	for _, managed := range libraries {
		for _, folder := range managed.Folders {
			paths = append(paths, folder.Path)
		}
	}

	available := catalog.roots.availableAll(ctx, paths)

	// A caller that gave up would otherwise see every folder reported unavailable.
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("describe libraries: %w", err)
	}

	counts, err := catalog.counts.TopLevelItemCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("count library items: %w", err)
	}

	overviews := make([]ManagedOverview, 0, len(libraries))

	for _, managed := range libraries {
		overview := ManagedOverview{
			Library: managed, Available: true, ItemCount: counts[managed.ID],
			Folders: make([]FolderOverview, 0, len(managed.Folders)),
		}

		for _, folder := range managed.Folders {
			reachable := available[0]
			available = available[1:]

			overview.Folders = append(overview.Folders, FolderOverview{Folder: folder, Available: reachable})
			overview.Available = overview.Available && reachable
		}

		overviews = append(overviews, overview)
	}

	return overviews, nil
}
