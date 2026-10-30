# M2-03 test report: domain library

Status: **PASS** (round 1, 2026-10-30)
Scope: `internal/domain/library/**` (15 new files, 8 production, 7 test). Other packages were
mid-change in wave 1 and are not judged here.

## Gates

| Gate | Command | Result |
|---|---|---|
| vet | `go vet ./internal/domain/library/...` | exit 0 |
| test + race | `go test -race -count=1 -coverprofile=... ./internal/domain/library/...` | `ok ... coverage: 100.0% of statements` |
| coverage (domain >= 90 %) | `go tool cover -func` | every function 100.0 %, total 100.0 % |
| coverage gate | `go tool -modfile=tools/go.mod go-test-coverage --config=.testcoverage.yml --profile=<lib profile>` | `Total coverage threshold (70%) satisfied: PASS`, 100% (79/79). The package line prints the global default `(0%)`; the `^internal/domain(/[\w-]+)*$` 90 % override is configured (visible in `--debug`), and the real value is 100 %, so it holds either way |
| lint | `golangci-lint run ./internal/domain/library/...` | `0 issues.` |
| format | `golangci-lint fmt --diff ./internal/domain/library/...`, `gofmt -l` | no diff |
| layer rule (archtest) | `go test -count=1 ./internal/archtest/...` (incl. `TestRepositoryFollowsLayerMatrix`) | ok |
| layer rule (independent) | `go list -f '{{join .Imports "\n"}}' ./internal/domain/library` | only `fmt`, `internal/domain`, `path`, `strings`, `time`, `unicode`, `unicode/utf8`. No `database/sql`, `net/http`, `os/exec`, `os`, `path/filepath`, adapters, app, or `internal/domain/media`. `os` appears only in `go list -deps` through `fmt`/`time` |
| TODO / forbidigo | grep `TODO`, `filepath` in the package | none |

Web, E2E, media corpus and Docker gates: not applicable (domain-only task).

## Acceptance criteria

1. `Kind`, `ParseKind`, `Definition.Validate` (name non-blank and <= 128 runes, root non-empty
   without NUL, known kind, all wrapping `domain.ErrInvalid`): **met**. `definition.go:13-107`;
   tests include 129 `a` and 129 `ü` (rune count), NUL root, unknown kind. OS absoluteness is not
   checked, as required.
2. `Library{ID, Name, Root, Kind}` and `Item{ID, LibraryID, Kind, Title, SortTitle, CreatedAt}`:
   **met** (`item.go:12-68`). `Item.Kind` is the typed `ItemKind` (see decisions).
3. `IsVideoFileName`: **met** (`file_policy.go:16`). The table (`file_policy_test.go:16-47`) has
   `MOVIE.MKV` true, `real.rmvb` true, `sample.m4a`, `noextension`, `x.srt` and `movie.mkv.part`
   false. All 23 ADR 0022 extensions are present in the switch, which I compared one by one.
4. `IsIgnoredDirectory`, `IsHiddenEntry`: **met**. The tests cover `@eaDir`, `#recycle`,
   `#snapshot`, `.@__thumb`, `$RECYCLE.BIN`, `System Volume Information`, `lost+found`, and the
   leading dot, plus a negative `@eaDir2`.
5. `TitleFromFileName` / `SortTitle`: **met** (`title.go`). Extension stripped, `.` and `_`
   become spaces, whitespace collapsed and trimmed, never empty (falls back to the trimmed file
   name, then `Untitled`). The Unicode case (`Über_Film.2019.mkv`) is in `title_test.go`.
6. `ParseRelativePath`: **met**. Rejects empty, `/x`, `C:/x`, `\\server\x`, `..` segments,
   backslash, NUL and empty segments with `domain.ErrInvalid`. See the adversarial probe below.
7. `FileState` / `Changed`, ms precision: **met** (`file_state.go:16`). Tests: a difference of
   999,999 ns is ignored, +1 ms is detected, size and path changes are detected, and a
   stored-vs-disk case matches ADR 0004.
8. `PageRequest`, `NormalizeLimit` (0 gives 100, 1-500, else `ErrInvalid`): **met**
   (`page.go:29`). Tests cover the bounds including 501.
9. Tests table-driven, no mocks, coverage >= 90 %: **met**, 100 %. The tests are parallel and use
   the external `library_test` package.
10. Gates (lint, layer rules, coverage gate): **met**, see above.
11. Notes: no import of `domain/media` (checked via imports), no `path/filepath`, and slash paths
    use `path`/`strings`. **met**.

## Adversarial probe of ParseRelativePath

I tested a copy of `relative_path.go` in a scratch module outside the repo (nothing was added to
the repo). I also ran the same inputs against `os.Root.Open` on Windows. Results:

