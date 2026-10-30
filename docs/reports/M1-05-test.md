# M1-05 Structured logging – test report

Status: PASS (round 1)
Scope: `./internal/platform/logging/...` plus `./internal/archtest/...`. Repo-wide gates were not run
because of parallel unfinished work (M1-04, M1-06, M1-18, M1-14).

## Gates

| Gate | Command | Result |
|---|---|---|
| vet | `go vet ./internal/platform/logging/...` | exit 0 |
| test | `go test -race -count=1 -cover -coverprofile=<scratch>/cover.out ./internal/platform/logging/...` | ok, 100.0 % of statements |
| per function | `go tool cover -func` | New, parseLevel, redactSensitive, anySensitiveKey, isSensitiveKey all 100.0 % |
| lint | `golangci-lint run ./internal/platform/logging/...` | 0 issues |
| format | `golangci-lint fmt --diff ./internal/platform/logging/...` | no diff |
| coverage gate | `go tool -modfile=tools/go.mod go-test-coverage --config=.testcoverage.yml --profile=<scratch>/cover.out` | Total threshold 70 % PASS (100 %, 26/26) |
| layer test | `go test -count=1 ./internal/archtest/...` | ok |

The package imports only the standard library (`errors`, `fmt`, `io`, `log/slog`, `strings`). It does not
import `platform/config`.

## Acceptance criteria

1. `logging.New(Options{Level, Format, Writer})` returns `*slog.Logger` or an error for an unknown level
   or format: MET. `logging.go:55`. `TestNewRejectsInvalidOptions` covers unknown level, empty level,
   `info+2`, unknown format and a nil writer (extra `ErrNoWriter`). The errors are matched with
   `errors.Is`. `TestNewErrorNamesTheInvalidValue` checks that the error text quotes the bad value.
2. Records below the level are dropped, tested for each level: MET.
   `TestNewDropsRecordsBelowTheConfiguredLevel` covers debug, info, warn, error and `WARN` (case-insensitive).
3. JSON output is valid per line with `time`, `level` and `msg`: MET.
   `TestNewJSONWritesOneValidObjectPerLine` decodes each line and checks the three keys and the level values.
4. Redaction of `password`, `token`, `secret`, `apiKey`, `authorization`, case-insensitive and nested in
   groups: MET. `TestNewRedactsSensitiveAttributes` has 16 cases:
   - all five keys, plus upper, mixed and header casing;
   - a non-string value;
   - one and two group levels, `WithGroup`, `With`, and a `LogValuer`;
   - a sensitive group name, where every leaf below it is redacted.

   `TestNewKeepsNonSensitiveAttributes` confirms that `tokenCount` and `passwordHint` stay visible.
   `TestNewRedactsInTextFormat` covers the text handler.

   Independent probe, built from a scratch file via `go run -overlay`, so nothing was written to the repo:
   ```
   {"msg":"login","request":{"auth":{"name":"alice","Password":"[REDACTED]"}},"Secret":{"anything":"[REDACTED]"},"apiKey":"[REDACTED]"}
   {"msg":"x","Authorization":"[REDACTED]","g":{"TOKEN":"[REDACTED]","keep":"ok"}}
   request.auth.Password=[REDACTED] Secret.anything=[REDACTED] apiKey=[REDACTED]   (text)
   ```
   The value `hunter2` appears nowhere in the output.
5. No `slog.SetDefault` and no package-level logger: MET. A grep of the non-test sources for
   `SetDefault|slog.Default|^var |func init` finds only the `var (...)` block of sentinel errors at
   `logging.go:30` (`ErrUnknownLevel`, `ErrUnknownFormat`, `ErrNoWriter`). No logger or `init()`.
6. Table-driven tests for level, format and redaction, package coverage ≥ 85 %: MET. All three areas use
   table-driven tests. Coverage is 100 %.
7. Gates (`golangci-lint run`, `go test`, coverage check): MET for the scoped package. See the gates table.

## Observations (non-blocking)

- `ReplaceAttr` is not called for group attributes. The implementation handles this by redacting every
  leaf under a sensitive-named group, and the probe confirms it.
- The redaction key list is a documented constant set (`redact.go:11-21`), as the task notes require.
- The `.testcoverage.yml` per-package threshold shows "0%". Only the total (70 %) and the domain override
  (90 %) apply. This is existing config and not part of M1-05.
- Repo-wide `go test ./...` and `golangci-lint run` were not evaluated. Failures from M1-04, M1-06,
  M1-18 and M1-14 are out of scope for this report.

## History

- Round 1: PASS.

REPORT
status: PASS
task: M1-05
files: docs/reports/M1-05-test.md
tests: go vet, go test -race -count=1 -cover ./internal/platform/logging/... (100.0 %), golangci-lint run (0 issues), golangci-lint fmt --diff (clean), go-test-coverage (PASS), go test ./internal/archtest/... (ok), redaction probe via nested groups (redacted)
findings: none
next: hand over to mivo-reviewer. The repo-wide gate should be re-run once M1-04, M1-06, M1-18 and M1-14 have landed.
