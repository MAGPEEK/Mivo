# M3-19d FS: folder browser and folder check
Owner: mivo-backend
Depends on: M3-19b
Files/areas: `internal/adapters/fs/folderbrowse/**` (new package, incl. `_windows.go` /
`_unix.go` files and tests)

## Goal
The server can list the readable sub-directories of a browse root and check that a chosen
folder exists and is readable, without ever leaving the root, so the folder browser and the
validation of new libraries have a safe file system port. UX: LS-02.

## Acceptance criteria
- [ ] `New(roots []string, options)` with options for the reserved directories (config, data,
      cache, transcode), a per-call timeout (default 5 s) and the listing cap (1000).
      Root ids are the first 16 hex digits of SHA-256 over the cleaned root path (stable across
      restarts, opaque to clients).
- [ ] `Roots(ctx)` returns `{ID, Path, Available}` per configured root; availability uses the
      bounded, joined check pattern of ADR 0022 Notes M2-18 (a hung mount never blocks the call
      beyond the timeout and never starts a second blocked stat for the same root).
- [ ] `List(ctx, rootID, relativePath)` opens the root with `os.OpenRoot` and lists only
      directories inside it: `..`, absolute paths, `\`-escapes on Unix, NUL bytes and empty
      elements in `relativePath` are refused with `domain.ErrInvalid`; links that leave the root,
      absolute links and Windows junctions are not listed (ADR 0022 symlink rules); hidden names
      (`library.IsHiddenFolderName`) and, on Windows, entries with the hidden or system
      attribute are not listed; an entry that is, or lies inside, a reserved directory is not
      listed. Each remaining entry is listed only if it can be opened and read (open + read one
      entry); unreadable ones are counted in `UnreadableCount`, never named. Sorted by name
      (case-insensitive, then exact), capped with `Truncated`.
- [ ] `Check(ctx, rootID, relativePath) (absolutePath, error)`: the folder exists inside the
      root, is a directory and is readable; errors wrap `domain.ErrNotFound` (missing),
      `domain.ErrForbidden` (unreadable – the use case maps it to `folder_unreadable`),
      `domain.ErrInvalid` (bad path, unknown root) or `domain.ErrUnavailable` (root missing or
      timed out). The returned path is the cleaned `root + relativePath` (not link-evaluated,
      ADR 0022 identity rule).
- [ ] `ServiceUser()` returns the name of the user the process runs as (`os/user`), or "" when
      it cannot be determined; never an error.
- [ ] Tests against temp directories: traversal attempts (`..`, `a/../../x`, absolute, Windows
      drive and UNC forms, `%2e%2e` passed verbatim), a relative link inside the root (listed),
      a link escaping the root (not listed, `Check` refuses), an absolute link and a junction on
      Windows (not listed), hidden and `@`/`#` names, a reserved directory inside the root, an
      unreadable directory (Unix: mode 000 in Docker as non-root; Windows: deny ACL) counted but
      not named, the cap, a missing root (unavailable), cancellation.
- [ ] Gates: `go vet ./...`, `golangci-lint run` (depguard: adapters may import app and domain),
      `golangci-lint fmt --diff`, `go test -race ./internal/adapters/fs/folderbrowse/...` on
      Windows and in Docker for Linux (`docker run … golang:1.27.3 go test -race
      ./internal/adapters/fs/folderbrowse/...` as a non-root user for the permission case),
      coverage gate.

## Notes
- Reuse helpers of `adapters/fs/libraryfs` (path checks, availability) by moving shared code
  into an internal helper package only if the move stays small; otherwise duplicate with a
  comment naming the original – report the choice.
- Readability checks open every listed entry; run them with a small bounded concurrency
  (named constant, e.g. 4) under the call's timeout, because a NAS share may be slow.
- Whether an entry is "in use" by a library is decided by the use case (M3-19e), not here.
