# M3-19l test report

Status: FAIL (round 1 of 3)

## Gates
- go vet ./... and -tags webui: OK
- go test -race -coverprofile ./...: all ok; go-test-coverage PASS, total 94.6 %
- golangci-lint run (default and --build-tags webui): FAIL, 3 issues
  - internal/app/library/manage_update.go:14 UpdateLibrary too long (51 > 50) (funlen)
  - internal/adapters/sqlite/library_store_v2_test.go:195 seedFile 6 args, max 4 (revive argument-limit)
  - internal/adapters/sqlite/library_store_v2_test.go:203 seedItem 6 args, max 4 (revive argument-limit)
- golangci-lint fmt --diff, go mod tidy -diff (root, tools): clean
- Docker sqlc generate: no diff against committed sqlcgen
- Docker Linux go test -race (cmd, http, sqlite, app/library, scan, scantrigger): ok
- Corpus tests (media + generated library corpus): corpustest and cmd ok, no SKIP
- docker build packaging/docker/Dockerfile: OK
- Release smoke vs image (testdata/media at /media): chromium-desktop 30/30, firefox-desktop 28/28 pass; container removed

## Acceptance criteria
- ImportConfigured replaces SyncLibraries: met (no SyncLibraries left; cmd/mivo/wire_management.go:165)
- Wiring (v2 store, ledger, browser, ScanControl, LibraryEvents, guard, kinds): met (cmd tests pass)
- Call order Suspend, Cancel, WaitIdle, store, event, Forget/start: met (manage_test.go:289,439,462; manage.go:187-200, manage_update.go)
- Docker defaults MIVO_BROWSE_ROOTS=/media, MIVO_INSTALL_KIND=docker: met (Dockerfile; README/compose changed)
- Smoke without editing CLAUDE.md: met via scripts/playback-smoke seeding through the API (library_seed.go)
- Integration test (cmd/mivo/library_management_test.go): met, 4 tests pass incl. import-once ledger
- UUID root ids: met (cmd/mivo/folder_roots.go UUIDv5)
- Manual checkpoint (curl + UI): not run by tester
- Gates incl. golangci-lint: NOT met (3 issues)
