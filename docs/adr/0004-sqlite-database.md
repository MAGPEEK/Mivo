# 0004 SQLite as the only database

- Status: Accepted
- Date: 2026-10-29
- Source: tech stack in `docs/plan.md`

## Context

NAS users should not have to run a separate database. The binary must cross-compile without
cgo for amd64, arm64, armv7 and windows/amd64.

## Decision

- **SQLite** through `modernc.org/sqlite` (pure Go, driver name `sqlite`).
- Pragmas on every connection: `journal_mode=WAL`, `busy_timeout=5000`, `foreign_keys=ON`,
  `synchronous=NORMAL`.
- Two pools: a **writer** pool limited to one open connection (`_txlock=immediate`) and a
  **reader** pool for queries. Repositories send writes to the writer.
- Migrations: **goose** (`github.com/pressly/goose/v3`) through its `Provider` API (no
  package-global state). SQL files are embedded with `embed` and applied at startup before
  the HTTP server starts. Production is forward-only; `Down` sections exist for development.
- Queries: **sqlc** with the `sqlite` engine, reading the goose migrations as schema.
  Generated code is committed (`DO NOT EDIT` header). sqlc needs cgo to build, so it is not
  installed through `go install` on Windows. It runs from a pinned Docker image or release
  binary. The command is documented in `CLAUDE.md`.
- Conventions: timestamps are `INTEGER` Unix milliseconds in UTC. IDs exposed through the
  API are text UUIDs. The database file lives in the configured data directory
  (`<data>/server.db`).
- Full-text search later uses FTS5, which modernc includes.

## Consequences

- No external database, simple backups (`VACUUM INTO` later).
- Write throughput is bounded by the single writer, which is fine for a media server.
- Contributors regenerating queries need Docker or the sqlc binary. Building and testing do
  not need sqlc.

## Notes (M2-13)

- `<data>/server.lock` guards the data directory: the server takes an exclusive, non-waiting OS
  lock (flock / LockFileEx) on it before opening `server.db` and holds it for its whole run, so a
  second server on the same data directory is refused. The file may stay in place; the OS drops
  the lock when the process dies. Keep the data directory on local storage – SMB/NFS may not
  honour the lock.
