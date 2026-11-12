package main

import (
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	appaccess "github.com/magpeek/mivo/internal/app/access"
	appsearch "github.com/magpeek/mivo/internal/app/search"
)

// withCatalogReaders returns catalog with the services that read the finished catalog: the
// rows and the search.
func withCatalogReaders(db *sqlite.DB, catalog catalogServices) (catalogServices, error) {
	rows, err := newRowsService(db, catalog)
	if err != nil {
		return catalogServices{}, err
	}

	search, err := newSearchService(db, catalog)
	if err != nil {
		return catalogServices{}, err
	}

	catalog.rows, catalog.search = rows, search

	return catalog, nil
}

// newSearchService builds the search on the sqlite full-text index and catalog. The libraries
// are the viewer's, through the same scoped library source as the rows (M7-14).
func newSearchService(db *sqlite.DB, catalog catalogServices) (*appsearch.Service, error) {
	service, err := appsearch.NewService(appsearch.Dependencies{
		Libraries: appaccess.NewScopedLibraries(catalog.store),
		Index:     sqlite.NewSearchIndex(db),
		Summaries: sqlite.NewCatalogReader(db),
	})
	if err != nil {
		return nil, fmt.Errorf("create search service: %w", err)
	}

	return service, nil
}
