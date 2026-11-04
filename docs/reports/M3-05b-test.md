# M3-05b test report

Status: PASS (round 1)

| Gate | Result |
|---|---|
| go vet ./... | clean |
| go test -race -coverprofile ./... | all ok |
| go-test-coverage | total 95.1 % (>=70), thresholds PASS |
| golangci-lint run / fmt --diff | 0 issues / clean |
| go mod tidy -diff (root, tools) | clean |
| sqlc generate (Docker) | no diff on tracked files; catalog_placement.sql.go matches generator output |
| Docker Linux race internal/adapters/sqlite/... | ok (21.6s) |
| go vet -tags webui | SKIPPED: web/dist absent in worktree (embed pattern), unrelated to the change |

Acceptance criteria: all met; evidence is the green sqlite integration suite (rescan IDs, versions, rename, move, 131 episodes, contract errors, WAL concurrency) plus lint/coverage gates. I did not read the individual test names.

History: round 1 PASS.
