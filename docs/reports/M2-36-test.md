# M2-36 test report

Task: M2-36 | Round 1 | Tester: mivo-tester | Result: PASS

Scope: docs-only change (README.md, docs/development.md, docs/tasks/BACKLOG.md, three ADR notes,
`docs/reports/M2-release-readiness.md`, comments in `.testcoverage.yml` and `tools/go.mod`).
The long runs (Docker smoke, E2E, race) were not repeated; they are recorded in
`docs/reports/M2-release-readiness.md`.

## Gates

| Gate | Result |
|---|---|
| `golangci-lint run` | 0 issues |
| `go test ./...` | all packages ok, no FAIL |
| `pnpm -C web lint` | ESLint, dependency-cruiser (no violations, 304 modules), Prettier, `tsc -b` all clean (node_modules already present) |

## Links

All `](` targets in the 7 changed Markdown files were resolved against the tree, anchors
included: no broken link, no bad anchor. Only README.md has inline links (6); the other files
reference paths in backticks, which are not links. Backtick-path spot checks exist
(`packaging/docker/README.md` "Verify an image", `internal/adapters/corpustest`).

## Documented commands vs. the scripts

- `gen-testmedia` flags `-out -ffmpeg -force -only -jobs` match the doc. `-only` takes
  comma-separated globs and the doc example `'video-mp4-*,video-webm-vp9-opus'` is the one in
  the script's own help text. The quoting advice (PowerShell array, bash glob) is correct.
- `playback-smoke` flags `-base -manifest -profile -ffprobe -case-timeout -scan-timeout` match;
  defaults `2m` and `5m` match the constants.
- Quoting of dotted arguments (`"-modfile=tools/go.mod"`, `"-coverprofile=coverage.out"`,
  `"--config=.testcoverage.yml"`) is correct in both shells. Environment variable examples
  use `$env:` for PowerShell and a prefix form for bash. `MSYS_NO_PATHCONV` and `Remove-Item
  Env:A, Env:B` are valid. `"${PWD}/testdata/media:/media:ro"` works in both.
- Comment-only edits in `.testcoverage.yml` and `tools/go.mod` changed no code lines.

## Readiness report sections

Present: release candidate (commit, checkout, tools), 1. gate list with command, result and
time (21 rows), 2. corpus generation and corpus integration test, 3. E2E with the three
projects (`library`, `chromium`, `branded`: 31 passed, 0 skipped), 4. Docker build and smoke
for both profiles (30 and 28 cases passed), 5. Synology package ("not ready - attached
later", OD-SPK-1 A), 6. documentation and backlog, 7. open decisions, cleanup.

## BACKLOG

- No row is due "before 0.1.0-alpha.2"; the remaining mentions are "after 0.1.0-alpha.2".
- No row "docs coverage without cgo" or "docs nits" is left; the header records them as done in
  M2-36 except the CLAUDE.md part, which stays as the row "docs / CLAUDE.md".

## Acceptance criteria

| Criterion | Status |
|---|---|
| development.md content (config, corpus, `-only`, `-short`, smoke, E2E projects, run time) | met |
| BACKLOG rows done | met |
| README can / cannot / LAN-only | met (verified by reading the sections) |
| CLAUDE.md commands | not part of this change; waits for owner approval, tracked in the BACKLOG row and open decision 3 of the report (not a failure) |
| BACKLOG lists the open M2 findings | met (one overdue row is flagged, see below) |
| Readiness report | met |
| Relative links resolve | met |
| Go and web gates unchanged | met |

## Observations (non-blocking)

- BACKLOG row 41 (http 5xx log text, M2-08 review) is due "before M2-28/M2-29 add client-derived
  error text", which is already past. The implementer flagged it in the readiness report
  (section 6, decision 5); it needs a new due date from the owner.
- The smoke doc uses `ls /transcode/sessions` and the report `ls -A`; both fine.

## History

- Round 1: PASS.

REPORT
status: PASS
task: M2-36
files: docs/reports/M2-36-test.md
tests: golangci-lint run (0 issues); go test ./... (all ok); pnpm -C web lint (clean); link check over 7 Markdown files (no broken links or anchors); doc commands vs script flags (match)
findings:
- No failures.
- BACKLOG row 41 (http 5xx log text limit) is overdue against its own due date. The implementer already flagged it as open decision 5. The owner has to set a new due date.
- The CLAUDE.md Commands change is still pending owner approval, as planned.
next: forward to mivo-reviewer.
