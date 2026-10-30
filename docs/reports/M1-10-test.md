# M1-10 test report: composition root

Status: **PASS** (round 1)

Scope: full repo gates plus a smoke run of the built binary. Changed files:
`cmd/mivo/main.go`, `cmd/mivo/serve.go` (new), `cmd/mivo/main_test.go`, `cmd/mivo/serve_test.go` (new).
Orchestrator decision applied: address and version may appear in two consecutive log lines.

## Gates

| Gate | Command | Result |
|---|---|---|
| vet | `go vet ./...` | exit 0 |
| tests | `go test -race -count=1 -coverprofile=<scratch>/coverage.out ./...` | all `ok`, exit 0 |
| coverage | `go tool -modfile=tools/go.mod go-test-coverage --config=.testcoverage.yml --profile=<scratch>/coverage.out` | package thresholds PASS, total threshold (70%) PASS, **total 97.1 % (1415/1458)** |
| lint | `golangci-lint run` | `0 issues.` |
| format | `golangci-lint fmt --diff` | no diff, exit 0 |
| tidy (root) | `go mod tidy -diff` | exit 0 |
| tidy (tools) | `go mod tidy -diff` in `tools/` | exit 0 (see note) |
| vulnerabilities | `go tool -modfile=tools/go.mod govulncheck ./...` | `No vulnerabilities found.` |
| archtest | `go test -count=1 -v ./internal/archtest/` | all PASS (incl. `TestRepositoryFollowsLayerMatrix`, `TestDepguardDeniesEveryStandardPackageOfTheMatrix`) |
| layer rule | `go list -deps ./internal/domain/...` filtered for `database/sql\|net/http\|os/exec\|internal/adapters\|internal/app` | no matches |
| app layer | `go list -deps ./internal/app/...` filtered for adapters/platform/net/http/database/sql | no matches |

Note on tidy (tools): a first attempt with `go mod tidy -diff -modfile=tools/go.mod` from the root
reported a diff. That form applies the tools modfile to the root packages, so it is the wrong check
for the separate tools module. Running it inside `tools/` is clean. Not a finding.

### Coverage

| Package | Coverage |
|---|---|
| `cmd/mivo` | 86.7 % (`main()` 0 % as expected; `serve.go` error branches 75 to 88 %) |
| `internal/domain/system` | 100 % (threshold 90 %) |
| total | 97.1 % (threshold 70 %) |

## Acceptance criteria

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | No config file: default directories (ADR 0018), `<data>/server.db`, listening log line with address and version, port 8460 | met | Smoke run 1 (isolated `LOCALAPPDATA`, no `MIVO_SERVER_PORT`): created `Mivo\cache`, `Mivo\data`, `Mivo\transcode`, `Mivo\data\server.db`. Log: `msg="starting Mivo" version="v0.0.0-...+dirty (commit eef27b7b46f4, ...)"` then `msg="http server listening" address=[::]:8460`, in that order (accepted per orchestrator decision). |
| 2 | `/api/v1/health` returns `status = ok` | met | HTTP 200, `{"checks":[{"name":"database","status":"ok"}],"status":"ok"}` |
| 3 | `/api/v1/system/info`: `productName = Mivo`, version, `serverId`; same after restart | met | HTTP 200, `productName":"Mivo"`, `version":"v0.0.0-2026...+dirty"`, `serverId":"2e2e0b1a-aa61-4406-bbd6-42bef233b920"`. Restart on same data dir returned the identical id (twice: run 2 and run 3). |
| 4 | `--config-dir` and `MIVO_CONFIG_DIR` select the config dir; `MIVO_*` overrides apply; `--version` works | met | Run 2: `MIVO_SERVER_PORT=8470` bound `[::]:8470`. Run 3: `--config-dir cfg2` (yaml port 8471) won over `MIVO_CONFIG_DIR`, bound `[::]:8471`, `config_dir=...\cfg2`. `mivo.exe --version` prints `Mivo v0.0.0-... (commit ..., go1.27.3)`, exit 0. Unit tests also cover the env-var path. |
| 5 | Startup errors: one clear stderr message, exit 1, no panic or stack trace | met (see observation) | Broken YAML: `mivo: load configuration: ...config.yaml: invalid YAML: yaml: line 1: did not find expected ',' or ']'`, exit 1, 1 stderr line. `MIVO_SERVER_PORT=eighty`: `mivo: load configuration: environment variable MIVO_SERVER_PORT: invalid configuration value: port "eighty" is not a number`, exit 1, 1 stderr line. Port in use (127.0.0.1:8480 held by a listener): final line `mivo: listen on 127.0.0.1:8480: listen tcp 127.0.0.1:8480: bind: Only one usage of each socket address ...`, exit 1. Data dir under a regular file: `mivo: prepare directories: create directory ...\blockfile\data: mkdir ...: The system cannot find the path specified.`, exit 1. stdout empty in all cases. |
| 6 | Ctrl+C / context cancel: graceful shutdown, DB closed, exit 0 | met | Console Ctrl+C event (`CTRL_C_EVENT`, runs 1 and 3) and `CTRL_BREAK_EVENT` (run 2) delivered to the child console: logs `http server shutting down`, `Mivo stopped`, exit code 0 each time. After stop, only `server.db` remained: `server.db-wal` and `server.db-shm` (present while running) are gone. |
| 7 | Smoke test in `cmd/mivo` (temp dirs, port 0, hook for address, both endpoints, cancel, exit 0, second run reuses server ID) | met | `TestRunServesSystemEndpointsAndKeepsServerIDAcrossRestarts` PASS under `-race`. Also `TestRunReportsStartupErrorsOnOneLine` (4 subtests), `TestRunExitsCleanlyWhenStoppedWhileStarting`. |
| 8 | `cmd/mivo` only wiring and flag parsing; every function <= 50 lines; no business logic | met | funlen, gocognit, mnd, revive enabled in `.golangci.yml` and `golangci-lint run` reports 0 issues. Longest function (`serveWith`) is about 33 lines; `run` has exactly 4 parameters. Reviewer check on business logic remains with `mivo-reviewer`. |
| 9 | Gates: golangci-lint, go test, archtest, coverage check | met | see Gates table |