- **Rejected** (correct): `""`, `..`, `../x`, `a/../b`, `a/..`, `.`, `./a`, `a/./b`, `a\b`,
  `\\server\x`, `\\?\C:\x`, `\\.\pipe`, `a\x00b`, `/x`, `//x`, `C:/x`, `C:x`, `c:\x`, `a//b`,
  `a/` (trailing slash), and mixed separators with a backslash.
- **Accepted, and harmless** (no traversal):
  - `%2e%2e/x` and `%2e%2e%2fx`. The domain does not URL-decode, so these are literal file names.
  - Unicode lookalikes (`..<U+2024>/x`, fullwidth `..`, `U+2025`, `U+FF0F`, `U+2215`,
    `U+2216`). None is a path separator or dot-dot on any supported OS.
  - Control characters, BOM, invalid UTF-8.
- **Accepted on Windows, no escape found** (see findings F1 and F2): `.. /x`, `.../x`,
  `a./b`, `b.mkv.`, `b.mkv `, `CON`, `aux.mkv`, `file.mkv:Zone.Identifier`, `dir/a:b.mkv`,
  `dir/C:x`. Against a real `os.Root` on Windows: `.. /secret.txt`, `sub/.. /secret.txt`,
  `.../secret.txt` and `..../secret.txt` fail with "path not found". `sub/../../secret.txt` and
  `../secret.txt` fail with "path escapes from parent". `ok.txt::$DATA` fails. So the ADR 0022
  layered defence (`os.Root`) holds and no escape exists. The one aliasing case: `ok.txt.` and
  `ok.txt ` open the same file as `ok.txt` on Windows.

## Evaluation of the implementer's decisions

- **`ItemKind` (movie|episode), `ItemKindForLibrary`, `ParseItemKind`**: acceptable. The task
  names `Item.Kind` without a type, and ADR 0022 has an `items.kind` column. Library `movies`
  maps to `movie` and `shows` to `episode`, matching "one item per video file". The values
  `show`/`season` are deferred to milestone 3 (doc comment). It is a small addition beyond the
  task; the orchestrator or reviewer should confirm the mapping.
- **`ParseRelativePath` returns `string`**: acceptable. The task specifies none, and a plain string
  is the least ceremony. The canonical-form-only rule means one file has one stored path.
- **Rejecting `.` segments**: consistent with "canonical form only" and the ADR's no-`..` rule.
- **Rejecting drive-relative `C:x` and with it a top-level Linux file `a:b.mkv`**: ADR 0022 does
  not forbid it. It says paths are relative with `/` and never absolute, and the task rejects
  `C:/x`. Rejecting `C:x` is defensible on a Windows-first project, and the choice is
  documented in `relative_path.go:66-68`. Caveat (finding F1): the rule applies only to the
  first segment (`dir/a:b.mkv` is accepted), and a caller that treats the parse error as fatal
  would fail the scan on such a file. The scanner (M2-xx) should skip and log it.
- **`SortTitle` with `ToLower(ToUpper(r))`**: acceptable for "case-folded" in the ADR. The doc
  comment states that `ß` is not folded.
- **`FileState` mtime truncated to ms**: correct per task and ADR 0004. `UnixMilli` floors.

## Findings (non-blocking)

- F1 (`relative_path.go:69-72`): the drive-letter rule is inconsistent. It applies to the first
  segment only, so `a:b.mkv` is rejected while `dir/a:b.mkv` is accepted. Either document it as
  intentional (it is, partly) or have the scanner decide what to do with unparsable names. No
  failing test.
- F2 (`relative_path.go:55-64`): on Windows, segments with trailing dots or spaces and
  alternate data streams (`x.mkv:stream`) are accepted by the domain. No escape (checked with
  `os.Root`), but `ok.txt.` aliases `ok.txt`. The walker only produces names the file system
  reports, so this is a hardening idea for the adapter, not a defect against the criteria.

## History

- Round 1 (2026-10-30): all gates green, all criteria met, PASS.

REPORT
status: PASS
task: M2-03
files: docs/reports/M2-03-test.md
tests: go vet, go test -race -count=1 (internal/domain/library) ok, coverage 100.0 % (domain >= 90, gate >= 70 satisfied), golangci-lint run 0 issues, golangci-lint fmt --diff clean, archtest ok, go list imports clean
findings:
- All 9 acceptance criteria and the Notes are met; no failures.
- F1 (non-blocking): drive-letter rejection covers only the first segment, so `a:b.mkv` is rejected but `dir/a:b.mkv` is accepted. ADR 0022 does not forbid the rule. The scanner should skip and log a name that fails to parse.
- F2 (non-blocking): trailing dot/space segments and `file:stream` pass the domain check. Checked against Windows `os.Root`: no escape, only the `ok.txt.` alias of `ok.txt`.
- Decisions (ItemKind, string return, `.` segment rejection, simple case folding, ms-truncated mtime) are acceptable. ItemKind is a small addition beyond the task text, for the reviewer to confirm.
next: Forward to mivo-reviewer; no implementer rework needed.
