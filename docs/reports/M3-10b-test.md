# M3-10b test report (round 2: PASS)

| Gate | Result |
|---|---|
| go vet ./... | clean |
| go test -race -coverprofile ./... | all ok |
| go-test-coverage | total 95.3 % (>= 70), domain gate PASS |
| golangci-lint run | 0 issues |
| golangci-lint fmt --diff | clean |
| go mod tidy -diff | clean |
| Docker Linux race, internal/app/scan | ok |
| go vet -tags webui | not runnable: webui/dist not built in worktree (embed pattern all:dist); environmental, untagged vet clean |
| Layer rule (go list -deps domain) | no forbidden imports |

Acceptance: NFO rules, Merge/Save-on-change, artwork selection, never-failing NFO with metadataWarnings,
all listed scenarios (NFO wins, locks, season poster, loose movie, embedded cover, mtime refresh,
malformed, multi-episode, cancellation) have named tests in internal/app/scan/metadata*_test.go
and pass. Known limitations (multi-episode first record) are documented in the task file.

History: round 1 aborted; round 2 PASS.
