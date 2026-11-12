// Package rows contains the use cases of the server-generated rows (tasks M5-12, M5-13): the
// rows and hero of Home and the rows of a library's Overview pivot. The rules live in
// internal/domain/rows; this package reads the catalog through the ports it declares, and
// cmd/mivo wires them (ADR 0003).
package rows

import (
	"context"
	"time"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	appscan "github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/rows"
)

// LibrarySource lists the libraries the viewer of the context may see. Implemented by
// app/access.ScopedLibraries: the viewer's effective access set (ADR 0032 section 7).
type LibrarySource interface {
	// VisibleLibraries returns the visible libraries in display order (by name).
	VisibleLibraries(ctx context.Context) ([]library.ManagedLibrary, error)
	// VisibleLibrary returns one visible library; an unknown or hidden one wraps
	// domain.ErrNotFound.
	VisibleLibrary(ctx context.Context, id string) (library.ManagedLibrary, error)
}

// ItemRequest asks for the first items of a row of top-level items. Query.Filter.Kind is the
// listed kind (empty for a library without one), Query.Filter.Genres narrows a genre row and
// Query.Filter.OfflineRootIDs are the folders that cannot be reached now. Query.Restriction is
// what the viewer may see; its zero value allows nothing.
type ItemRequest struct {
	LibraryID string
	Query     library.ListingQuery
	Limit     int
}

// EpisodeRequest asks for the episodes of a shows library. Limit counts shows for
// ListEpisodeGroups and episodes for ListRecentEpisodes; Window is the RecentEpisodeWindow of
// ListEpisodeGroups. Restriction is what the viewer may see; its zero value allows nothing.
type EpisodeRequest struct {
	LibraryID      string
	OfflineRootIDs []string
	Limit          int
	Window         time.Duration
	Restriction    access.Restriction
}

// EpisodeRef names an episode with its season and show.
type EpisodeRef struct {
	EpisodeID string
	SeasonID  string
	ShowID    string
}

// RowReader reads the items of rows. Implemented by adapters/sqlite (M5-13). Every method
// leaves out unavailable items: those whose primary file could not be probed or lies in an
// offline folder (a show counts as available while one of its episodes is).
type RowReader interface {
	// ListRowItems returns up to request.Limit top-level items in the order of
	// request.Query.Order under its filter, leaving out the items without a value for the
	// sort key (an item without a release date in "newReleases").
	ListRowItems(ctx context.Context, request ItemRequest) ([]applibrary.ItemSummary, error)
	// ListEpisodeGroups returns, for the request.Limit shows with the most recently added
	// episodes, per season the episodes added within request.Window of the show's newest.
	ListEpisodeGroups(ctx context.Context, request EpisodeRequest) ([]rows.EpisodeGroup, error)
	// ListRecentEpisodes returns up to request.Limit episodes with an air date, newest
	// first (ties by date added, newest first).
	ListRecentEpisodes(ctx context.Context, request EpisodeRequest) ([]EpisodeRef, error)
}

// SummaryReader reads the summaries of items as listings carry them. Implemented by
// adapters/sqlite (CatalogReader.ItemSummaries).
type SummaryReader interface {
	// ItemSummaries returns the summaries of the given items; unknown IDs and items the
	// restriction hides are left out. The zero restriction allows none.
	ItemSummaries(ctx context.Context, itemIDs []string, restriction access.Restriction) ([]applibrary.ItemSummary, error)
}

// Catalog is the part of the library use cases the rows need. Implemented by
// app/library.Service.
type Catalog interface {
	// ListFacets returns the facet counts of a library listing.
	ListFacets(ctx context.Context, libraryID string, request applibrary.FacetRequest) ([]applibrary.Facet, error)
	// GetItem returns an item with its detail; an unknown ID wraps domain.ErrNotFound.
	GetItem(ctx context.Context, itemID string) (applibrary.ItemDetail, error)
	// OfflineFolders returns the IDs of the folders that cannot be reached now.
	OfflineFolders(ctx context.Context, folders []library.Folder) []string
}

// ScanStatus reports the scan of a library. Implemented by app/scan.Service.
type ScanStatus interface {
	// Status returns the status of the current or last scan of the library.
	Status(libraryID string) appscan.Status
}