M1-02 carry-over: `run` takes `(ctx, args, runEnv, runHooks)` (4 parameters, I/O and env bundled in `runEnv`). `main_test.go`: success cases assert empty stderr (`wantEmptyStderr`), the unknown-flag case asserts `flag provided but not defined: -unknown-flag`, the unexpected-argument message appears exactly once (`assertStderr` counts occurrences; smoke run confirmed a single `unexpected argument "extra"` line, exit 2). Met.

Orchestrator note (M1-04b): `paths.HostEnvironment(env.lookupEnv)` is used in `serve.go:48`; the sqlite wiring is `Open` -> `Migrate` -> `NewIdentityRepository`, the db is the `DatabaseChecker`. The `CreatedAt` truncation item applies only if `app/system` is touched; it was not touched. Met / not applicable.

## Smoke run details

Built binary: `go build -o <scratch>\mivo.exe ./cmd/mivo` (exit 0). Environment isolated: `LOCALAPPDATA`,
`USERPROFILE`, `APPDATA` and `MIVO_CONFIG_DIR` pointed into the scratch directory; no `MIVO_*` variables inherited.
Ports 8460, 8470, 8471, 8480, 8481 were free before the run. The server logs to stderr; stdout stays empty.
Other CLI results: `--bogus` exit 2 with reason and usage; extra positional argument exit 2; `-h` exit 0 with usage.

Cleanup: after the run `Get-Process mivo` lists 0 processes and no listener remains on the used ports.
Scratch files are under `...\scratchpad\m110smoke` (outside the repo).

## Observations (not failures)

1. Port-in-use and unwritable-data-dir failures happen after the logger exists, so stderr then holds the
   INFO log lines `starting Mivo` (and `server identity ready` for port in use) before the single
   `mivo: ...` error line. Config errors (YAML, env value) print exactly one stderr line. The task AC says
   "one clear message", and the test (`assertStartupError`) deliberately treats earlier lines as log records
   and requires exactly one `mivo: ` error line. I rated the AC met. If the orchestrator wants literally one
   stderr line in every error case, startup info logs would have to be suppressed or buffered until the
   server is up. Decision is the orchestrator's.
2. The working tree also shows `docs/tasks/M1-16-embed-web-ui.md` as modified. It is outside this task's
   changed files and was not part of the check.
3. Nothing was staged by this run.

## History

- Round 1: PASS.

REPORT
status: PASS
task: M1-10
files: docs/reports/M1-10-test.md
tests: go vet ./... exit 0; go test -race -count=1 ./... all ok; go-test-coverage total 97.1% (>=70%), domain >=90% satisfied; golangci-lint run 0 issues, fmt --diff clean; go mod tidy -diff clean (root, tools); govulncheck no vulnerabilities; archtest PASS; smoke run of built binary: health 200, system/info 200 with stable serverId across restarts, Ctrl+C and CTRL_BREAK exit 0 with no -wal/-shm leftovers, 4 startup-error cases exit 1 with one "mivo: ..." line, no mivo process left
findings:
- Observation: port-in-use and unwritable-data-dir print 1 to 2 INFO log lines to stderr before the single "mivo: ..." line (config errors print exactly one line). AC rated met; strict "exactly one stderr line" reading would need log buffering. Orchestrator decides.
- docs/tasks/M1-16-embed-web-ui.md is modified in the tree (not part of this task).
next: Hand over to mivo-reviewer; the orchestrator may decide on the stderr-lines observation.
