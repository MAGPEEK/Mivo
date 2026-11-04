# M3-05b SQLite: catalog writer v2 – placements, files, sidecar states
Owner: mivo-backend
Depends on: M3-05, M3-10
Files/areas: `internal/adapters/sqlite/library_catalog.go` (new type, shared by M3-05d:
constructor and struct only), `catalog_placement.go` (new), `catalog_sidecars.go` (new),
`queries/catalog_placement.sql` (new), `sqlcgen/catalog_placement.sql.go` (generated), their
tests (`catalog_placement_test.go`, `catalog_placement_helpers_test.go`); the M2
`catalog_writer.go` stays untouched

## Goal
The sqlite adapter implements the placement half of the scan's v2 catalog port declared in
M3-10: files are stored under a show → season → episode or movie (→ extra) chain whose items
keep their IDs across rescans, files move when their placement changes, empty containers
disappear, and sidecar states are stored for change detection. UX data for DT-20 … DT-22,
DT-24, DT-09.

## Acceptance criteria
- [ ] A new type `sqlite.LibraryCatalog` implements exactly `scan.PlacementCatalog` (M3-10)
      with its documented error contract; compile-time assertion `var _ scan.PlacementCatalog
      = (*LibraryCatalog)(nil)`. The assertion for the whole `scan.CatalogV2` is added by
      whichever of M3-05b/M3-05d merges second.
- [ ] Saving a probed file upserts the chain level by level by `(library, parent, kind,
      match_key)` in one transaction, creates missing items with IDs from the injected
      generator, sets title/sort title/year/numbers only when it creates an item (later title
      changes belong to the metadata writer), stores the file with version label and part
      number, and returns the chain's item IDs.
- [ ] `PlacedFiles` returns each file's stored placement key (`library.Placement.Key()` built
      from the stored chain) so the scan can detect moves; moving a file re-parents it in one
      transaction without touching its streams.
- [ ] Removing files and moving files delete items left without files bottom-up (episode →
      season → show; movie with its extras); extras cascade with their parent.
- [ ] Sidecar states: list per library, upsert and delete by relative path; idempotent like
      `RemovePlacedFiles` (unknown paths and other libraries' paths are ignored).
- [ ] Integration tests against a temp SQLite: rescan keeps every ID; a second version joins
      the existing movie; a renamed episode file keeps the episode; a file whose placement
      changes moves and the old show disappears; 131 absolute-numbered episodes produce one
      show, one season, 131 episodes (DT-24); an unknown library and a conflicting relative
      path return the contract errors; concurrency with a reader (WAL) under `-race`.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./internal/adapters/sqlite/...`, coverage gate.

## Notes
- Runs in parallel with M3-05d (metadata and artwork writes, same package, same type): this
  task owns `library_catalog.go` (struct, constructor taking the DB, ID generator and clock);
  M3-05d adds methods in its own files and does not edit it. If M3-05d needs another
  constructor argument, the orchestrator merges M3-05b first. Keep test helpers in your own
  helper file; do not edit `catalog_helpers_test.go`.
- The M2 `CatalogWriter` and its queries are deleted in M3-11b once the scan uses only v2.
- The single writer pool (ADR 0004) serialises transactions; keep one transaction per file so
  a long scan does not starve the API's writes.
- The M2 cursor nit (`\r`/`\n` in cursors) belongs to the reader task M3-05c.
