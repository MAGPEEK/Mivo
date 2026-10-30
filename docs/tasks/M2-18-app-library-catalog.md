# M2-18 Use cases: library catalog (sync, list, items, detail)
Owner: mivo-backend
Depends on: M2-02, M2-03
Files/areas: `internal/app/library/**` (new package)

## Goal
The configured libraries are synced into the catalog at startup, and clients can list libraries,
page through a library's items and open one item with its files and streams (ADR 0022).

## Acceptance criteria
- [ ] Ports declared here (1–3 methods each): a library store (`SyncLibraries`, `ListLibraries`,
      `GetLibrary`), an item reader (`ListItems`, `GetItem`), an ID generator.
- [ ] `Service.SyncLibraries(ctx, []library.Definition)` validates every definition
      (`ErrInvalid` names the index) and delegates; returns the synced libraries.
- [ ] `ListLibraries`, `GetLibrary`, `ListItems(ctx, libraryID, PageRequest)` (limit normalised by
      `library.NormalizeLimit`, unknown library → `ErrNotFound`), `GetItem(ctx, itemID)` returning an
      app-level `ItemDetail{Item, Files []media.File}`.
- [ ] Library availability: `ListLibraries` marks a library unavailable when a root-check port
      (`RootAvailable(ctx, root) bool`, implemented by `libraryfs` in M2-32 wiring) says so; checks
      run with a short timeout so a hung NAS mount cannot block the list.
- [ ] `NewService` rejects nil dependencies with `ErrInvalid`.
- [ ] Tests with in-memory fakes (no mock framework): sync validation, not-found paths, limit
      normalisation, availability timeout; coverage ≥ 90 %.
- [ ] Gates: lint + layer rules (app imports domain only) + `go test -race ./internal/app/...` +
      coverage gate.

## Notes
- Scanning is a separate package (`internal/app/scan`, M2-19) so both tasks run in parallel.
- Scan status is not stored here; the HTTP adapter (M2-26) combines `ListLibraries` with the
  scan service's status.
- Return domain types and the small `ItemDetail` struct; adapters may import it.
