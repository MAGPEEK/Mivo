package sqlite

import (
	"database/sql"
	"time"

	"github.com/magpeek/mivo/internal/app/scan"
)

// The placement half of the v2 scan catalog (M3-05b); the metadata half (M3-05d) adds its
// methods to the same type.
var (
	_ scan.PlacedFileCatalog = (*LibraryCatalog)(nil)
	_ scan.SidecarCatalog    = (*LibraryCatalog)(nil)
	_ scan.CatalogV2         = (*LibraryCatalog)(nil)
)

// LibraryCatalog stores what the v2 scan finds: the item hierarchy with its files, sidecar
// states and, through the methods of the metadata writer, merged metadata and artwork
// (ADR 0027). Writes run in one transaction each on the writer pool (ADR 0004); reads go
// through the reader pool. IDs of new items come from the caller of each method.
type LibraryCatalog struct {
	writer *sql.DB
	reader *sql.DB
	now    func() time.Time
}

// NewLibraryCatalog returns a catalog on db, which must be migrated before the first call.
// now supplies the creation time of the items the catalog creates.
func NewLibraryCatalog(db *DB, now func() time.Time) *LibraryCatalog {
	return &LibraryCatalog{writer: db.writer, reader: db.reader, now: now}
}
