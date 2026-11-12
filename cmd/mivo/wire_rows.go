package main

import (
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	appaccess "github.com/magpeek/mivo/internal/app/access"
	approws "github.com/magpeek/mivo/internal/app/rows"
)

// newRowsService builds the Home and Overview rows on the sqlite catalog. A library is
// visible to its viewer, scoped by the request context (M7-14); the server's local clock seeds
// the genre rotation. The Continue watching and Up next rows read the watch store, and their
// cards carry the viewer state the watch service describes (M7-19).
func newRowsService(db *sqlite.DB, catalog catalogServices) (*approws.Service, error) {
	reader := sqlite.NewCatalogReader(db)

	service, err := approws.NewService(approws.Dependencies{
		Libraries: appaccess.NewScopedLibraries(catalog.store),
		Rows:      reader,
		Summaries: reader,
		Catalog:   catalog.libraries,
		Scans:     catalog.scans,
		Clock:     time.Now,
		Watch:     watchRowSource{store: sqlite.NewWatchStore(db), clock: time.Now},
		States:    catalog.watch,
	})
	if err != nil {
		return nil, fmt.Errorf("create rows service: %w", err)
	}

	return service, nil
}
