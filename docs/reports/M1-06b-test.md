# M1-06b test report

Status: PASS (round 1)

Scope: `./internal/domain/...`, `./internal/app/...`, plus archtest. Web, media, Docker gates not applicable.

## Gates

| Gate | Command | Result |
|---|---|---|
| vet | `go vet ./internal/domain/... ./internal/app/...` | exit 0 |
| tests | `go test -race -count=3 -coverprofile=coverage.out ./internal/domain/... ./internal/app/...` | all ok |
| coverage | `go tool cover -func=coverage.out` | `domain/system` 100.0 %, `app/system` 100.0 %; no function below 100 % |
| lint | `golangci-lint run <scope>` | `0 issues.` |
| format | `golangci-lint fmt --diff <scope>` | no diff |
| layer rule | `go test ./internal/archtest/...` | ok |
| stress | `go test -race -count=100 -run "EnsureIdentity\|Health" ./internal/app/system/` | ok (3.66 s) |

Thresholds: domain >= 90 % (100 %), total >= 70 % (100 % in scope). Met.

## Acceptance criteria

1. Lock-free load, context-aware create serialisation: MET.
   `internal/app/system/identity.go:18-25` loads first and returns when found; `:27-43` takes a
   1-slot channel semaphore (`service.go:14,54,71`) with `select` on `ctx.Done()` and reloads after
   acquiring. Tests: `TestEnsureIdentityCreatesOneIdentityUnderConcurrentCalls` (8 callers, 1 save,
   `service_test.go:210`), `TestEnsureIdentityWaiterUsesIdentityCreatedMeanwhile`,
   `TestEnsureIdentityLoadsWithoutWaitingForCreator`,
   `TestEnsureIdentityWaiterReturnsContextErrorWhenCancelled` (asserts the waiter blocks, then
   `errors.Is(err, context.Canceled)`, zero identity, creator still saves exactly once;
   `identity_test.go:124`). All green under `-race -count=100`.
2. `ErrConflict` reload: MET. `identity.go:74-77,89-96` loads once more and returns the stored
   identity. Tests: `TestEnsureIdentityReturnsStoredIdentityAfterSaveConflict` (`identity_test.go:79`),
   failure case "save conflicts but no identity is stored" (`service_test.go:288`).
3. Nil and max UUID rejected: MET. `internal/domain/system/server_id.go:19-24,43-46`; table rows
   "nil uuid" and "max uuid" with `ErrInvalid` in `server_id_test.go:67-68`; neighbours
   (`...0001`, `...fffe`) accepted at `:22-23`.
4. Fixed Health detail, no raw error text: MET. `health.go:17-24` defines the two fixed strings;
   `:56-62` maps only by `errors.Is(err, context.DeadlineExceeded)`; no duration in the text;
   the design choice (adapter owns and logs the raw error) is documented in the `Health` comment
   (`:29-31`). Test `TestHealthDetailNeverContainsAdapterErrorText` (`health_test.go:47`) covers
   plain failure, wrapped deadline, wrapped cancellation; plus late-answer, blocking ping,
   cancelled caller and timeout-bound tests.
5. Empty `ProductName` rejected: MET. `service.go:94-98` (trimmed, wraps `domain.ErrInvalid`);
   table rows "empty product name" and "blank product name" in `service_test.go:97-98`.
6. No product-name literal: MET. Case-insensitive grep for `mivo` in `internal/app/system` and
   `internal/domain/system` matches only the Go module path in imports (`github.com/magpeek/mivo/...`)
   and the directory references `cmd/mivo` in doc comments (`service.go:16,39`, `ports.go:3`,
   `domain/system/info.go:9`); these are paths, not the product name.
7. Gates (100 % coverage, 0 lint issues, `-race -count=3`): MET, see table.

## Findings

None blocking. Observation only: the `cmd/mivo` doc-comment path references would need updating if the
binary directory is ever renamed; not a criterion violation.

## History

- Round 1: PASS.
