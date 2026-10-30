# M1-22 test report: developer documentation and CLAUDE.md "Commands"

Status: **PASS** (round 1)
Date: 2026-10-30
Tester: mivo-tester
Task: `docs/tasks/M1-22-developer-commands-docs.md`

Scope: documentation, `lefthook.yml` and `scripts/commitmsg` only. Changed files tested:
`CLAUDE.md` (Commands section), `README.md`, `docs/development.md` (new),
`docs/adr/0019-quality-gates.md`, `lefthook.yml`, `scripts/commitmsg/{header,message}.go` and
their tests, `docs/tasks/BACKLOG.md` (sqlc row removed by the orchestrator).

Environment: Windows 11, PowerShell 7, Go 1.27.3, golangci-lint 2.15.0, clang 23.1.4
(LLVM-MinGW), Node 24.22.1, pnpm 10.34.5, Git 2.56.0, FFmpeg 9.0.2, Docker Engine 29.9.2.

## 1. Go gates in the real repository

| Gate | Result |
|---|---|
| `go vet ./...` | exit 0 |
| `go test -race "-coverprofile=coverage.out" ./...` (also with `-count=1`) | exit 0, all packages ok |
| `go tool "-modfile=tools/go.mod" go-test-coverage "--config=.testcoverage.yml"` | PASS, total 97.0 % (1555/1603), threshold 70 % |
| `internal/domain/...` coverage (`go tool cover -func`) | every function 100.0 %, threshold 90 % |
| `golangci-lint run` | `0 issues.` |
| `golangci-lint fmt --diff` | exit 0, empty diff |
| `go test -race -count=1 -cover ./scripts/commitmsg/...` | ok, 97.9 % |
| `go mod tidy -diff`, `go -C tools mod tidy -diff` | exit 0 |
| Layer rule: `go list -deps ./internal/domain/...` filtered for `database/sql`, `net/http`, `os/exec`, `internal/adapters`, `internal/app`; grep of `internal/domain` imports | no matches |

## 2. CLAUDE.md Commands, run verbatim in a scratch clone with spaces in the path

Clone: `...\scratchpad\qa m1-22\mivo clone` (fresh `git clone` of HEAD f314dae, plus the
working-tree changes copied over; no node_modules, dist or testdata at the start). All in
PowerShell. Every command exit 0 unless stated.

