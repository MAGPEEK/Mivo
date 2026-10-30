# M2-12 File system adapter: confined library walker and path resolver
Owner: mivo-backend
Depends on: M2-03, M2-01 (`domain.ErrUnavailable`)
Files/areas: `internal/adapters/fs/libraryfs/**` (new package)

## Goal
The scanner sees every video file inside a library root – and nothing outside it – and every
later open of a media file is confined to its root (ADR 0022 path safety).

## Acceptance criteria
- [ ] `Walk(ctx, root, visit func(library.FileState) error) error` opens the root with
      `os.Root` and walks it: only names accepted by `library.IsVideoFileName`, skipping
      `library.IsIgnoredDirectory` / `IsHiddenEntry`; reports relative slash paths
      (`library.ParseRelativePath`), size and mtime; stops with `ctx.Err()` when cancelled
      (checked per entry); unreadable subdirectories are logged and skipped, not fatal.
- [ ] A missing or unreadable root returns an error wrapping `domain.ErrUnavailable`.
- [ ] Symlinks: links inside the root are followed (file and directory); links escaping the
      root are skipped and logged at info; directory symlink loops terminate. Tests create links
      where the OS allows it (skip with a reason on Windows without the privilege).
- [ ] `Resolve(root, relativePath) (string, error)` returns the absolute path for FFmpeg after
      evaluating symlinks of root and file and verifying containment (case-insensitive on
      Windows); escape → `domain.ErrForbidden`; missing → `domain.ErrNotFound`.
- [ ] `Open(root, relativePath) (*os.File, error)` opens through `os.Root` (used by the direct-play
      handler, M2-29).
- [ ] Tests in `t.TempDir()`: nested folders, `@eaDir` and `.hidden` skipped, upper-case
      extensions, Unicode names, `..` in a relative path rejected, cancellation mid-walk.
- [ ] Gates: lint + layer rules + `go test -race` + coverage gate (package ≥ 85 %).

## Notes
- Package name must not be `fs`. No writes anywhere (Docker mounts `/media` read-only).
- `os.Root.FS()` plus `fs.WalkDir` is a natural fit; check how it reports escaping symlinks and
  test it on Windows and Linux (Docker run) – behaviour differs per OS.
- Logging through an injected `*slog.Logger`; paths in logs are relative to the root.
