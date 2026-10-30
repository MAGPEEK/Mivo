# M1-09 HTTP adapter: server, middleware, system handlers, problem errors
Owner: mivo-backend
Depends on: M1-06, M1-07
Files/areas: `internal/adapters/http/restapi/**`, `internal/adapters/http/server/**`

## Goal
`GET /api/v1/health` and `GET /api/v1/system/info` are served over `net/http` according to
the spec, with consistent problem-details errors, request logging, panic recovery and
graceful shutdown.

## Acceptance criteria
- [ ] `restapi` implements `apigen.StrictServerInterface`. It consumes a small interface it
      declares itself (`Health(ctx)`, `Info(ctx)`) that `app/system.Service` satisfies.
- [ ] `GET /api/v1/health` → `200` with `{"status":"ok", ...}` when healthy and `503` with
      `status: unavailable` otherwise, `Content-Type: application/json`.
- [ ] `GET /api/v1/system/info` → `200` with every `SystemInfo` field. `serverId` is the
      service's ID.
- [ ] Error mapping (table-tested): `ErrNotFound`→404, `ErrForbidden`→403, `ErrConflict`→409,
      `ErrInvalid`→400, anything else→500. The body is `application/problem+json` with
      `code`. A 500 never exposes the internal error text (it is logged instead).
- [ ] Unknown route under `/api/` → 404 problem. Wrong method on a known route → 405 problem.
- [ ] Middleware (each tested):
      - request ID: `X-Request-ID` reused if present, else generated, and echoed
      - access log via the injected `*slog.Logger` with method, path, status, duration and
        request ID
      - recover: a panicking handler yields a 500 problem and an error log with stack
      - security headers: `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`
- [ ] `server.New(Options{...})` takes the address or a listener, logger, API handler and an
      optional fallback handler for non-API paths (used by M1-16; 404 when nil).
      `Run(ctx) error` serves until ctx is cancelled, then shuts down within a named timeout
      constant (10 s). `ReadHeaderTimeout` is set. `WriteTimeout` stays 0 with a comment
      (future streaming).
- [ ] A test starts the server on `127.0.0.1:0`, calls both endpoints, cancels the context
      and asserts `Run` returns nil.
- [ ] Tests: `httptest`-based handler and middleware tests with an in-memory fake service.
- [ ] Gates: `golangci-lint run`, `go test ./...`, archtest and the coverage check pass.

## Notes
- Do not name a package `http` (it would shadow `net/http`). Use the subpackages listed
  above.
- Route registration uses the generated std-http handler on a `http.ServeMux`. The
  `/api/` catch-all for problem 404s must not shadow generated routes. Test both.
- The fallback handler hook is the only extension point M1-16 needs, so it should not have
  to edit routing logic beyond passing the handler.
- No CORS yet (same origin; webOS in milestone 10).

## Orchestrator notes (from M1-06 review)
- `/api/v1/health` is unauthenticated: never expose raw adapter error text. M1-06b makes the
  service return fixed detail strings; keep it that way in the HTTP mapping.
- Add a test that ties `system.APIVersion` to the `/api/v<version>` prefix in `api/openapi.yaml`.
