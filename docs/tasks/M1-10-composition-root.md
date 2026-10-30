# M1-10 Composition root: wire config, logging, SQLite, use cases and HTTP
Owner: mivo-backend
Depends on: M1-04, M1-05, M1-08, M1-09
Files/areas: `cmd/mivo/**`

## Goal
`go run ./cmd/mivo` starts a working server: config is loaded, directories are created, the
database is migrated, the server identity is ensured, and the health and system info
endpoints answer. Ctrl+C shuts it down cleanly. This is the backend end of the walking
skeleton.

## Acceptance criteria
- [ ] With no config file: `go run ./cmd/mivo` creates the OS default directories
      (ADR 0018), creates `<data>/server.db`, logs one "listening" line with address and
      version, and serves on port 8460.
- [ ] `Invoke-RestMethod http://localhost:8460/api/v1/health` returns `status = ok`.
- [ ] `Invoke-RestMethod http://localhost:8460/api/v1/system/info` returns `productName =
      Mivo`, the build version and a `serverId`. After a restart with the same data
      directory, `serverId` is the same.
- [ ] `--config-dir <dir>` and `MIVO_CONFIG_DIR` select the config directory. `MIVO_*`
      overrides from ADR 0018 apply (e.g. `MIVO_SERVER_PORT=8470`). `--version` still works.
- [ ] Startup errors (invalid YAML, invalid env value, port already in use, unwritable data
      directory) print one clear message to stderr and exit with code 1. No panic, no stack
      trace.
- [ ] Ctrl+C (or context cancel) triggers graceful shutdown: server stops, DB is closed, exit
      code 0.
- [ ] Smoke test in `cmd/mivo`: `run(...)` starts with a temp config directory, a temp
      data directory and port 0. A hook reports the bound address. The test calls both
      endpoints, cancels and asserts exit code 0. A second run reuses the server ID.
- [ ] `cmd/mivo` contains only wiring and flag parsing. Every function ≤ 50 lines. No
      business logic (reviewer check).
- [ ] Gates: `golangci-lint run`, `go test ./...`, archtest and the coverage check pass.

## Notes
- Shape: `main` → `run(ctx, args, env lookup, stdout, stderr, hooks) int`. Use
  `signal.NotifyContext` for SIGINT/SIGTERM in `main` only.
- Map `platform/buildinfo` and `platform/brand` values into `app/system`'s own `BuildInfo`
  and product name (ADR 0003).
- Order: config → logger → `paths.Ensure` → `sqlite.Open` → `Migrate` → `EnsureIdentity` →
  HTTP server. On failure, close what was opened (in reverse order).
- Leave the HTTP fallback handler nil. M1-16 plugs in the web UI.
- `CLAUDE.md` says `go run ./cmd/mivo` starts the server. That must hold after this task.

## Review carry-over from M1-02
- `run(args, stdout, stderr)` already has 3 parameters. Adding `ctx` and an env lookup must not
  exceed the 4-parameter limit: bundle I/O and environment into a small struct
  (e.g. `runEnv{stdout, stderr io.Writer; lookupEnv func(string) (string, bool)}`).
- Tighten `cmd/mivo/main_test.go`: success cases assert empty stderr; the unknown-flag case
  asserts the rejection reason; format the "unexpected argument" error only once.
