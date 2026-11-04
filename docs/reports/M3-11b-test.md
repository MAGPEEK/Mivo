# M3-11b test report (round 1) - PASS

Gates (all green): go vet, go vet -tags webui, golangci-lint run (+ --build-tags webui) 0 issues,
golangci-lint fmt --diff clean, go mod tidy -diff (root + tools) clean, go test -race ./... (local),
Docker golang:1.27.3 go test -race ./... green, go test -race -tags webui http/... cmd/... green,
go-test-coverage: total 94.7 % (>= 70), go build -tags webui OK, docker build OK.
Corpus test cmd/mivo (TestRunBuildsTheLibraryHierarchyFromTheCorpus, 10.5 s) ran, not skipped.

Criteria: v2 scan deps, images wiring, scantrigger + shutdown-order test (TestStartBackgroundStopCancelsAndWaits),
ItemReader/ChildReader, M2 pipeline/port/CatalogWriter removal (no CatalogFile/ProbedFile in app/scan; M2 writer
kept test-only, documented in task; sqlc output unchanged), integration assertions (ETag, immutable, Dragon Ball 131,
rescan no-op, NFO touch) present and passing: met.

History: round 1 PASS.