| Command | Result |
|---|---|
| `go vet ./...`, `go test ./...` | ok |
| `go test -race "-coverprofile=coverage.out" ./...` | ok (LLVM-MinGW present) |
| `go tool "-modfile=tools/go.mod" go-test-coverage "--config=.testcoverage.yml"` | PASS, 97.0 % |
| `golangci-lint run`, `golangci-lint fmt --diff` | 0 issues, empty diff |
| `go tool "-modfile=tools/go.mod" govulncheck ./...` [net] | `No vulnerabilities found.` |
| `go mod tidy -diff`, `go -C tools mod tidy -diff` | ok |
| `docker run ... -v gomodcache:/go/pkg/mod ... golang:1.27.3 go test -race ./...` [docker] | ok (Linux race run, all packages) |
| `go generate ./...` | exit 0, `git status` unchanged (no drift) |
| `pnpm -C web gen` | exit 0, no drift |
| `docker run ... sqlc/sqlc:1.31.1 generate -f internal/adapters/sqlite/sqlc.yaml` [docker] | exit 0, no drift (byte-identical `sqlcgen`) |
| `docker run ... sqlc/sqlc:1.31.1 diff -f ...` (development.md drift check) | exit 0 |
| `pnpm -C web install` [net] | ok, 7.6 s |
| `pnpm -C web lint` | exit 0 (ESLint, depcruise, Prettier, tsc) |
| `pnpm -C web test` | exit 0, coverage 100 % statements/branches/functions/lines |
| `pnpm -C web build` | exit 0, output in `internal/adapters/http/webui/dist` |
| `pnpm -C web dev` | Vite ready on :5173; `GET /api/v1/health` through the proxy -> 200 `{"checks":[{"name":"database","status":"ok"}],"status":"ok"}`; stopped |
| `pnpm -C web dev` with `$env:MIVO_DEV_API_TARGET='http://localhost:18460'` | proxy health 200 against a server on 18460 |
| `pnpm -C web/apps/web exec playwright install --only-shell chromium` | exit 0 (browser was already cached, so this was a no-op) |
| `pnpm -C web exec playwright install --only-shell chromium` (the task's form) | exit 1 `Command "playwright" not found` -> the documented alternative is justified |
| `pnpm -C web e2e` | exit 0, 10 passed (6.5 s) |
| `go vet -tags webui ./...` | ok |
| `golangci-lint run --build-tags webui` | `0 issues.` |
| `go build -tags webui -o bin/mivo.exe ./cmd/mivo` | ok, 19.8 MB |
| `go test -race -tags webui ./internal/adapters/http/... ./cmd/...` (development.md) | ok |
| `go run ./cmd/mivo` (throwaway config via the MIVO_* recipe in development.md, host 127.0.0.1) | `/api/v1/health` 200, `/` 200 placeholder page; stopped |
| `go run -tags webui ./cmd/mivo` | health 200, `/` 200 embedded app; stopped |
| `go run ./scripts/gen-testmedia` | exit 0, 43 generated, 0 failed, 9 gaps (local FFmpeg capability gaps) |
| `docker build -f packaging/docker/Dockerfile -t mivo:dev .` [docker][net] | exit 0 |
| `docker run -d --rm --name mivo-qa -p 8460:8460 --stop-timeout 20 mivo:dev` (development.md) | health 200 from the container; stopped, container gone |
| `go tool "-modfile=tools/go.mod" lefthook install` | exit 0, `sync hooks: (pre-commit, commit-msg)` |

Skipped: nothing over 10 minutes. Not exercised: a first-time Chromium download (cache
existed); the sqlc release-binary route (digests only, see section 3).

Documented PowerShell pitfalls confirmed: unquoted `-modfile=tools/go.mod` fails with
`file does not have .mod extension`; unquoted `-coverprofile=coverage.out` fails with
`no required module provides package .out` and leaves a stray `coverage*` file;
`CGO_ENABLED=0 go test -race` prints `-race requires cgo`.

## 3. Cross-document consistency

- Versions agree with the sources: `toolchain go1.27.3` (go.mod, tools/go.mod, Dockerfile
  `GO_IMAGE=golang:1.27.3`, docker race line), lefthook v2.2.2 (tools/go.mod), golangci-lint
  v2.15.0 (installed), `pnpm@10.34.5` and `node >=24` (web/package.json), sqlc 1.31.1.
- CLAUDE.md, docs/development.md, lefthook.yml and ADR 0019 use the same quoted
  `go tool "-modfile=tools/go.mod"` forms for install, coverage and govulncheck. The root
  `web/package.json` scripts (`lint`, `test`, `build`, `gen`, `dev`, `e2e`) exist and match.
  The Vite scripts use `--configLoader runner` as documented.
- sqlc release digests quoted in development.md match the GitHub API digests for
  `sqlc_1.31.1_windows_amd64.zip` and `sqlc_1.31.1_linux_amd64.tar.gz`; the release has no
  `checksums.txt` asset, as the docs say. The BACKLOG sqlc row is resolved by the Docker run above.
- `git diff CLAUDE.md` is one hunk, at "Commands (Windows dev machine, PowerShell)" only.
  It still needs the user's explicit approval before commit (orchestrator).
- Non-blocking: `.testcoverage.yml` (header comment, line 4) and `internal/adapters/http/apigen/generate.go`
  still show the unquoted `-modfile=` form. The second one is a go:generate line run by `sh`/
  go, which is fine. The first would fail if copy-pasted into PowerShell.
  ADR 0019 line 71 keeps the unquoted form but points to the quoted form in the next lines.

## 4. lefthook and commit-msg (scratch clone only)

- Hooks installed in the scratch clone only; the real repo has none added.
- Invalid message `bad message`: exit 1, `header is not a Conventional Commits header` plus format help.
- Valid message `chore: hook test`: exit 0, commit created.
- `LEFTHOOK=0` and `--no-verify` let an invalid message through; hooks active again afterwards (exit 1).
- `'{1}'` quoting: linked worktree `wt with spaces` (git dir
  `.../mivo clone/.git/worktrees/wt-with-spaces`, path contains spaces). With the old
  unquoted `{1}` the hook fails for both valid and invalid messages with
  `usage: commitmsg <commit message file>`. With the working-tree `lefthook.yml` (`'{1}'`) an
  invalid message is rejected with the right reason and a valid one is accepted. The fix works.

## 5. commitmsg behaviour

- Differential test (scratch module with HEAD's and the new `checkMessage`): 300 000 random
  messages built from headers, blank lines, trailers, comments, scissors line, BOM, CRLF and
  folded lines: 0 differences in accept/reject. Reason: only the header and the line
  directly below it are checked, and the removed code only stripped a trailing paragraph
  that is never the first one, so removing it cannot change the outcome.
- Help text now lists `Merge …, Revert "…", Reapply "…", fixup! …, squash! …, amend! …`; the
  test `TestCheckMessageErrorShowsTheExpectedFormat` asserts them (seen in the hook output above).
- New tests present: `Merge branch ... into main`, `Merge remote-tracking branch ... into ...`,
  nested `Revert "Revert "..""`, `Reapply "Revert ..."`, `Merge into main` rejected,
  trailers stay in the body, only `#` is stripped (commentChar limitation documented in
  `message.go`, `docs/development.md`).

## 6. Acceptance criteria

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | CLAUDE.md Commands section replaced, nothing else changed | met (approval pending with the orchestrator) | `git diff CLAUDE.md`: single hunk `@@ -55,17 +55,50 @@` |
| 2 | Command list covers the required commands | met | all listed commands present (quoted `-modfile` form, sqlc Docker 1.31.1, pnpm install/lint/test/build/gen/dev/e2e, Playwright `--only-shell chromium`, go run, tagged build, gen-testmedia, docker build, lefthook install) |
| 3 | Race detector line accurate for Windows, Docker alternative | met | LLVM-MinGW route and Docker `golang:1.27.3` run both executed; no-cgo error text confirmed |
| 4 | development.md: prerequisites with versions, Windows notes, ports, ADR 0017 drift checks, ADR 0002 pointer | met | sections Prerequisites, Shells, Code generation (drift check), Ports, Renaming; ADR 0002 has the rename checklist. Docker Desktop "4.97" not independently verified (Engine 29.9.2 seen) |
| 5 | README gets a Development section linking to development.md | met | `git diff README.md` |
| 6 | Every CLAUDE.md command runs in PowerShell or is marked Docker/network | met | section 2 |
| 7 | Gates stay green | met | section 1 |
| - | Orchestrator notes: `MIVO_DEV_API_TARGET`, `--configLoader runner` | met | documented and exercised |
| - | M1-21 carry-over: `'{1}'`, quoted install in ADR 0019, install/bypass/glob limitation, commitmsg nits, `--build-tags webui` | met | sections 3 to 5 |

## 7. Stray files and side effects

- Repo root: no `coverage` file. `coverage.out` (git-ignored, pre-existing) was regenerated by
  the gate run. `git status` of the real repo is exactly the implementer's change set; no hooks
  installed, nothing staged.
- `docker build ... -t mivo:dev` re-tagged the existing local `mivo:dev` image (same Dockerfile,
  cached build). The pre-existing `gomodcache` volume was reused. The `mivo-qa` container was
  removed. Scratch clone, worktree and differential-test directories removed from the scratchpad.
- `C:\c` unchanged (empty tree from earlier session, pre-existing).
- **`%LOCALAPPDATA%\Mivo` did not exist at the start and exists now** (`data\server.db`,
  `cache`, `transcode`, created 18:36:51, last write 18:37:02). All my server runs used
  `MIVO_CONFIG_DIR` and `MIVO_PATHS_*` pointing into the scratchpad, and no `mivo` process
  or listener is left, so I cannot attribute this directory to my runs (it appeared during
  the `docker run` check, which uses container storage). I left it untouched; the orchestrator
  or user may delete it if it is not theirs.

## History

- Round 1 (2026-10-30): PASS.

REPORT
status: PASS
task: M1-22
files: docs/reports/M1-22-test.md
tests: go vet, go test -race + coverage gate (97.0 % total, domain 100 %), golangci-lint run and fmt --diff, govulncheck: all exit 0. Every CLAUDE.md command run verbatim in a scratch clone with spaces in the path: all exit 0 (web lint/test/build/e2e 10 passed, tagged lint/build, run and health 200, gen-testmedia, docker build and run, sqlc Docker no drift). lefthook '{1}' fix proven in a linked worktree with spaces (old form fails with usage, new form works); commit-msg rejects invalid and accepts valid; differential test 300k messages 0 differences.
findings:
- non-blocking: `.testcoverage.yml` line 4 header comment still shows the unquoted `-modfile=` form (fails if pasted into PowerShell); ADR 0019 line 71 unquoted but followed by a quoted note
- non-blocking: Docker Desktop "4.97" in development.md not independently verified (Engine 29.9.2 seen); first-time Chromium download not exercised (cache existed)
- note: `pnpm -C web exec playwright install` from the task fails as the implementer said; documented alternative `pnpm -C web/apps/web exec ...` works
- note: `%LOCALAPPDATA%\Mivo` (empty DB, created 18:36-18:37) appeared during my session, not attributable to my runs (all used isolated config dirs); left untouched; local `mivo:dev` image was re-tagged by my docker build
- CLAUDE.md diff touches only the Commands section and still needs explicit user approval before commit
next: orchestrator obtains the user's approval for the CLAUDE.md diff, then hands to mivo-reviewer; optionally decide on the stray %LOCALAPPDATA%\Mivo directory
