# M2-15 SQLite: media catalog repository (items, files, streams)
Owner: mivo-backend
Depends on: M2-10
Files/areas: `internal/adapters/sqlite/queries/media.sql`, `internal/adapters/sqlite/catalog.go`
(+ tests), `internal/adapters/sqlite/sqlcgen/**` (regenerated)

## Goal
Scan results are persisted atomically and the catalog can be read page by page for the library
grid, per item for the detail page and per file for playback (ADR 0022).

## Acceptance criteria
- [ ] `FileStates(ctx, libraryID) ([]library.FileState + file ID, error)` for change detection.
- [ ] `SaveProbedFile(ctx, record)` in one write transaction: creates the item (new file) or
      keeps it (changed file), upserts the `media_files` row and replaces its `media_streams`.
      A probe failure stores the file with `probe_error` and no streams. IDs come from the caller.
- [ ] `RemoveFiles(ctx, libraryID, fileIDs)` deletes files and items left without files, in one
      transaction.
- [ ] `ListItems(ctx, libraryID, page)` returns items ordered by `(sort_title, id)` with an opaque
      cursor (base64url of the last key; a tampered cursor → `domain.ErrInvalid`) plus, per
      item, its primary file with streams, loaded with one extra query for the page (no N+1).
- [ ] `GetItem(ctx, itemID)` returns the item with all files and streams; unknown → `ErrNotFound`.
- [ ] `PlayableFile(ctx, fileID)` returns the `media.File` with streams plus the library root
      (for path resolution); unknown → `ErrNotFound`.
- [ ] Integration tests on real temp SQLite: save/re-save/remove round trips, stream replacement,
      cascade, pagination across 3 pages incl. equal titles, cursor tampering, 1 000 items list in
      under 200 ms on the dev machine (log the time).
- [ ] sqlc drift check clean (command in `docs/development.md`).
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/sqlite/...` + coverage gate.

## Notes
- Method shapes must satisfy the ports of `app/library` (M2-18), `app/scan` (M2-19) and
  `app/playback` (M2-20): keep one method per use; split into two repository types if a type
  grows beyond one responsibility (catalog writer vs reader).
- All writes go through the writer pool, reads through the reader pool (ADR 0004).
- Store `relative_path` exactly as `library.ParseRelativePath` returned it.
