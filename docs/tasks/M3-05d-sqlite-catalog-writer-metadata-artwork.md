# M3-05d SQLite: catalog writer v2 – metadata, tags, people, provider IDs, artwork
Owner: mivo-backend
Depends on: M3-05, M3-10
Files/areas: `internal/adapters/sqlite/catalog_metadata.go` (new), `catalog_artwork.go`
(new), `queries/catalog_metadata.sql` (new), `queries/catalog_artwork.sql` (new), the
generated `sqlcgen/catalog_metadata.sql.go` and `sqlcgen/catalog_artwork.sql.go`, their tests

## Goal
The sqlite adapter implements the metadata and artwork half of the scan's v2 catalog port
(M3-10): it reads an item's current record, locks and artwork, and replaces them with the
merged result – without ever overwriting a locked field – so local NFO data and local artwork
show up and milestone 6's edit dialog can rely on the locks. UX: ED-02 (guard), ED-05 data,
ED-07 data, DT-06, DT-08 names, LB-40 data.

## Acceptance criteria
- [ ] Methods on `sqlite.LibraryCatalog` (type of M3-05b) implement exactly
      `scan.MetadataCatalog` (M3-10) and its error contract; compile-time assertion for the
      sub-interface, and for the whole `scan.CatalogV2` if M3-05b has already merged.
- [ ] Reading an item's metadata state returns the stored `metadata.Record`, its `LockSet`, its
      files (relative paths, attached-picture streams) and its artwork rows.
- [ ] Saving a record updates the item columns and replaces tags (`genre|studio|country|
      collection`, ordered), people (created by name, case-insensitively reused, ordered
      credits) and provider IDs in one transaction; **locked fields are left unchanged** even
      when the record differs (integrity guard of ADR 0027 §6) – one test per lockable field
      group.
- [ ] Saving artwork replaces the item's `local` and `embedded` rows, keeps `provider` and
      `upload` rows, keeps a locked kind's selection and otherwise stores the selection the
      scan chose; artwork IDs of unchanged candidates (same source location) are kept so image
      URLs stay valid; the tag is stored per row.
- [ ] People and tags that no item references any more are deleted (in the same transaction
      or by a cleanup method the port declares).
- [ ] Integration tests against a temp SQLite: round trip of a full record; lock guard; tag and
      person reuse across items; artwork ID stability across two saves; cascade when the item
      is deleted.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./internal/adapters/sqlite/...`, coverage gate.

## Notes
- Runs in parallel with M3-05b in the same package: stay in the files above, do not edit
  `library_catalog.go` (M3-05b owns the struct and constructor; if you start before it has
  merged, develop against a local copy of its struct and drop it when rebasing), and keep test
  helpers in your own helper file.
- Dates map to `metadata.Date`; a stored value the domain rejects is a server fault (plain
  error, not `ErrInvalid`, ADR 0022 Notes M2-15).
- Field locks are only read here; nothing in milestone 3 writes them except tests.
