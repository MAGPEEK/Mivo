# M1-06 test report

Status: PASS (round 1)

Scope: `./internal/domain/...`, `./internal/app/...`, `./internal/adapters/uuidgen/...`, plus archtest.
Go toolchain: go1.27.3 windows/amd64.

## Gates

| Gate | Command | Result |
|---|---|---|
| vet | `go vet ./internal/domain/... ./internal/app/... ./internal/adapters/uuidgen/...` | exit 0 |
| tests | `go test -race -count=3 -coverprofile=<scratch>/cov.out <scope>` | all ok |
| lint | `golangci-lint run <scope>` | `0 issues.` |
| format | `golangci-lint fmt --diff <scope>` | no diff |
| archtest | `go test -count=1 ./internal/archtest/...` | ok |
| full suite | `go test ./...` | all packages ok (platform/config now compiles, so the earlier M1-04 note no longer applies) |

## Coverage

```
internal/domain          coverage: [no statements]  (only sentinel var declarations)
internal/domain/system   100.0%
internal/app/system      100.0%
internal/adapters/uuidgen 100.0%
```

`go tool cover -func` lists no function below 100.0 %. The domain >= 90 % rule is met. The total
>= 70 % threshold was measured on the scope only, and the scope is at 100 %.

## Layer rule (independent check)

`go list -f '{{.Imports}}' ./internal/domain/...`:
- `internal/domain`: `errors`
- `internal/domain/system`: `fmt`, `internal/domain`, `slices`, `time`

No `database/sql`, `net/http`, `os/exec`, `internal/adapters` or `internal/app` imports.
`app/system` imports neither `platform/buildinfo` nor `platform/brand` (ADR 0003).

## Claim checks

- `uuid` is standard library: `go list -f '{{.ImportPath}} std={{.Standard}}' uuid` prints `uuid std=true`. The adapter imports `uuid`, so no third-party dependency was added for M1-06. The `go.mod` diff holds only `go.yaml.in/yaml/v3`, which belongs to M1-04.
- Health returns no error and lists all checks: `Health(ctx) system.Health` built via `NewHealth(checks...)` (service.go:142).
- Late ping counts as unavailable: service.go:150-155 re-checks `pingContext.Err()` after a nil ping result. `TestHealthTreatsLateAnswerAsUnavailable` covers it.
- EnsureIdentity is mutex-serialised: service.go:104. `TestEnsureIdentityCreatesOneIdentityUnderConcurrentCalls` runs 8 callers and passes under `-race -count=3` (one save).
- Info calls EnsureIdentity: service.go:175. `TestInfoReturnsPersistedServerIDWithoutPriorEnsure` covers the stored case and `TestInfoDescribesServer` covers the first-use case.
- No TODO/FIXME, `panic(` or `init()` in the scope.

## Acceptance criteria

| # | Criterion | Met | Evidence |
|---|---|---|---|
| 1 | Sentinels `ErrNotFound`, `ErrConflict`, `ErrInvalid`, `ErrForbidden` with doc comments | yes | errors.go:8-18; `TestSentinelErrorsAreDistinctAndSurviveWrapping` |
| 2 | `ServerID`, `ParseServerID` (canonical lowercase only, wraps `ErrInvalid`), `Identity`, `HealthStatus`/`Health`/`HealthCheck`, `Info` | yes | server_id.go:30-37; `TestParseServerIDAcceptsCanonicalLowercaseUUIDs` and `...RejectsNonCanonicalText`; health.go; info.go and identity.go compile and are used by tests |
| 3 | Ports `IdentityStore` (2 methods), `DatabaseChecker` (`Ping`), `IDGenerator` (`NewID() (string, error)`); `Service` built from a `Dependencies` struct (ports, clock, build info, product name) | yes | ports.go:13-32; service.go:30-43, 60; `TestNewServiceRejectsMissingDependencies` |
| 4 | `EnsureIdentity`: load, on `ErrNotFound` generate, validate, save; idempotent (saves once); other errors wrapped | yes | service.go:103-138; the `ReturnsStoredIdentity`, `CreatesIdentityOnFirstUse`, `IsIdempotent`, `Concurrent` and `Failures` tests (load, generate, uppercase id, save) |
| 5 | `Health`: ok on success; unavailable with `database` check and detail on error or timeout; 2 s named constant; deadline via context | yes | `DatabasePingTimeout = 2 * time.Second` (service.go:16); tests `OKWhenPingSucceeds`, `UnavailableWhenPingFails`, `BoundsPingWithTimeout`, `HonoursContextDeadlineOfBlockingPing`, `TreatsLateAnswerAsUnavailable`, `ReportsCancelledCaller` |
| 6 | `Info`: build values, product name, API version "1", persisted server ID | yes | service.go:174-188; `TestInfoDescribesServer` (APIVersion "1") |
| 7 | `uuidgen` implements `IDGenerator` with random canonical lowercase UUIDs | yes | uuidgen.go; compile-time assertion in uuidgen_test.go:13 |
| 8 | Tests: table-driven domain tests without mocks, domain >= 90 %; app tests with hand-written fakes covering every branch; uuidgen format and uniqueness test | yes | 100 % coverage; fakes in fakes_test.go (no mock framework); `TestNewIDReturnsUniqueCanonicalVersion4UUIDs` (16 calls, format, version and variant, uniqueness) |
| 9 | Gates: lint (depguard), `go test ./...`, archtest, coverage | yes | see Gates table |

## Findings

None blocking.

Observations (non-blocking):
- `TestHealthBoundsPingWithTimeout` allows 250 ms slack around the 2 s deadline. This is reasonable given Windows timer granularity, and it was stable over 3 race runs.
- The web gates and E2E do not apply to this task.

## History

- Round 1: PASS.
