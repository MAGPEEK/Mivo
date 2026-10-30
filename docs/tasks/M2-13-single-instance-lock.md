# M2-13 Single server instance per data directory
Owner: mivo-backend
Depends on: none (scheduled in wave 2 because it changes `go.mod`)
Files/areas: `internal/platform/instancelock/**` (new), `cmd/mivo/serve.go` (acquire/release
only) + `cmd/mivo/serve_test.go`, `go.mod`, `go.sum` (`golang.org/x/sys` becomes direct; the
only go.mod change in wave 2)

## Goal
A second server started on the same data directory fails fast with a clear message instead of
racing migrations and purging the first server's transcode sessions (BACKLOG "sqlite concurrent
Migrate", ADR 0024).

## Acceptance criteria
- [ ] `instancelock.Acquire(dataDir) (*Lock, error)` takes an exclusive, non-blocking OS lock on
      `<data>/server.lock` (`flock` on Linux, `LockFileEx` on Windows via `golang.org/x/sys`);
      `Release()` unlocks and closes. The lock disappears when the process dies (no stale-lock
      cleanup needed); the file itself is left in place.
- [ ] A second `Acquire` on the same directory (other process) fails with an error wrapping
      `ErrLocked` that names the directory. Test with a helper subprocess (`os.Args[0]` re-exec
      pattern), on Windows and Linux (Docker).
- [ ] `serve` acquires the lock after creating the directories and before opening the database,
      and releases it last; the error message is
      `another server is already using <data dir>`. Test in `serve_test.go` with two starts on one
      temp directory.
- [ ] Lock file mode `0o600`.
- [ ] Gates: lint + layer rules + `go test -race ./internal/platform/... ./cmd/...` + coverage
      gate; `go mod tidy -diff` clean; `govulncheck` clean.

## Notes
- `platform` may use third-party packages; `golang.org/x/sys` is already an indirect dependency.
- The BACKLOG row "sqlite DB/WAL/SHM file mode" (due M7) stays open; do not change the database
  adapter here (M2-10 runs in parallel).
- Docker: `/config/data` is a volume; the lock works on local file systems. Document in the
  REPORT that network file systems (SMB/NFS data dirs) may not honour locks.
