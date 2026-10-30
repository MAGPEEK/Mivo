# 0019 Quality gates and their enforcement

- Status: Accepted (2026-10-30, confirmed by project owner)
- Date: 2026-10-29
- Refines: "Werkzeuge & Qualitäts-Gates" in `docs/plan.md`

## Context

`CLAUDE.md` defines code rules (function size, complexity, parameters, layering, coverage).
Rules nobody checks erode, so each one needs a tool and a command that `mivo-tester` can run.

## Decision

**Go**
- **golangci-lint v2** (`.golangci.yml`, `version: "2"`). Linters at minimum: depguard,
  gocognit (15), funlen (50 lines), revive (argument-limit 4, exported doc comments), errcheck,
  errorlint, gosec, govet, staticcheck, unused, ineffassign, contextcheck, noctx, bodyclose,
  sqlclosecheck, rowserrcheck, mnd, gochecknoglobals, gochecknoinits, forbidigo (`panic`
  outside `cmd/`, package-level `slog` functions inside `internal/`, file system functions of
  `path/filepath` and package-level `log` functions inside `internal/domain/`), misspell, exhaustive,
  nolintlint (explanation required). Formatters: gofumpt, goimports (local prefix =
  module path). Generated files are excluded. `_test.go` files are exempt from funlen and mnd.
- **depguard layer matrix** (module prefix `github.com/magpeek/mivo`):
  | Files | May import | Must not import |
  |---|---|---|
  | `internal/domain/**` | std lib (incl. `net/url`, `net/netip`, `net/mail`), `internal/domain` | `os/...`, `net`, `net/http/...`, `net/rpc/...`, `net/smtp`, `net/textproto`, `crypto/tls`, `database/sql/...`, `syscall/...`, `io/ioutil`, `log/syslog`, `expvar`, `plugin`, `C` (cgo), `internal/app`, `internal/adapters`, `internal/platform`, `cmd`, third party |
  | `internal/app/**` | std lib, `internal/domain`, `internal/app` | `os/exec`, `net/http`, `database/sql`, `internal/adapters`, `internal/platform`, `cmd` |
  | `internal/platform/**` | std lib, third party | `internal/domain`, `internal/app`, `internal/adapters`, `cmd` |
  | `internal/adapters/**` | anything except `cmd` | `cmd` |
- An **architecture test** (`internal/archtest`) checks the same matrix with `go/parser`, so
  `go test ./...` catches violations even without golangci-lint.
- **Coverage**: `go-test-coverage` (pinned `go tool`, `.testcoverage.yml`) on
  `coverage.out`: total ≥ 70 %, `internal/domain/...` ≥ 90 %. Generated packages and
  `internal/archtest` are excluded.
- `govulncheck` as a pinned `go tool` (needs network access to the vulnerability database).

**Web**
- ESLint 9 flat config: typescript-eslint `strictTypeChecked` + `stylisticTypeChecked`,
  react, react-hooks, jsx-a11y (strict), import-x `no-cycle`, sonarjs `cognitive-complexity`
  15, `max-lines-per-function` 50, `max-params` 4, `no-explicit-any` as error, and no
  hex colour literals outside `packages/ui-tokens`.
- Prettier (LF, checked in lint).
- dependency-cruiser: no circular dependencies; `packages/*` never import `apps/*`;
  `player-core` and `api-client` never import React; `src/shared/**` never imports
  `src/features/**` or `src/app/**`; a feature never imports another feature's internals;
  generated client code is imported only through the package entry.
- Vitest with v8 coverage **enabled by default** with thresholds: global ≥ 70 % (lines,
  branches, functions, statements), `packages/player-core` ≥ 90 %.

**Hooks and CI**
- **lefthook**: pre-commit runs format check, fast lint and fast tests on staged files, with
  coverage disabled for partial runs. commit-msg enforces Conventional Commits. Full gates run
  in CI (GitHub Actions, separate task, not part of milestone 1).

## Consequences

- Every `CLAUDE.md` rule maps to a failing command.
- Thresholds are never lowered to get a green run (`mivo-tester` rule).
- On Windows, `go test -race` needs cgo and a C toolchain. Without one, the race detector
  runs in Docker or CI (see `docs/development.md`, task M1-22).

## Notes

Added 2026-10-30 with task M1-03b:

- **funlen** runs with `ignore-comments: true`: the 50-line limit counts code, so doc
  comments explaining *why* never push a function over the limit.
- **Tool modfile**: the pinned Go tools (go-test-coverage, govulncheck) live in
  `tools/go.mod`, not in `go.mod`, so their dependencies (AWS SDK, go-github, freetype, …)
  stay out of the production module graph. Run them from the repository root with
  `go tool -modfile=tools/go.mod go-test-coverage --config=.testcoverage.yml` and
  `go tool -modfile=tools/go.mod govulncheck ./...` (in PowerShell quote the argument:
  `"-modfile=tools/go.mod"`, see `docs/development.md`). Both `go.mod` files pin
  `toolchain go1.27.3`, so `GOTOOLCHAIN=auto` fetches the patched toolchain.
- **archtest vs depguard scope**: depguard also lints `_test.go` files and matches
  `**/internal/domain/**` at any depth; archtest checks production files only, anchors
  layers at the module root, skips `testdata`, `_x` and `.x` directories and ignores build
  constraints. depguard's `$gostd` allow list would win over prefix denies, so
  `.golangci.yml` denies every standard library package of a forbidden tree exactly
  (`os`, `os/exec`, `os/signal`, `os/user`, `net/http/...`, `net/rpc/...`,
  `database/sql/...`, `syscall/...`); archtest matches these trees by prefix and therefore
  also catches packages a future Go release adds. An archtest test expands the matrix
  against `go list std` and fails until `.golangci.yml` lists the new package. The domain
  row forbids every standard library package that does network, file or process I/O
  (`net/smtp`, `crypto/tls`, `io/ioutil`, `log/syslog`, `expvar`, `plugin`, cgo, …);
  pure parsers such as `net/url`, `net/netip` and `net/mail` stay allowed. Functions of
  allowed packages that touch the file system (`filepath.Walk`, `WalkDir`, `Glob`,
  `EvalSymlinks`, `Abs`) or the standard logger (`log.Print*`, `log.Fatal*`, `log.Panic*`)
  are forbidden in the domain by forbidigo.

Added 2026-10-30 with task M1-21:

- **lefthook** is a pinned Go tool in `tools/go.mod` (no global or npm install). Install the
  hooks per clone with `go tool "-modfile=tools/go.mod" lefthook install` (PowerShell needs
  the quotes, it would split the unquoted argument at the dot); the `lefthook:` key
  in `lefthook.yml` makes the generated `.git/hooks` scripts call the same command, so they
  work from PowerShell and Git Bash. pre-commit lints only the packages that contain staged
  Go files (golangci-lint needs whole packages); breakage in importing packages is caught by
  the full gates. commit-msg runs `scripts/commitmsg`, which also accepts git's own merge,
  revert and `fixup!`/`squash!`/`amend!` subjects.
