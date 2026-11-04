# M3-19f SQLite: migration 00004 library folders, options and import ledger
Owner: mivo-backend
Depends on: M3-11b (all M3 sqlite writers and readers merged), ADR 0028 written
Files/areas: `internal/adapters/sqlite/migrations/00004_library_management.sql` (new),
`internal/adapters/sqlite/sqlcgen/**` (regenerated), a new schema test file, minimal
compatibility edits in the existing library, catalog writer and reader queries and their Go
files (single-folder behaviour only)

## Goal
The database can hold several folders per library, per-library options and metadata language,
and the ledger of configuration roots already imported, while the server keeps behaving exactly
as before with one folder per library. UX: data for AD-40, LS-01 … LS-03.

## Acceptance criteria
- [ ] Goose migration `00004_library_management.sql`:
      `library_roots(id TEXT PK, library_id → libraries ON DELETE CASCADE, path TEXT NOT NULL,
      path_key TEXT NOT NULL UNIQUE, position INTEGER NOT NULL, created_at)` with
      `UNIQUE(library_id, position)`;
      `libraries` gains `metadata_language TEXT NOT NULL DEFAULT ''`, `scan_extras`,
      `read_nfo`, `local_artwork` (`INTEGER NOT NULL DEFAULT 1 CHECK IN (0,1)`) and
      `imported_from_config INTEGER NOT NULL DEFAULT 0 CHECK IN (0,1)`;
      `library_imports(path_key TEXT PRIMARY KEY, path TEXT NOT NULL, imported_at TEXT NOT NULL)`.
- [ ] Existing libraries are carried over: one `library_roots` row each (`id` = the library id,
      position 0, `path` = the old root), one `library_imports` row each, and
      `imported_from_config = 1` (they came from the configuration).
- [ ] The catalog tables that store library-relative paths (`media_files`, `library_sidecars`,
      the local-file columns of `artwork`) get `root_id → library_roots ON DELETE CASCADE`, and
      their uniqueness moves from `(library_id, relative_path)` to `(root_id, relative_path)`.
      Because the catalog is rebuildable (ADR 0027 §5), the migration may drop and recreate
      these tables and their children instead of rebuilding them in place; library rows and
      their folders are never dropped. Users upgrading from 0.1.0-alpha.2 run `00003` and
      `00004` in one start, so they see one rebuild.
- [ ] `libraries.root_path` is removed, or kept only if removing it would need a parent-table
      rebuild with foreign keys switched off; then it is documented as legacy, no longer
      unique, and no query reads it (report the choice).
- [ ] `path_key` is computed by the adapter with `library.FolderKey` (case folded on Windows);
      the migration fills it for carried-over rows with the same rule (SQL `lower()` is not
      Unicode-complete – carried-over rows on Windows are re-keyed by a one-time Go fix-up at
      open, or the limitation is documented; report which).
- [ ] Compatibility: existing writers write `root_id` = the library's only folder, readers
      resolve a file's absolute path from its folder; every existing sqlite, app and cmd test
      stays green (`go test ./...`).
- [ ] Schema tests: on an empty database and on a `00003` database with rows (libraries kept
      with their folder and ledger entry, catalog rows handled as specified), `goose down` to
      `00003` works, each `CHECK`/unique rule rejects a bad row (table test), deleting a folder
      cascades to its files, sidecars and local artwork.
- [ ] `docker run … sqlc/sqlc:1.31.1 generate …` output committed; `sqlc diff` clean.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./internal/adapters/sqlite/... ./cmd/...`, coverage gate.

## Notes
- The second-wave migrations (M3-13 … M3-15) number from `00005`.
- Do not add the management queries (M3-19g) or real multi-folder writes (M3-19h) here.
- Column names are decided here for M3-19g/h; report every deviation from this file.
