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
  outside `cmd/`, package-level `slog` functions inside `internal/`), misspell, exhaustive,
  nolintlint (explanation required). Formatters: gofumpt, goimports (local prefix =
  module path). Generated files are excluded. `_test.go` files are exempt from funlen and mnd.
- **depguard layer matrix** (module prefix `github.com/magpeek/mivo`):
  | Files | May import | Must not import |
  |---|---|---|
  | `internal/domain/**` | std lib, `internal/domain` | `os`, `os/exec`, `net`, `net/http`, `database/sql`, `syscall`, `internal/app`, `internal/adapters`, `internal/platform`, `cmd` |
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
