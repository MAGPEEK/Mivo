# M2-05 Transcode workspace: session directories and safe cleanup
Owner: mivo-backend
Depends on: none
Files/areas: `internal/adapters/fs/transcodedir/**` (new package)

## Goal
Each playback session gets its own directory below `<transcode>/sessions/`, which is removed when
the session ends, and a server start removes leftovers of a crash – without ever deleting
anything that is not a session directory (ADR 0024).

## Acceptance criteria
- [ ] `transcodedir.New(root string)` validates an absolute root and creates `<root>/sessions`
      (mode `0o750`) lazily on first use.
- [ ] `Create(ctx, sessionID) (dir string, error)` accepts only canonical lowercase UUIDs
      (else `domain.ErrInvalid`), creates the directory exclusively (existing → `ErrConflict`).
- [ ] `Remove(ctx, sessionID)` deletes the directory recursively; on Windows it retries while
      files are still locked by a just-killed FFmpeg (bounded retries with named constants,
      cancellable by ctx). Removing a missing session is not an error.
- [ ] `Purge(ctx)` deletes only entries of `<root>/sessions` whose name is a UUID; other files and
      directories, the `sessions` directory itself and everything outside it stay untouched
      (test with decoy files next to and inside `sessions`). Returns the number removed.
- [ ] `OpenFile(sessionID, name)` opens a file inside the session directory through `os.Root`;
      names are not validated here (the HTTP adapter does that) but `..`, absolute names and
      symlinks leaving the directory fail (tests incl. a symlink where the OS allows it).
- [ ] Tests in `t.TempDir()`; coverage ≥ 85 %.
- [ ] Gates: lint + layer rules + `go test -race` + coverage gate pass.

## Notes
- The package name must not be `fs` (shadows `io/fs`).
- Use `os.Root` (Go 1.24+) for every open below the session directory.
- The method set should satisfy the small ports M2-20/M2-25 declare (`Create`, `Remove`,
  `Purge`) and the stream handler's opener (M2-29); keep signatures as listed.
- No brand string in directory names (ADR 0002).
