# M3-06 test report

Status: PASS (round 1)

## Gates
| Gate | Result |
|---|---|
| `go vet ./...` | clean |
| `go test -race "-coverprofile=coverage.out" ./...` | all packages ok, no FAIL |
| `go-test-coverage --config=.testcoverage.yml` | package threshold PASS, total threshold (70%) PASS, total 96.1% (7239/7530) |
| `internal/app/library` coverage | 100.0% |
| `golangci-lint run` | 0 issues |
| `golangci-lint fmt --diff` / `gofmt -l internal/app/library` | no diff |
| `go mod tidy -diff` | exit 0 |
| web / docker / media | not applicable (backend-only task) |

## Layer rules
- `go list -deps ./internal/domain/...`: no `database/sql`, `net/http`, `os/exec`, `internal/adapters`, `internal/app`.
- `go list -deps ./internal/app/...`: no `internal/adapters`.

## Acceptance criteria
- ItemSummary v2: met. Poster/Thumb `*artwork.Ref`, ChildCount, LeafCount, PrimaryFile optional (`HasPrimaryFile`), year via `Item.Year` (`items.go:23`).
- ItemDetail v2: met. Metadata, Ancestors (`ItemRef{ID,Kind,Title,Number}`), PreviousID/NextID, PlayItemID, Artwork by kind, Extras, ItemFiles with VersionLabel/PartNumber (`items.go:80`). `TestGetItemPassesTheV2DetailOn`.
- ItemReader keeps ListItems/GetItem; separate `ChildReader` port with optional `Dependencies.Children`; error and ordering contract documented: met (`ports.go`, `service.go`).
- `Service.ListChildren`: met. Normalises limit (`children.go:30`), passes ErrNotFound from reader, empty page for leaf, `ErrUnavailable` without reader (`children.go:26`). Tests: `TestListChildrenFailures` (unknown item, no reader), `TestListChildrenOfALeafOrAnEmptyContainerIsEmpty`, `TestListChildrenRejectsALimitOutOfRangeWithoutReading`.
- M2 behaviour for movies preserved: met (existing service tests green, additive fields only, sqlite adapter compiles unchanged).
- Fake-based tests: met. `memoryChildReader` in `fakes_test.go` with doc comment on the mirrored adapter; `TestListChildrenOrdersByTheContract` (specials last, unnumbered last, descending, extras excluded), cursor tests (tampered, past end, other direction), stored-row fault as plain error.
- Gates: met (see table).

## History
- Round 1: PASS.

REPORT
status: PASS
task: M3-06
files: docs/reports/M3-06-test.md
tests: go vet clean; go test -race ./... all ok; coverage total 96.1%, internal/app/library 100%; golangci-lint 0 issues; fmt no diff; go mod tidy -diff clean
findings: none
next: hand over to mivo-reviewer
