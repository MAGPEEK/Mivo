# M3-16b test report

Status: PASS (round 2)

| Gate | Result |
|---|---|
| go vet ./... | clean |
| go test -race -coverprofile ./... | all ok; total 94.6 % (go-test-coverage PASS, total >= 70 %) |
| domain layer imports | none of database/sql, net/http, os/exec, adapters, app |
| golangci-lint run / fmt --diff | 0 issues / no diff |
| go mod tidy -diff (root, tools) | clean |
| Docker Linux -race -count=3 libraryfs, app/scan | ok |
| Docker Linux -race ./cmd/... | ok |

Coverage: libraryfs/disc.go functions 82-100 % (reportDisc 83.3, findIndex 82.4); scan walkFolder 100 %.

Guard check (by reading): disc_guard_test.go seeds a stored vob, then scans a VIDEO_TS disc entry
and an ISO entry (both FileClassVideo with Layout set). Without the `Layout.IsDisc()` guard in
internal/app/scan/walk.go:68 both would be appended to `found.videos` and counted Added (>0, and
the stored vob under the disc path would be removed), so the `Added/Removed/Failed != 0` assertion fails.

Acceptance: all criteria met (disc folder/ISO entries, case variants, nested/extras, tests, gates).

History: round 1 aborted; round 2 PASS.
