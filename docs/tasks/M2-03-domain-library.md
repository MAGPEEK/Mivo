# M2-03 Domain: libraries, items and file policies
Owner: mivo-backend
Depends on: none
Files/areas: `internal/domain/library/**` (new package)

## Goal
Pure rules for how libraries are organised: library definitions and kinds, which files count as
videos, which folders are skipped, how a title is derived from a file name, safe relative paths,
change detection and paging (ADR 0022).

## Acceptance criteria
- [ ] `Kind` (`movies`, `shows`) with `ParseKind`; `Definition{Name, Root, Kind}` with
      `Validate()` (non-empty name ≤ 128 runes, non-empty root without NUL, known kind) →
      `domain.ErrInvalid`. OS-specific absoluteness is checked in config, not here.
- [ ] `Library{ID, Name, Root, Kind}` and `Item{ID, LibraryID, Kind, Title, SortTitle,
      CreatedAt}`.
- [ ] `IsVideoFileName(name)`: case-insensitive extension allow-list of ADR 0022; table test
      incl. `MOVIE.MKV`, `.rmvb`, `sample.m4a` (false), `noextension` (false), `x.srt` (false).
- [ ] `IsIgnoredDirectory(name)` and `IsHiddenEntry(name)`: `@eaDir`, `#recycle`, `#snapshot`,
      `.@__thumb`, `$RECYCLE.BIN`, `System Volume Information`, `lost+found`, leading dot.
- [ ] `TitleFromFileName(name)` strips the extension, turns `.` and `_` into spaces, collapses
      whitespace, trims; never returns empty (falls back to the file name). `SortTitle` is the
      case-folded title. Table test with Unicode (`Über_Film.2019.mkv`).
- [ ] `ParseRelativePath(string)` accepts slash-separated relative paths only: rejects empty,
      absolute (`/x`, `C:/x`, `\\server\x`), `..` segments, backslashes, NUL, empty segments →
      `domain.ErrInvalid`.
- [ ] `FileState{RelativePath, Size, ModifiedAt}` and `Changed(previous, current)`; mtime compared
      at millisecond precision (matches SQLite storage, ADR 0004).
- [ ] `PageRequest{Limit, Cursor}` with `NormalizeLimit` (default 100, 1–500, else
      `ErrInvalid`).
- [ ] Tests: table-driven, no mocks; coverage ≥ 90 %.
- [ ] Gates: lint + layer rules + coverage gate pass.

## Notes
- Parallel to M2-02; do not import `internal/domain/media`.
- No `path/filepath` file-system functions in the domain (forbidigo); use `path` and
  `strings` for slash paths.
- The name parser (year, season/episode) is milestone 3; keep `TitleFromFileName` simple and
  replaceable.
