# M1-03 test report: Go quality gates

Status: PASS (round 1)
Tools: golangci-lint 2.15.0 (go1.27.0), go 1.27.0 windows/amd64

## Gates

| Gate | Command | Result |
|---|---|---|
| Config | `golangci-lint config verify` | exit 0 |
| Lint | `golangci-lint run` | `0 issues.` |
| Format | `golangci-lint fmt --diff` | exit 0, no diff |
| Vet | `go vet ./...` | exit 0 |
| Tests | `go test -race '-coverprofile=coverage.out' ./...` | all ok (cmd/mivo 87.2 %, archtest 94.2 %, buildinfo 97.1 %) |
| Coverage gate | `go tool go-test-coverage '--config=.testcoverage.yml'` | Total 70 % PASS; total 91.8 % (67/73); exit 0 |
| Vulnerabilities | `go tool govulncheck ./...` | `Your code is affected by 0 vulnerabilities.` 1 in imported packages and 13 in required modules, not called |
| Module hygiene | `go mod tidy -diff`, `go mod verify` | clean, all modules verified |

Note: `go tool cover -func` reports 93.1 % total (it includes archtest); the gate figure
91.8 % excludes archtest as configured.

## Layer rule independent check

Created `internal/domain/probe.go` with `import _ "database/sql"`:

- `golangci-lint run`: `internal\domain\probe.go:3:8: import 'database/sql' is not allowed from list 'domain': the domain is pure; persistence belongs to adapters/sqlite (depguard)` (plus a revive blank-imports hit), exit 1.
- `go test ./internal/archtest/...`: `tree_test.go:33: layer violation: internal/domain/probe.go imports "database/sql": internal/domain must not import it`, FAIL.
- Reverted: probe file deleted (it was untracked, so `git checkout` was not applicable); `git status` shows no leftovers in `internal/domain`.

## Extra probe: domain coverage override

The real tree has no domain code yet, so the output reads `Package coverage threshold (0%)`
and the 90 % override is not exercised. Probed with a temporary package
`internal/domain/probecov` at 50 % coverage:
`internal/domain/probecov 50.0% (1/2) 90%` -> `Package coverage threshold (0%) satisfied: FAIL`, exit 1.
The override regex works for sub-packages. Probe removed, tree clean.

## Acceptance criteria

- [x] v2 format, `config verify` passes (`version: "2"`, exit 0).
- [x] Linters and settings match ADR 0019: funlen 50, gocognit 15, revive argument-limit 4 + exported, forbidigo for `panic` and package-level slog (restricted to `internal/` via exclusion), gofumpt + goimports with local prefix `github.com/magpeek/mivo`. All ADR-listed linters are enabled (checked against `.golangci.yml:19-42`).
- [x] depguard matrix for domain, app, platform, adapters present (`.golangci.yml:45-104`); domain probe confirmed it triggers.
- [x] Generated files excluded (`generated: strict`); `_test.go` exempt from funlen and mnd only (`.golangci.yml:173-176`).
- [x] `golangci-lint run` and `fmt --diff` clean.
- [x] `internal/archtest`: go/parser `ImportsOnly` walk (`tree.go`, `layers.go:96`), unit-tested with in-memory fixtures, one allowed and one forbidden case per layer (`layers_test.go:17-126`), real-tree test passes.
- [x] `go-test-coverage` and `govulncheck` pinned as `tool` in `go.mod`; total >= 70 % and domain >= 90 % enforced (probe-verified); `apigen`, `sqlcgen`, `internal/archtest` excluded (`.testcoverage.yml:18-24`).
- [x] `go tool govulncheck ./...` runs: 0 affecting vulnerabilities.
- [x] Temporary violation produced depguard error and archtest failure, reverted.
- [x] Gates pass.

## Observations (non-blocking)

- `go-test-coverage` prints "Package coverage threshold (0%)" because the override is
  per-package and the default package threshold is 0. Behaviour is correct (see probe).
- The domain 90 % threshold is vacuous until domain code exists; do not lower later.
- The `app` depguard list is `list-mode: strict` with only std lib, domain and app allowed, which is stricter than the ADR table ("third party" is implicitly forbidden in app too). The archtest mirrors this, so the two are consistent.

## History

- Round 1: PASS.
