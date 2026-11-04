# M3-09 test report

Round 1 – status: PASS

## Gates
| Gate | Result |
|---|---|
| `go vet ./...` | clean |
| `go test -race -coverprofile=coverage.out ./...` | all packages ok |
| `go-test-coverage --config=.testcoverage.yml` | Package (0%) PASS, Total (70%) PASS, total 96.0% (7439/7752) |
| `golangci-lint run` | 0 issues |
| `golangci-lint fmt --diff` | empty diff |
| `go mod tidy -diff` | exit 0 |
| `-race -count=5` on both packages | ok (images 2.1s, imagecache 12.0s), no flakiness |
| Docker Linux `golang:1.27.3 go test -race -count=3` on both packages | ok |

Package coverage: `internal/app/images` 97.4%, `internal/adapters/fs/imagecache` 86.0%
(below 100% only in OS-error branches: createTemp 54.5%, writeAndClose 50.0%, Store 70.6%,
Open 76.9%). `internal/domain/...` untouched, unchanged at 100%/no statements.

## Layer rules
- `go list -deps ./internal/app/images`: no `net/http`, `database/sql`, `os/exec`, or adapters.
- `go list -deps ./internal/domain/...`: no forbidden imports.
- archtest passes. Only `internal/app/images/` and `internal/adapters/fs/imagecache/` are new
  (git status).

## Acceptance criteria
- Ports in `app/images` (source, resolver, renderer, cache): met (`ports.go`; Service uses
  `Artwork`, `ArtworkTags`, `Resolve`, `Render`, `Open`/`Store`/`Purge`).
- `Service.Image`: met – unknown ID error wrapped from source, cache hit returns early without
  render, miss resolves then renders with `MaxWidth = key.Width` and `artwork.OutputFormat(kind)`,
  stores, serves; `Immutable: request.Tag == stored.Tag` (service.go).
- 2 concurrent renders (`maxConcurrentRenders = 2`), per-key sharing and ctx wait (`flight.go`,
  tests with fake renderer pass under -race -count=5): met.
- Renderer errors wrapped with `%w` (classification preserved); a failed render returns before
  `Store`: met.
- `Service.Purge` keeps only current (id, tag) pairs: met.
- `imagecache` layout, validation, temp+rename, `ErrNotFound`, purge of stale files/dirs,
  foreign files ignored, Windows retry (`retry.go`, `lockerror_windows.go`): met; tests pass on
  Windows and Linux.
- Tests and coverage thresholds: met.
- Gates: met (see table).
- No cache size cap: correct per ADR 0027 section 8 (not flagged).

## Failures
none

## History
- Round 1: PASS.
