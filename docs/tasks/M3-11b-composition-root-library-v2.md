# M3-11b Composition root: wire library v2, images and scan triggers
Owner: mivo-backend
Depends on: M3-05b, M3-05c, M3-05d, M3-06b, M3-08, M3-09b, M3-10b, M3-11, M3-17
Files/areas: `cmd/mivo/**` (`wire_library.go`, `wire_http.go`, `serve.go`, new
`wire_images.go`, `library_corpus_test.go` + tests), deletions only in `internal/app/scan/**`
(M2 pipeline and port), `internal/adapters/sqlite/{catalog_writer*.go,queries/media.sql,
sqlcgen/**}` (M2 writer and its now unused queries), `internal/adapters/http/restapi/**` (only
removing the `not_implemented` stub path), `docs/tasks/BACKLOG.md` (only the `cmd/mivo` M2-32
row if touched)

## Goal
`go run -tags webui ./cmd/mivo` with a movies and a shows library builds the hierarchy from
names and NFO files, serves posters through the image endpoint, and keeps the libraries
current through the watcher and the periodic scan. This is the **walking-skeleton checkpoint**
of milestone 3. UX: LB-40, LB-44, DT-20 … DT-22, DT-24, AD-23.

## Acceptance criteria
- [ ] The scan service gets the v2 dependencies: walker/resolver/sidecar reader
      (`libraryfs`), prober, the sqlite catalog writer (both halves), the NFO parser, IDs,
      clock, logger, and a `Finished` hook that purges the image cache.
- [ ] The images service is built on the sqlite artwork source, `libraryfs` resolver, the
      FFmpeg renderer and `imagecache` at `<paths.cache>/images`; the image handler is mounted
      and its quiet pattern registered; the cache is purged once at startup (in the
      background, not blocking the listener).
- [ ] `scantrigger` runs with the config keys `scan.watch` and `scan.interval` after the
      startup scan has been started; it stops before the scan service closes at shutdown
      (shutdown order tested).
- [ ] The library service gets `sqlite`'s v2 reader as `ItemReader` and `ChildReader`.
- [ ] Clean-up of the dual path (M3-10, M3-06): the milestone-2 scan pipeline, the M2
      `scan.CatalogWriter` port with its fakes, the M2 `sqlite.CatalogWriter` and its
      unused queries are deleted, and the regenerated sqlc output is committed; the
      `not_implemented` path in `restapi` is removed when no operation uses it any more.
- [ ] Integration test (real ffprobe/FFmpeg and SQLite, skipped under `-short` or without the
      library corpus of M3-17): scan `testdata/library/movies` and `…/shows` as two libraries
      and assert the manifest's expectations – folder movie with two versions listed once,
      trailer as extra, NFO title used, poster rendered through the HTTP image endpoint at
      width 320 (JPEG, ≤ 320 px wide, ETag, immutable with tag), shows with seasons in order
      and specials last, **"Dragon Ball Super": 1 season, 131 episodes (DT-24)**, a rescan
      changes nothing, touching an NFO refreshes only its item.
- [ ] Manual checkpoint documented in the REPORT: commands, the library config used, and that
      the web UI lists posters (after M3-12) – or "API only" if M3-12 has not landed.
- [ ] REPORT lists the release-note lines (one-time catalog rebuild and re-probe, new config
      keys, new dependency).
- [ ] Gates: `go vet ./...` and `go vet -tags webui ./...`, `golangci-lint run` (+ `--build-tags
      webui`), `golangci-lint fmt --diff`, `go test -race ./...`, `go test -race -tags webui
      ./internal/adapters/http/... ./cmd/...`, coverage gate, `go build -tags webui -o
      bin/mivo.exe ./cmd/mivo`.

## Implementation notes (decisions)
- Wiring lives in `cmd/mivo/wire_images.go` (image cache, FFmpeg renderer, handler, background
  purge with a stop function), `wire_scantrigger.go` (trigger on `scan.watch`/`scan.interval`,
  `startBackground` stops it before the scan service closes), `wire_metadata.go` (adapter from
  `metadata/nfo` to `scan.NFOParser`) and `wire_library.go` (v2 scan service, `Children` reader).
- Dead M2 code: the M2 scan pipeline, `scan.CatalogWriter`, `CatalogFile`, `ProbedFile` and
  their fakes are deleted. `run_test.go` went with them; the behaviour every scan shares
  (probe errors, resolve errors, no absolute path in logs, probe concurrency, finish log) moved
  to `run_shared_test.go` on the v2 harness.
- The M2 `sqlite.CatalogWriter` is no longer production code: it moved to the test-only
  `catalog_seed_test.go` because the reader tests seed the one-item-per-file shape through it
  (rewriting them onto `LibraryCatalog` is not worth the churn). Its sqlc queries stay for that
  reason, so sqlc output is unchanged. `catalog_writer_test.go` (tests of the seeding helper)
  is deleted; `isUniqueViolation` and `removeBatchSize` moved to `constraint.go`.
- `corpustest` (outside the listed areas) scans through `LibraryCatalog` now, because the M2
  writer no longer exists in production code.
- The `not_implemented` stubs in `restapi` stay: the library management operations of lane L
  (M3-19f/i/j/k/l) still use them. `app/library` keeps `Children` optional for the same reason
  (lane L owns that package).
- Rename left for later: `scan.CatalogV2` / `Dependencies.CatalogV2` could drop the suffix now
  that it is the only catalog port.

## Notes
- Keep `cmd/mivo` free of logic: conversion of config values and wiring only (CLAUDE.md).
- The renderer and the prober share the configured FFmpeg/ffprobe paths of M2 (`tools.*`).
- Update `docs/development.md` only if a command changes (the new corpus flag comes from
  M3-17).
