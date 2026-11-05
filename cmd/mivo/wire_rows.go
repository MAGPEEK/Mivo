package main

import (
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	approws "github.com/magpeek/mivo/internal/app/rows"
)

// newRowsService builds the Home and Overview rows on the sqlite catalog. Every library is
// visible until profiles exist (M7); the server's local clock seeds the genre rotation.
func newRowsService(db *sqlite.DB, catalog catalogServices) (*approws.Service, error) {
	reader := sqlite.NewCatalogReader(db)

	service, err := approws.NewService(approws.Dependencies{
		Libraries: approws.NewAllLibraries(catalog.store),
		Rows:      reader,
		Summaries: reader,
		Catalog:   catalog.libraries,
		Scans:     catalog.scans,
		Clock:     time.Now,
	})
	if err != nil {
		return nil, fmt.Errorf("create rows service: %w", err)
	}

	return service, nil
}
