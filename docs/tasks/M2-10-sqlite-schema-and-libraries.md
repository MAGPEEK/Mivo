# M2-10 SQLite: media catalog schema and library repository
Owner: mivo-backend
Depends on: M2-02, M2-03
Files/areas: `internal/adapters/sqlite/migrations/00002_media_catalog.sql`,
`internal/adapters/sqlite/queries/libraries.sql`, `internal/adapters/sqlite/library.go` (+ tests),
`internal/adapters/sqlite/sqlcgen/**` (regenerated), `internal/adapters/sqlite/{db.go,ping.go}` and
existing tests (BACKLOG nits only)

## Goal
The database holds libraries, items, media files and streams (ADR 0022), and the configured
libraries can be synced, listed and read.

## Acceptance criteria
- [ ] Migration `00002` creates `libraries`, `items`, `media_files`, `media_streams` exactly as in
      ADR 0022: `STRICT`, foreign keys with `ON DELETE CASCADE`, `UNIQUE(libraries.root_path)`,
      `UNIQUE(media_files.library_id, relative_path)`, indexes for `items(library_id,
      sort_title, id)` and `media_files(item_id)`, `CHECK` constraints for kinds and
      non-negative sizes. The `Down` section drops them in reverse order.
- [ ] `LibraryRepository.SyncLibraries(ctx, definitions, newID)` in one write transaction:
      inserts new roots with new IDs, updates name/kind of existing roots, deletes libraries whose
      root is no longer configured (cascade), returns all libraries ordered by name. Running it
      twice with the same input changes nothing (`updated_at` unchanged).
- [ ] `ListLibraries(ctx)` returns libraries with their item count; `GetLibrary(ctx, id)` returns
      `domain.ErrNotFound` for unknown IDs.
- [ ] Integration tests against a real temp SQLite: migrate up/down/up, sync add/rename/remove,
      cascade removes items/files/streams (insert rows directly in the test), unique root,
      `foreign_keys` enforced.
- [ ] BACKLOG rows "sqlite reader pool" and "sqlite nits" done: reader pool
      `SetMaxIdleConns(maxOpen)`, `GOMAXPROCS(0)` instead of `NumCPU`, Ping touches the file,
      zero `CreatedAt` rejected, UNC path test (Windows only, skipped elsewhere), `t.Run` in the
      DSN role loop.
- [ ] sqlc drift check: `docker run --rm -v "${PWD}:/src" -w /src sqlc/sqlc:1.31.1 diff -f
      internal/adapters/sqlite/sqlc.yaml` is clean.
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/sqlite/...` + coverage gate.

## Notes
- IDs come from the caller (`newID func() (string, error)` or a small generator interface), so
  the app layer decides identity like `app/system` does.
- Root paths are stored cleaned and absolute exactly as config produced them; the case-insensitive
  uniqueness on Windows is enforced by config (M2-04), not by SQL.
- The media queries (items, files, streams) are M2-15 in wave 3; this task only creates the
  tables. Keep `queries/libraries.sql` separate so M2-15 adds `queries/media.sql`.
- Timestamps: Unix ms UTC (ADR 0004).

## Orchestrator note (from M2-03)
- `internal/domain/library` defines `ItemKind` with the values `movie` and `episode`, plus
  `ItemKindForLibrary` (`movies` → `movie`, `shows` → `episode`) and `ParseItemKind`. Use exactly
  these values (e.g. the `items.kind` CHECK constraint) and derive the kind via `ItemKindForLibrary`.
- `ParseRelativePath` returns a plain `string` (the input unchanged) and rejects `.` segments and
  drive-relative prefixes such as `C:x`.

## Orchestrator note (from M2-02)
- `media.Stream` has an extra field `CodecTag` (ffprobe `codec_tag_string`) because M2-11's
  integration test compares it. ADR 0022's schema has no column for it: add a nullable
  `codec_tag TEXT` column to `streams` (recommended; the remux decision in later milestones needs
  it, e.g. hvc1 vs hev1) and note the addition under ADR 0022 "Notes (M2)".
- Store `frame_rate` losslessly as TEXT via `FrameRate.String()` / `media.ParseFrameRate`.
- `media.ContainerUnknown` is the string `"unknown"` (never empty) – CHECK constraints must allow it.
