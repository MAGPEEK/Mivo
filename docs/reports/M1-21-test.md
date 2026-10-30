# M1-21 test report (lefthook hooks)

Status: PASS (round 1)
Scope: `lefthook.yml`, `scripts/commitmsg/**`, `tools/go.mod`, `tools/go.sum`, ADR 0019 note.
`cmd/mivo` is mid-change by M1-10 and was excluded from the package-wide gates.

## Gates

| Gate | Result | Evidence |
|---|---|---|
| `go -C tools mod tidy -diff` | pass | no output, exit 0 |
| `go mod tidy -diff` | pass | no output, exit 0 |
| `go test -race -cover ./scripts/commitmsg/...` | pass | `ok ... coverage: 98.6% of statements` |
| `go vet ./scripts/...` and all packages except cmd/mivo | pass | exit 0 |
| `golangci-lint run ./scripts/...` | pass | `0 issues.` |
| `golangci-lint run ./internal/... ./api/... ./scripts/...` | pass | `0 issues.` |
| `golangci-lint fmt --diff ./scripts/...` | pass | no output, exit 0 |
| `go tool "-modfile=tools/go.mod" lefthook validate` | pass | `All good` |
| `go test` all packages except cmd/mivo | pass | all `ok` |
| `go generate ./...` after tools bump | pass | `apigen.gen.go` SHA-256 before and after both `2FA607AA...D6C83B`; `git status` shows the file unmodified (identical to HEAD) |

Web lint was not run: no web files changed. Layer rule / domain coverage thresholds are not
affected (no `internal/` change).

## Acceptance criteria

1. `lefthook.yml` defines pre-commit (parallel) with go-format (`golangci-lint fmt --diff
   {staged_files}`), go-lint (per package of staged files), go-test (`./internal/domain/...
   ./internal/app/...`), web-format (prettier --check), web-lint (eslint), web-test
   (`vitest related --run --coverage.enabled=false`), and commit-msg `go run ./scripts/commitmsg {1}`:
   MET. Observed in the hook runs below. Behavioural deviation from the letter: commit-msg
   uses `go run ./scripts/commitmsg {1}` exactly as specified.
2. `scripts/commitmsg` formats, Merge/Revert, comments and trailers, error message with
   expected format, table-driven tests: MET (see table-test review).
3. Windows install without global tool: MET. `go tool "-modfile=tools/go.mod" lefthook install`
   printed `sync hooks: (pre-commit, commit-msg)`; `.git/hooks/pre-commit` and
   `.git/hooks/commit-msg` exist and call `go tool -modfile=tools/go.mod lefthook`. Hooks ran
   from `git commit` in PowerShell (Git's sh). Method is documented in `lefthook.yml` header and
   ADR 0019; the orchestrator must carry it into the M1-22 REPORT.
4. Temporary clone: MET.
   - `git commit --allow-empty -m "update stuff"` rejected (exit 1, format help printed).
   - `git commit --allow-empty -m "chore: test"` accepted.
   - `lefthook run pre-commit` on a clean tree: 0.45 s (limit 60 s). With `--all-files` (all
     six jobs actually executing, 264 vitest tests): 14.2 s.
5. Gates (`golangci-lint`, `go test`, coverage): MET for everything except `cmd/mivo`
   (excluded by instruction, not verified here).

## Hook behaviour in the scratch clone (hooks installed there only)

| Scenario | Result |
|---|---|
| Staged badly formatted Go file | rejected, exit 1. go-format showed the diff; go-lint reported gofumpt/goimports/revive; go-test still passed. 1.3 s |
| Staged TS file with `any` (eslint `no-explicit-any` x2, also prettier) | rejected, exit 1. web-lint and web-format failed. 4.9 s |
| Staged clean Go file + clean TS file, `chore: clean files` | committed. All six jobs green, 4.4 s hook + 0.2 s commit-msg |
| `chore: test` empty commit | all jobs skipped ("no files"), commit-msg ok |

## Table-test review (own runs of the built checker against message files)

Exit codes (0 accept, 1 reject):

- accept: `Merge branch 'main' of https://...`, `Merge pull request #12 from u/b`,
  `Revert "feat: add x"` (with body), `Reapply "feat: add x"`, `fixup! feat: add x`,
  `squash! feat: add x`, `feat(play-back2)!: x`, `fix!: x`, `revert: feat x`, valid header +
  blank line + body, header + trailer block (`Co-Authored-By`), body + `Signed-off-by`,
  comment lines around a valid header, scissors section (`git commit -v`), leading blank lines,
  trailing whitespace on the header, CRLF line endings, UTF-8 BOM.
- reject: `Merge stuff`, `feat(Scope): x` (uppercase scope), `feat(): x`, `Feat: x`,
  `feat:x`, `wip: x`, `feat: x` directly followed by `body line` (missing blank line, also
  with CRLF and with a trailer-shaped line), `feat: x` + `Continued: y`, comments only, empty
  file, `Reviewed-by: Z` as sole header.
- CLI: no argument -> usage, exit 2; missing file -> `read message file: ...`, exit 1.

All matched the specification.

## Observations (non-blocking)

- The generated hook scripts contain a fallback branch with an absolute `go-build` cache path
  to lefthook.exe. It is only reached if `go tool` fails and is harmless.
- The first `go tool lefthook` call after a fresh clone compiles lefthook (cold Go cache); the
  timings above were measured with a warm cache.
- go-lint lints only packages containing staged files, as documented in ADR 0019.

## History

- Round 1: PASS.

REPORT
status: PASS
task: M1-21
files: docs/reports/M1-21-test.md
tests: tidy diff (both modules) clean; go test -race -cover ./scripts/commitmsg/... ok 98.6%; golangci-lint run + fmt --diff 0 issues; lefthook validate ok; go generate byte-identical; hook scenarios in a temporary clone all as expected (clone deleted); cmd/mivo excluded
findings: none blocking; pre-commit on clean tree 0.45 s, all-files 14.2 s
next: orchestrator proceeds to review; carry the "go tool -modfile=tools/go.mod lefthook install" install method into the M1-22 REPORT
