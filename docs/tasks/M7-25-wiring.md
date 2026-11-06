# M7-25 Wiring in `cmd/mivo`
Owner: mivo-backend
Depends on: M7-07, M7-09, M7-10, M7-21, M7-22, M7-23, M7-27, M7-28 (and transitively all M7 backend tasks)
Files/areas: `cmd/mivo/wire_*.go` (new `wire_auth.go`, `wire_users.go`, `wire_watch.go`), `cmd/mivo/serve.go`, `cmd/mivo/*_test.go`

## Goal
A running server requires sign-in: fresh data directories start in setup state, existing alpha
catalogs are kept, and every M7 service is connected.

## Acceptance criteria
- [ ] Wiring constructs hasher, token source, account/session/watch stores, `app/auth`,
      `app/users`, `app/watch`, the scoped library source (replacing `rows.AllLibraries`), the
      session middleware and the extended manage guard; the cache-eviction and revoke ports of
      `app/users` are wired to `app/auth`.
- [ ] Startup runs `BackfillAgeRatings` once and starts a periodic `PurgeExpired` (hourly,
      stopped on shutdown with the server context).
- [ ] `cmd/mivo` tests: a fresh data dir answers `GET /api/v1/setup` `{required:true}` and
      `401 setup_required` on `/api/v1/libraries`; after setup the cookie session lists
      libraries; a migrated alpha database keeps its libraries and items.
- [ ] All gates incl. `go test -race -tags webui ./internal/adapters/http/... ./cmd/...`,
      the corpus tests and `pnpm -C web e2e` (with M7-28's harness) are green; the playback
      smoke against the Docker image passes with M7-27's sign-in.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
This is the switch that turns sign-in on; merge only when M7-27 and M7-28 are on `main`.
`cmd/mivo` corpus tests that call the HTTP API need a test helper that performs setup.
