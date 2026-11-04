# M3-19a test report

Status: PASS (round 1 of max 3)

## Gates
- go vet ./... and go vet -tags webui: clean
- go test -race -coverprofile ./...: all ok; go-test-coverage: total 95.5 % (>= 70 %), package thresholds PASS
- golangci-lint run (also --build-tags webui): 0 issues; golangci-lint fmt --diff: clean
- go mod tidy -diff: clean
- Drift: go generate + pnpm -C web gen run twice, generated diff hash identical each time, no new changes
- pnpm -C web lint: clean; test: 91 files / 1298 tests passed; build: ok
- go test -race -tags webui ./internal/adapters/http/... ./cmd/...: ok
- pnpm -C web e2e: 31 passed

## Acceptance criteria
All met: spec 1.3.0 contains createLibrary, updateLibrary, deleteLibrary, listFolderRoots, listFolders, unavailableFolderCount, libraryKinds, importedFromConfig, the nine problem codes; spec_test.go covers operation ids, codes and additionalProperties/kind checks (go test ./api passes); restapi library_management_stub.go returns 501 not_implemented; generated output in sync.

## History
- Round 1: PASS
