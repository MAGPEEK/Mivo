# M3-19g SQLite: library store v2 (CRUD, folders, import ledger)
Owner: mivo-backend
Depends on: M3-19e (port shapes), M3-19f (schema)
Files/areas: `internal/adapters/sqlite/library_store_v2.go` (new, or a name next to the existing
library repository), `internal/adapters/sqlite/queries/library_manage.sql` (new) and its
`sqlcgen/library_manage.sql.go`, tests

## Goal
The ports of the library management use cases are backed by SQLite: libraries with their
folders, options and language are created, read, updated and deleted transactionally, and the
import ledger remembers which configuration roots were already imported.

## Acceptance criteria
- [ ] Implements `LibraryWriter`, `LibraryCatalogReader` and `ImportLedger` of M3-19e, and the
      read the scan needs (M3-19i: one library with folders, kind and options).
- [ ] Create: one transaction inserts the library and its folders (ids from the injected
      generator, `position` in request order, `path_key` from `library.FolderKey`).
- [ ] Update: one transaction; kept folders keep id and catalog; removed folders are deleted
      (cascade removes their files, sidecars and local artwork); added folders inserted;
      positions renumbered; `updated_at` set. Items left without files are removed by the next
      scan (ADR 0027 §3), not here.
- [ ] Delete: one transaction deletes the library (catalog cascade); unknown id →
      `domain.ErrNotFound`.
- [ ] A `path_key` unique violation maps to `domain.ErrConflict` naming the folder index; the
      use case turns it into `folder_overlap`.
- [ ] List and get return libraries ordered by name (then id) with folders by position, options,
      language, `ImportedFromConfig` and the item count of top-level items (ADR 0027 §2).
- [ ] Ledger: `NotImported(ctx, keys)` returns the keys without a row; `Record(ctx, entries)`
      inserts and ignores existing ones.
- [ ] Tests against a real SQLite file in a temp dir: CRUD round trips, keep/add/remove folder
      with catalog cascade (rows of the removed folder gone, others kept), unique conflict with
      another library, unknown id, ordering, ledger idempotence, rollback on a failing step
      (no partial library).
- [ ] `sqlc` output committed; `sqlc diff` clean.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./internal/adapters/sqlite/...`, coverage gate.

## Notes
- The milestone-2 `SyncLibraries` implementation stays until M3-19l removes it with its port.
- Stored rows the domain rejects are reported as plain errors, never `ErrInvalid` (ADR 0022
  Notes M2-15/M2-18 contract).
