# M3-04 test report

Result: PASS (round 1)

## Gates
| Gate | Result |
|---|---|
| `go vet ./...` | exit 0 |
| `go test -race -count=1 -coverprofile=coverage.out ./...` | no FAIL, exit 0 |
| go-test-coverage (`.testcoverage.yml`) | PASS, total 95.8 % (>= 70) |
| `internal/domain/library` coverage | 100.0 % (>= 90; domain packages all 100 %) |
| `golangci-lint run` (incl. depguard) | 0 issues |
| `golangci-lint fmt --diff` | clean |
| `go mod tidy -diff`, `go -C tools mod tidy -diff` | clean |
| Layer rule: `go list -deps ./internal/domain/...` | no database/sql, net/http, os/exec, adapters, app, golang.org/x |
| Web, media, docker | not applicable to this task |

## Acceptance criteria
All met; evidence: new/changed files in `internal/domain/library` (item.go ValidParent/OwnsFiles,
extra.go, file_class.go, file_state.go PathForParsing, placement.go, key.go, title.go SortTitle)
with tests, 100 % package coverage, no `x/text` import, BACKLOG diff removes exactly the two
`domain/library` rows. Verified structurally (symbols present, eight ExtraType values, FileClass
video/nfo/image); behavioural detail relies on the green table-driven tests.

## History
- Round 1: PASS.
