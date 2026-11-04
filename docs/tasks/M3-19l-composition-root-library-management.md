# M3-19l Composition root: wire library management, Docker defaults, integration test
Owner: mivo-backend
Depends on: M3-19c, M3-19d, M3-19g, M3-19h, M3-19i, M3-19j, M3-19k
Files/areas: `cmd/mivo/**` (`wire_library.go`, `wire_http.go`, `serve.go`, a new
`library_management_test.go`), deletions of the milestone-2 sync path in
`internal/app/library/**` and `internal/adapters/sqlite/**` (old `SyncLibraries` port,
implementation, queries, fakes), `packaging/docker/{Dockerfile,README.md,docker-compose.yml}`,
`scripts/playback-smoke/**` (only if the smoke needs a library without the UI),
`docs/development.md` (only commands that change)

## Goal
`go run -tags webui ./cmd/mivo` starts with the libraries of the database, imports configured
libraries once, and serves library management and the folder browser; Docker starts empty with
`/media` as the browse root (OD-M3-7 A). Checkpoint of the M3-19 lane.

## Acceptance criteria
- [ ] Startup: `ImportConfigured` replaces `SyncLibraries` (before the listener binds, as today);
      the old sync port, its sqlite implementation, queries and fakes are deleted; the
      generated sqlc output is committed.
- [ ] The library service gets the v2 store, the import ledger, the folder browser
      (`browse.roots`, reserved = config/data/cache/transcode directories), the scan service as
      `ScanControl`, `scantrigger` as `LibraryEvents`; the HTTP server gets the management
      handlers, the guard and `libraryKinds` (`movies`, `shows`); `install.kind` and the
      service user reach `listFolderRoots`.
- [ ] Docker image (OD-M3-7 A; if the owner chose B, only add the two new variables):
      `MIVO_LIBRARIES=/media` is replaced by `MIVO_BROWSE_ROOTS=/media` and
      `MIVO_INSTALL_KIND=docker`; `packaging/docker/README.md` and `docker-compose.yml` say: mount
      media below `/media`, add libraries in Settings › Libraries; `MIVO_LIBRARIES` still works
      as a one-time seed; upgrade note for 0.1.0-alpha.2 users ("add your library once in
      Settings › Libraries"; the catalog is rebuilt anyway, ADR 0027).
- [ ] The release smoke of `CLAUDE.md` keeps working without editing `CLAUDE.md`: either
      `scripts/playback-smoke` creates the library through `POST /api/v1/libraries` (browse root
      `/media`) when the server has none, or the smoke documentation passes
      `-e MIVO_LIBRARIES=/media`; prefer the former (it exercises the new API). Report the
      choice; `CLAUDE.md` changes need the owner's approval and are only proposed in the REPORT.
- [ ] Integration test (real SQLite, temp dirs, real file system; ffprobe-dependent parts
      skipped under `-short` or without the corpus): server with zero libraries → list empty;
      folder roots listed; listing a root hides a `@eaDir`, a `.hidden` and the data directory
      placed inside the root; traversal (`path=../x`) → 400; create a movies library with two
      folders → 201, scan runs, items appear from both folders; edit (remove one folder) →
      its items gone after the scan; delete → 404 afterwards; a non-local `RemoteAddr` and a
      foreign `Origin` → 403; restart with a configured library → imported once, removed in
      the app → not imported again on the next restart.
- [ ] Manual checkpoint in the REPORT: `go run -tags webui ./cmd/mivo` with an empty config,
      add a library through the API (`curl` commands listed), library visible in the UI.
- [ ] Gates: `go vet ./...` and `go vet -tags webui ./...`, `golangci-lint run` (+
      `--build-tags webui`), `golangci-lint fmt --diff`, `go test -race ./...`,
      `go test -race -tags webui ./internal/adapters/http/... ./cmd/...`, coverage gate,
      `go build -tags webui -o bin/mivo.exe ./cmd/mivo`, `docker build -f
      packaging/docker/Dockerfile -t mivo:dev .` [docker][net] and the release smoke against it.

## Notes
- `cmd/mivo` converts config values and wires only (CLAUDE.md); the service user lookup lives in
  the folder browser adapter (M3-19d).
- Release-note lines for the REPORT: libraries are managed in Settings › Libraries; configured
  libraries are imported once; Docker starts empty unless `MIVO_LIBRARIES` is set; management
  works only from the local network until accounts exist.
