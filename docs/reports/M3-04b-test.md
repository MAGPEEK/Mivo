# M3-04b test report

Status: PASS (round 1)

## Gates
| Gate | Result |
|---|---|
| go vet ./... | clean |
| go test -race -coverprofile=coverage.out ./... | all ok |
| go-test-coverage (.testcoverage.yml) | PASS, total 95.9 % (threshold 70 %) |
| domain/metadata coverage | 100.0 % (>= 90 %) |
| domain/artwork coverage | 100.0 % (>= 90 %) |
| golangci-lint run | 0 issues |
| golangci-lint fmt --diff | no diff |
| go mod tidy -diff, go -C tools mod tidy -diff | no diff |

## Layer rules
- artwork imports: stdlib (cmp, crypto/sha256, encoding/hex, fmt, path, slices, strconv, strings, time) + internal/domain.
- metadata imports: stdlib + internal/domain, domain/artwork (ArtworkField), domain/library (SortTitle).
- No database/sql, net/http, os/exec, adapters or app in `go list -deps`. artwork does not import metadata: no cycle.

## Acceptance criteria
- Record, Credit, Role, Date/ParseDate: met (record.go, date.go, tests 100 %).
- Provider, ValidateProviderID: met (provider.go).
- Field, ParseField, LockSet (NewLockSet, Has): met (field.go; includes artwork.* fields).
- Source (filename|nfo + reserved embedded|tmdb|musicbrainz|user, documented), Merge with table tests: met (source.go, merge.go).
- SortTitleFor: met.
- artwork.Kind, SourceKind, Ref: met.
- ClassifyLocal: met; returns a LocalMatch struct (kind, priority, season number) instead of a bare (Kind, int, ok) tuple. Equivalent content, needed for the seasonNN-poster number.
- Select: met (locked keeps current; local by priority, then embedded; path tie-break).
- Width, ParseWidth, OutputFormat, Tag, RenderVersion: met.
- Stored, Location, RenderRequest, Rendered: met (stored.go).
- Tests table-driven, coverage >= 90 %: met.
- Gates: met.

## History
- Round 1: PASS.
