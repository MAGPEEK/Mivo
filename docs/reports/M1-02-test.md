# M1-02 test report

Status: PASS
Round: 1
Date: 2026-10-30
Toolchain: go1.27.0 windows/amd64, golangci-lint 2.15 (default config, `.golangci.yml` arrives in M1-03)

## Gates

| Gate | Command | Result |
|---|---|---|
| vet | `go vet ./...` | exit 0, no output |
| test + race | `go test -race -coverprofile=coverage.out ./...` | all `ok` (cmd/mivo, brand, buildinfo), exit 0 |
| lint | `golangci-lint run ./...` | `0 issues.` (default linters only, so `gochecknoglobals`/`depguard` are not active yet) |
| web gates | skipped | out of scope (M1-11 runs in parallel) |
| media / docker | skipped | task does not touch them |

## Coverage

| Package | Coverage |
|---|---|
| cmd/mivo | 87.2 % |
| internal/platform/buildinfo | 97.1 % |
| internal/platform/brand | no statements (constants only) |
| internal/{domain,app,adapters,platform} | no test files (doc.go only) |
| total | 91.8 % (threshold 70 %: met) |

`internal/domain` has no statements, so the 90 % threshold is vacuously met. Uncovered
statements: `main` (`os.Exit` wrapper), the stdout-write error branch in `printVersion`, and
the `ReadBuildInfo` not-ok branch in `Read`.

## Layer rule (independent check)

`go list -deps ./internal/domain` prints only `github.com/magpeek/mivo/internal/domain`.
No standard-library or other package is imported, so there is no `database/sql`, `net/http`,
`os/exec`, `internal/adapters` or `internal/app` dependency. Met.

## Acceptance criteria

1. **go.mod module and go directive**: met. `go.mod` has `module github.com/magpeek/mivo` and
   `go 1.27.0`. That equals the installed Go 1.27.0 and is at least 1.25.
2. **`ignore ./web`**: met. `go.mod:6` has `ignore ./web`. `go list ./...` prints only
   `cmd/mivo` and `internal/{adapters,app,domain,platform,platform/brand,platform/buildinfo}`.
   Nothing under `web/` appears.
3. **doc.go files state the import rule**: met. All four files have a package doc comment with
   an "Import rule:" paragraph that references ADR 0003.
4. **brand constants and single literal**: met. `brand.go` defines `DisplayName="Mivo"`,
   `ID="mivo"`, `EnvPrefix="MIVO"` and `RepositoryURL="https://github.com/MAGPEEK/Mivo"`.
   `Select-String -Pattern '"Mivo"' -CaseSensitive` over `cmd/` and `internal/` finds only
   `internal/platform/brand/brand.go:9`. A search for `Mivo` finds only `brand.go:9` and
   `brand.go:18`.
5. **buildinfo `Read() Info`**: met. `Info` has `Version`, `Commit`, `CommitTime`, `Modified`
   and `GoVersion`. `resolveVersion` applies override > `Main.Version` (ignoring `(devel)` and
   empty) > `0.0.0-dev`. Commit data is read from `vcs.revision`, `vcs.time` and
   `vcs.modified`.
6. **`go run ./cmd/mivo --version`**: met. Output is `Mivo 0.0.0-dev (go1.27.0)`, one line,
   exit 0.
7. **ldflags override**: met. After `go build -ldflags "-X
   github.com/magpeek/mivo/internal/platform/buildinfo.version=9.9.9" -o bin/mivo.exe
   ./cmd/mivo`, `./bin/mivo.exe --version` prints
   `Mivo 9.9.9 (commit 090fcaaebb39, modified, go1.27.0)`. A build without the override prints
   `Mivo v0.0.0-20261030102718-090fcaaebb39+dirty (commit 090fcaaebb39, modified, go1.27.0)`,
   so the VCS pseudo-version path works.
8. **`--unknown-flag`**: met. The binary exits 2 and writes this to stderr, with no panic:
   `flag provided but not defined: -unknown-flag`, then `Usage of mivo:` and `-version`.
9. **Tests**: met.
   - `TestResolveVersion` is table-driven with 6 cases. `resolve(*debug.BuildInfo, string)` is
     pure and covers the override, `(devel)`, empty, nil and module-version cases.
   - `TestResolveVCSSettings` and `TestInfoSummary` are table-driven.
   - The brand tests check that the id is lowercase and that `EnvPrefix == ToUpper(ID)`.
   - `TestRun` in `cmd/mivo` is table-driven and covers version, unknown flag, `-h`, a
     positional argument and no arguments.
10. **Gates `go vet` and `go test`**: met (see Gates table).

## Notes (non-blocking)

- `//nolint:gochecknoglobals` on `buildinfo.version` is correct per the task, but the linter
  that enforces it is not configured yet. M1-03 must verify the directive is accepted
  (`nolintlint`).
- No `go.sum` exists, which is correct because there are no third-party dependencies.
- `go build` stamps `+dirty` and `modified` because the tree is uncommitted. This is expected.
- My first `go test` call was malformed by PowerShell argument splitting and created a stray
  `coverage` file in the repo root. I deleted it. `bin/` and `coverage.out` are git-ignored.
  Nothing was staged.

## History

- Round 1: PASS. All gates green and all 10 criteria met.

REPORT
status: PASS
task: M1-02
files: docs/reports/M1-02-test.md
tests: go vet ./... (clean); go test -race -coverprofile ./... (all ok, total 91.8 %); golangci-lint run (default config, 0 issues); manual CLI checks for --version, ldflags and --unknown-flag (all as specified)
findings: none blocking. Note for M1-03: verify the nolint:gochecknoglobals directive once the golangci config exists.
next: forward to mivo-reviewer
