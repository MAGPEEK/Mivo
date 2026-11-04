# M3-19e App: library management use cases
Owner: mivo-backend
Depends on: M3-19b, M3-06 (edits `internal/app/library` in wave 2)
Files/areas: `internal/app/library/` new files `manage.go`, `manage_ports.go`, `import.go`,
`browse.go`, `manage_fakes_test.go` and their tests; `service.go` only to accept the new
dependencies through an options struct (existing constructor callers keep compiling)

## Goal
Creating, editing, removing and importing libraries and browsing folders are use cases with
fakes-tested rules: validation through the domain, folder resolution through the browse port,
a scan right after a change, a cancelled scan before a removal, and change notices for the scan
triggers. UX: AD-40, LS-01 … LS-03 (M3 subset).

## Acceptance criteria
- [ ] Ports (consumer-side, 1–3 methods each): `LibraryWriter` (create, update, delete in one
      transaction each), `LibraryCatalogReader` (list v2 libraries with folders, get one),
      `ImportLedger` (filter roots never imported, record roots), `FolderBrowser` (roots, list,
      check – the M3-19d shapes), `ScanControl` (start, cancel and wait), `LibraryEvents`
      (added, changed, removed – for M3-19j). Domain types from M3-19b cross the ports.
- [ ] `CreateLibrary(ctx, request)`: resolves every `{rootID, path}` through `FolderBrowser.Check`
      (errors mapped to typed folder errors with the index: missing → `folder_not_found`,
      forbidden → `folder_unreadable`, unknown root/invalid → `folder_outside_roots`,
      unavailable → passed on), validates with `library.ValidateLibrary` against all other
      libraries and the reserved directories, applies `DefaultOptions` for omitted options,
      stores, emits `added`, starts a scan (a failure to start is logged, not returned).
- [ ] `UpdateLibrary(ctx, id, patch)`: kind never changes; `{folderID}` keeps an existing folder
      (unknown id → `invalid`), `{rootID, path}` adds one; validation as for create, excluding the
      library itself from "other libraries". When folders or options changed: cancel a running
      scan of the library, store, emit `changed`, start a scan; a name/language-only change
      stores and emits `changed` without a scan.
- [ ] `RemoveLibrary(ctx, id)`: cancel and wait for its scan (bounded wait, named constant), then
      delete (catalog cascade), emit `removed`; unknown id → `ErrNotFound`.
- [ ] `ImportConfigured(ctx, definitions)` (replaces the startup `SyncLibraries` once M3-19l
      wires it): validates definitions as before; asks the ledger which roots were never
      imported; creates one-folder libraries for them (`ImportedFromConfig = true`); a root that
      is already a folder of another library is recorded and skipped (info log); records every
      handled root; never deletes or changes existing libraries; returns what it created and
      skipped. Invalid definitions fail before anything is written (as today).
- [ ] `BrowseRoots(ctx)` and `BrowseFolders(ctx, rootID, path)`: pass through the port and mark
      each listed folder `selectable=false` with `reason=reserved` (it holds a reserved
      directory) or `in_use` (it is, contains or lies inside a folder of an existing library),
      using the domain rules.
- [ ] Tests with in-memory fakes (no mock framework): every error mapping and index, overlap
      with another library named by id, reserved, defaults, keep/add/remove folders, kind
      unchanged, scan started after create and folder/option changes but not after a rename,
      cancel-before-delete order, events emitted once, import (new, already imported, root in
      use, invalid definition writes nothing), browse marking; package coverage ≥ the
      configured threshold.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./internal/app/library/...`, coverage gate.

## Notes
- The existing `Store.SyncLibraries` path stays untouched (dual path, as in M3-10/M3-06); M3-19l
  switches `cmd` to `ImportConfigured` and deletes the old method, port and fakes.
- `ScanControl` is satisfied by `app/scan.Service` once M3-19i adds `Cancel`; app may import app
  (ADR 0019), but declare the port here and let `cmd` pass the service.
- Concurrency: two management calls for the same library must not interleave validation and
  write (a second create with the same folder must fail with `folder_overlap`, not a raw
  unique-constraint error). Serialise management writes with one mutex in the service; the
  store's unique index (M3-19g) is the backstop and its conflict error maps to
  `folder_overlap`.
- Image cache entries of a removed library are purged by the existing purge of unknown ids after
  the next scan or at startup (ADR 0027 §8); no extra port.
- Running playback sessions of a removed library are not interrupted; new requests for its
  items return `not_found`.
