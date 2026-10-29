# M1-08 SQLite adapter: WAL, goose migrations, sqlc, identity repository
Owner: mivo-backend
Depends on: M1-03, M1-06
Files/areas: `internal/adapters/sqlite/**` (incl. `migrations/`, `queries/`, `sqlc.yaml`,
`sqlcgen/`), `go.mod`, `go.sum`

## Goal
The server can open its SQLite database with the agreed pragmas, apply embedded migrations
and persist its identity through sqlc-generated queries. This implements ADR 0004 and the
`IdentityStore` and `DatabaseChecker` ports from M1-06.

## Acceptance criteria
- [ ] `sqlite.Open(ctx, Options{Path})` returns a `*DB` with a writer pool (max 1 open
      connection, `_txlock=immediate`) and a reader pool. Both use `journal_mode=wal`,
      `busy_timeout=5000`, `foreign_keys=1` and `synchronous=NORMAL`. A test queries the
      pragmas on both pools.
- [ ] `DB.Migrate(ctx)` applies embedded goose migrations through the goose `Provider` API (no
      `goose.SetBaseFS` or other package-global goose state). Running it twice is a no-op.
      A test asserts the schema version and table existence.
- [ ] Migration `00001_server_identity.sql` creates a single-row table (`id INTEGER PRIMARY
      KEY CHECK (id = 1)`, `server_id TEXT NOT NULL`, `created_at INTEGER NOT NULL` as Unix
      ms UTC) with a goose `Down` section.
- [ ] `sqlc.yaml` (engine `sqlite`, schema = `migrations/`) generates into `sqlcgen/`.
      Regenerating with the pinned sqlc version (Docker command in Notes) leaves
      `git diff --exit-code internal/adapters/sqlite/sqlcgen` clean.
- [ ] `IdentityRepository` implements `app/system.IdentityStore`. Loading from an empty table
      returns an error matching `errors.Is(err, domain.ErrNotFound)`. Saving a second
      identity returns an error matching `domain.ErrConflict`. Save then load round-trips ID
      and `CreatedAt` (millisecond precision, UTC).
- [ ] `DB.Ping(ctx)` implements `app/system.DatabaseChecker`. `DB.Close()` closes both pools
      and returns a joined error.
- [ ] Compile-time assertions (`var _ system.IdentityStore = (*IdentityRepository)(nil)`) or
      equivalent checks exist in tests.
- [ ] Tests: integration tests against real SQLite in `t.TempDir()`. Every exported function
      is covered, including error paths (closed DB, cancelled context).
- [ ] Gates: `golangci-lint run` (incl. sqlclosecheck, rowserrcheck), `go test ./...` and the
      coverage check (`sqlcgen` excluded) pass.

## Notes
- Driver: `modernc.org/sqlite` (registered name `sqlite`). Goose dialect `sqlite3` works
  with it.
- Windows: verify the DSN works with drive-letter paths (`C:\...`) and paths with spaces.
  Prefer building the DSN with a `file:` URI and forward slashes and test it in `t.TempDir()`.
- sqlc needs cgo and cannot be `go install`ed on Windows without a C toolchain. Use the
  pinned Docker image, for example
  `docker run --rm -v "${PWD}:/src" -w /src sqlc/sqlc:<version> generate -f internal/adapters/sqlite/sqlc.yaml`.
  Record the exact version in the REPORT for M1-22.
- Route goose's logger to nothing or to an injected `*slog.Logger`. Never print to stdout.
- The database file name (`server.db`) and its directory are decided by the caller (M1-10).
  This package only takes a path.
