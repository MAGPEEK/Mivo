# M3-06b test report (round 1) - FAIL

| Gate | Result |
|---|---|
| go vet ./... | pass |
| go test -race -coverprofile ./... | pass |
| go-test-coverage | pass, total 95.7 % (domain >= 90 % ok) |
| golangci-lint run / fmt --diff | 0 issues / clean |
| go mod tidy -diff (root, tools) | clean |
| go generate ./... | no drift (api/ untouched) |
| pnpm gen / web test | skipped: api-client untouched, no node_modules |
| webui-tagged gates | skipped: webui/dist not built (embed pattern fails), no web build available |

## Acceptance criteria
- listItemChildren implemented, validation, 404: met (items.go, items_children_test.go; tests green)
- fields mapped / omitted when absent: met (items_detail_response.go + table tests)
- no absolute path/root in responses: met (test passes)
- BACKLOG row done: NOT MET - the row was deleted, but the spec note for `\` in `MediaFile.fileName` was not written; it was moved to a new `openapi` BACKLOG row (BACKLOG.md:33). Empty `?cursor=` rejection is met (items.go:129).
- table + handler tests: met
- gates: met

## Failures
- api/openapi.yaml: `MediaFile.fileName` lacks the note that `\` counts as a directory separator (AC 4). Fix: add note, bump spec patch version, `go generate ./...` + `pnpm -C web gen`, remove the new BACKLOG openapi row part; or have the architect amend the AC.

## History
- Round 1: FAIL (one AC)
