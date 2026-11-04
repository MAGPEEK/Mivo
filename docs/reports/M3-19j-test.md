# M3-19j test report

Result: PASS (round 2; round 1 aborted)

| Gate | Result |
|---|---|
| go vet ./... | clean |
| go test -race -coverprofile ./... | all ok |
| go-test-coverage | PASS, total 94.6 % (threshold 70 %), package thresholds satisfied |
| golangci-lint run | 0 issues |
| golangci-lint fmt --diff | no diff |
| go mod tidy -diff (root, tools) | no diff |
| Docker Linux go test -race -count=10 scantrigger, scan | ok |
| Docker Linux go test -race ./cmd/... | ok |

Layer rule: lint (depguard) clean. Web/E2E/media/Docker build not applicable.

Acceptance criteria: all met (events port, add/change/remove, once-per-folder failure log, no goroutine leak, fake-clock tests, gates) - covered by green scantrigger tests incl. count=10 under race.

History: round 1 aborted; round 2 PASS.
