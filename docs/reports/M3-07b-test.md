# M3-07b test report (round 1) - PASS

| Gate | Result |
|---|---|
| go vet ./... | clean |
| go test -race -coverprofile ./... | all ok |
| go-test-coverage | total 96.1% (>=70), thresholds PASS |
| golangci-lint run | 0 issues |
| golangci-lint fmt --diff | clean |
| go mod tidy -diff | clean (x/text promoted to direct) |
| Docker Linux race, adapters/fs + app/scan | ok (libraryfs, transcodedir, scan) |

Notes: --allow-parallel-runners is not a flag of `golangci-lint fmt`; run without it.
Coverage: libraryfs 89.4%, scan 97.9%. Domain untouched.

## Acceptance
- Walk Class/NormalizedPath: met (TestWalkReportsSidecarClassesAndNormalizedPaths, NFD case).
- ReadSidecar refusals (size, vanished, outside root, non-regular): met (TestReadSidecarRefusals, symlink + Windows junction tests).
- run.go single filter + test with poster.jpg/movie.nfo: met (TODO references M3-10).
- Tests incl. unix variants via Docker: met.
- Gates: met.

## History
Round 1: PASS.
