# M6-01 test report (run 2, worktree uncommitted)

Status: PASS

| Gate | Result |
|---|---|
| go generate ./... and pnpm -C web gen | idempotent, no unintended diff |
| go vet ./... | clean |
| go test -race ./api ./internal/adapters/http/... | ok |
| go test -race ./... (coverage) | all ok |
| golangci-lint run / fmt --diff | 0 issues / clean |
| go-test-coverage | PASS, total 94.7 % (>= 70) |
| pnpm -C web lint / test / build | ok / ok (99.56 % stmts) / ok |

Acceptance criteria: identify/search, identify, DELETE match, item and library refresh (202),
matchState + metadataSources + matchState filter, settings/metadata + metadata/status,
404/409/503 mapping with retryAfterSeconds, ADR 0028 notes (openapi.yaml lines 421-554),
posterUrl description, generation clean, spec tests (api/spec_test.go) green, 501 stubs
(metadata_stub.go) tested: all met.

History: run 1 aborted; run 2 PASS.
