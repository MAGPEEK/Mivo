# M1-09 test report: HTTP adapter

Status: **PASS**  (round 2; the round-1 sections are kept, round 2 is at the end)

Scope: `./internal/adapters/http/...`, `./api/...`, `./internal/archtest/...`. M1-08 (sqlite) and
M1-19 (scripts) are mid-change by other agents and were not gated here.

## Gates

| Gate | Command | Result |
|---|---|---|
| vet | `go vet ./internal/adapters/http/... ./api/...` | exit 0 |
| tests | `go test -race -count=3 -cover ./internal/adapters/http/... ./api/... ./internal/archtest/...` | all `ok`, exit 0 |
| lint | `golangci-lint run ./internal/adapters/http/... ./api/...` | `0 issues.` |
| format | `golangci-lint fmt --diff ./internal/adapters/http/... ./api/...` | no diff, exit 0 |
| layer rule | `go list -deps ./internal/domain/...` filtered for `database/sql\|net/http\|os/exec\|internal/adapters\|internal/app` | no matches |
| archtest | `go test ./internal/archtest/...` | ok, 95.4 % |

### Coverage

| Package | Coverage |
|---|---|
| `adapters/http/requestid` | 100.0 % |
| `adapters/http/restapi` | 98.1 % |
| `adapters/http/server` | 96.7 % |
| `adapters/http/apigen` (generated) | 0.0 %, excluded |
| Scoped profile (restapi + server + requestid, apigen removed) | **97.7 %** (threshold 70 %) |

The domain threshold (>= 90 %) is not affected by this task. No `internal/domain` file changed.

## Acceptance criteria

1. `restapi` implements `apigen.StrictServerInterface` and consumes its own `SystemService`
   interface (`Health(ctx)`, `Info(ctx)`): **met**.
   - `api.go:34` has `var _ apigen.StrictServerInterface = (*API)(nil)`.
   - `api.go:19` declares `SystemService`.
   - The probe compiled `var _ restapi.SystemService = (*appsystem.Service)(nil)`, so `app/system.Service`
     satisfies it.
2. `/health` returns 200 `ok` when healthy and 503 `unavailable` otherwise, with `application/json`: **met**.
   - Probe: 200 `{"checks":[{"name":"database","status":"ok"}],"status":"ok"}`, `CT="application/json"`.
   - Probe: failing ping gives 503 with `status:"unavailable"`.
   - `TestGetHealth` covers the cases.
3. `/system/info` returns 200 with every `SystemInfo` field, `serverId` taken from the service: **met**.
   - Probe body: `{"apiVersion":"1","buildTime":"2026-10-29T12:30:00Z","commit":"abc","productName":"Mivo","serverId":"0b8c6d4e-...","version":"1.0.0"}`.
4. Error mapping, table-tested, `application/problem+json` with `code`, no internal text on 500: **met**.
   - `TestProblemForError` covers 404/403/409/400/500, wrapped, joined, `context.Canceled` and nil.
   - `TestResponseErrorHandlerMapsErrorsWithoutErrorText` checks the body has no internal text.
   - Probe: `/system/info` with the error `read /secret/path/db.sqlite: %w ErrNotFound` gives 500
     `{"code":"internal_error","status":500,"title":"Internal Server Error","type":"about:blank"}`.
     The path appears only in the server log (`level=ERROR msg="api operation failed"`).
5. Unknown `/api/` route gives a 404 problem, wrong method gives a 405 problem: **met**.
   - Probe `GET /api/v1/nope`: 404 `problem+json`, code `not_found`.
   - Probe `POST /api/v1/health`: 405 `problem+json`, code `method_not_allowed`, `Allow: GET, HEAD`.
   - `TestRouting` and `TestRoutingProblems` check that generated routes are not shadowed.
6. Middleware, each tested: **met**.
   - Request ID: `requestid_test.go` and `TestRequestIDIsEchoedAndLogged`. The probe echoed `client-id-123`
     and replaced a 200-character ID with a UUID.
   - Access log: `TestAccessLog`. The probe line is
     `msg="http request" method=GET path=/api/v1/health status=200 duration=500.6µs bytes=61 request_id=... user_agent=...`.
   - Recover: `TestRecoverWritesProblemAndLogsStack` and `TestRecoverAfterHeadersWereSentOnlyLogs`.
     The probe panic gave 500 `problem+json`, and the log has `level=ERROR msg="http handler panicked"` with the stack.
   - Security headers: `TestSecurityHeadersOnEveryResponse` covers the api, not-found and panic paths.
     The probe shows `nosniff` and `no-referrer` on every response.
7. `server.New(Options)` and `Run(ctx)`: **met**.
   - `Options` has `Address` or `Listener`, `Logger`, `APIHandler` and an optional `Fallback`.
     `newHandler` returns 404 when `Fallback` is nil.
   - `ShutdownTimeout = 10 * time.Second` (`server.go:19`).
   - `ReadHeaderTimeout` is set.
   - `WriteTimeout` stays unset, with a comment explaining why (`server.go:70-72`).
   - The `Fallback` extension point is covered by `TestRoutesSplitAPIAndFallback`.
8. A test starts the server on `127.0.0.1:0`, calls both endpoints, cancels and asserts `Run` returns nil: **met**.
   - `TestRunServesAPIAndShutsDownOnCancel`. The probe also printed `Run returned: <nil>`.
9. `httptest` handler and middleware tests with an in-memory fake service: **met**.
   - `helpers_test.go` provides `fakeSystemService`.
10. Gates (lint, tests, archtest, coverage): **met** within scope, see the Gates table.

## Orchestrator notes

- `/health` never exposes raw adapter error text: **met**.
  - `publicHealthDetails()` is a closed set. Any other text becomes `check failed`.
  - Probe: the raw detail `dial tcp 10.0.0.5:5432: password=hunter2` was sent to the client as
    `"detail":"check failed"`. The raw text appears only in the server-side WARN log.
  - `TestGetHealthNeverSendsUnknownDetails` covers this.
- Test tying `system.APIVersion` to the `/api/v<version>` prefix: **met**.
  - `api/spec_test.go` has `TestEveryPathCarriesAPIVersion`.
- `Info` errors wrapping `ErrConflict`, `ErrNotFound` or `ErrInvalid` map to 500: **met**.
  - `TestGetSystemInfoFailuresAreInternal` covers plain, NotFound, Conflict, Invalid and Forbidden.
  - The probe confirmed the NotFound case.
- Problem+json error handlers instead of the generated text/plain defaults: **met**.
  - `api.go` uses `NewStrictHandlerWithOptions` and `HandlerWithOptions` with `RequestErrorHandlerFunc`,
    `ResponseErrorHandlerFunc` and `ErrorHandlerFunc`.
  - `TestMalformedRequestHandlersWriteProblemsWithoutErrorText` tests both the strict and the parameter-binding paths.
- `BuildTime` parsed with an explicit error path: **met**.
  - `parseBuildTime` uses RFC 3339 and wraps parse errors. `TestParseBuildTime` covers valid, offset, empty
    and three invalid forms.
  - A malformed value is omitted and logged as a warning. `TestGetSystemInfoOmitsMalformedBuildTime` covers it.
- `api/spec_test.go` uses `system.APIVersion` and prints the dereferenced security list: **met**.
  - `apiMajorVersion` is gone. `describeSecurity` prints `*security`.
  - `go test ./api/...` passes.

## Black-box probe

The probe was a throwaway `main` injected with `go run -overlay`. It lived only in the scratchpad
(`...\scratchpad\probe\main.go`). `git status --short cmd` is empty, so nothing was added to the repo.
It wired `restapi.NewHandler(fake)` and `server.New` on `127.0.0.1:0` and made real HTTP requests.

| Probe | Result |
|---|---|
| `GET /api/v1/health` | 200, `application/json` |
| Failing ping, public detail | 503, `"detail":"database ping failed"` |
| Failing ping, raw detail | 503, `"detail":"check failed"` |
| `/system/info`, error wrapping `ErrNotFound` | 500 `problem+json`, no internal text |
| `POST /api/v1/health` | 405 `problem+json`, `Allow: GET, HEAD` |
| `GET /api/v1/nope` | 404 `problem+json` |
| `GET /nope`, no fallback | 404 `text/plain` (expected, not an API path) |
| Valid `X-Request-ID: client-id-123` | echoed |
| `X-Request-ID` of 200 characters | replaced by a UUID |
| `X-Request-ID` with a tab | replaced by a UUID |
| `X-Request-ID` with a quote and `level=ERROR msg=forged` | replaced by a UUID |
| `X-Request-ID` with control characters | rejected with 400 by `net/http` before the handler |
| `X-Request-ID` followed by CRLF and an injected header | `abc` echoed, no `X-Injected` header |
| Query string `?token=SECRETQUERY`, `Authorization: Bearer SECRETAUTH`, `Cookie: sid=SECRETCOOKIE` | none of the three secrets, nor `token=`, appears in the log |
| Panic in a handler | 500 `problem+json`, ERROR log with stack, security headers still set |
| Cancel the context | `Run returned: <nil>` |

## Findings

No blockers. Two notes for later:

- Non-blocking: no test in `internal/adapters/http` asserts at compile time that `*appsystem.Service`
  satisfies `restapi.SystemService`. The probe checked it by hand. M1-10 wiring will enforce it at the
  composition root, or `restapi` could add `var _ SystemService = (*appsystem.Service)(nil)` in a test.
- Informational: the WARN log for a non-public health detail writes the raw detail
  (`detail="dial tcp 10.0.0.5:5432: password=hunter2"`). This is deliberate (operator log, not client),
  but if drivers can ever put credentials into the error text, it should go through the log redaction.

## History

- Round 1: PASS.
- Round 2: PASS (see the Round 2 section).

---

# Round 2

Status: **PASS**

Scope: `./internal/adapters/http/...`, `./api/...`, `./internal/archtest/...`. Changes under test:
new `statuswriter` package, `server/{middleware.go,logtext.go,middleware_internal_test.go}`,
`restapi/{api.go,router.go,system.go,system_test.go,abandoned_internal_test.go}`, task file.

## Gates

| Gate | Command | Result |
|---|---|---|
| vet | `go vet ./internal/adapters/http/... ./api/...` | exit 0 |
| tests | `go test -race -count=3 -cover ./internal/adapters/http/... ./api/... ./internal/archtest/...` | all `ok`, exit 0 |
| lint | `golangci-lint run ./internal/adapters/http/... ./api/...` | `0 issues.` |
| format | `golangci-lint fmt --diff ./internal/adapters/http/... ./api/...` | no diff, exit 0 |
| layer rule | `go list -deps ./internal/domain/...` filtered for `database/sql`, `net/http`, `os/exec`, `internal/adapters`, `internal/app` | no matches |
| restapi must not import server | `go list -deps ./internal/adapters/http/restapi` filtered for `http/server` | no matches |

Coverage (`-race -count=1`): requestid 100.0 %, restapi 98.2 %, server 97.1 %, statuswriter 100.0 %,
archtest 95.4 %, scoped total (generated apigen excluded) 98.1 % (threshold 70 %). The uncovered statements are
the existing defensive branches at `restapi/problem.go:90-93` and `server/server.go:166-173`. No
`internal/domain` file changed.

## Black-box probe, round 2

Throwaway `main` injected with `go run -overlay` (`...\scratchpad\probe2\main.go`, overlay
`...\scratchpad\overlay2.json`). `git status --short cmd` is empty. It used `server.New` on `127.0.0.1:0`
with a custom `APIHandler` for (a), (b), (c), (e) and `restapi.NewHandler` with a slow fake for (d).
The text log handler ran at debug level.

| Case | Result |
|---|---|
| (a) headers + `partial-body` flushed, then `panic` (chunked) | Client `io.ReadAll` returns `unexpected EOF` after `partial-body`. The raw bytes end after the first chunk, with no `0\r\n\r\n` terminator. One ERROR line (the panic, with stack). Access log: `status=200 aborted=true bytes=12`. **met** |
| (a) `WriteHeader(200)` then panic, nothing else | Client `Do` fails with `EOF`, no response. **met** |
| (a) `Content-Length: 1000`, 5 bytes written, then panic | Client `Do` fails with `EOF`. **met** |
| (b) `panic(http.ErrAbortHandler)` before any write | Client `EOF`. 0 ERROR and 0 WARN lines. Access log has `aborted=true`. **met** |
| (b) `ErrAbortHandler` after a flushed byte | Client gets `x` then `unexpected EOF`. 0 ERROR, 0 WARN. **met** |
| (c) 5 KB path and 5 KB `User-Agent` | `path` and `user_agent` are exactly 256 bytes and end in `...` (the marker is inside the 256). Stricter than "256 + marker", so **met**. |
| (c) 200 x 3-byte rune path and `User-Agent` | `path` 256 bytes, `user_agent` 255 bytes: the cut moves back to a rune boundary. Both end in `...`. **met** |
| (c) 900 KiB `User-Agent` | Accepted (below the 1 MiB header limit), logged as 256 bytes ending in `...`. **met** |
| (d) cancel mid-request on `/system/info`, fake returns `ctx.Err()` | 0 ERROR, 0 WARN, 0 `superfluous`. One DEBUG `api response abandoned`. `Run` returns nil. **met** |
| (d) same, fake returns `fmt.Errorf("query: %w", ctx.Err())` | Same: 0 ERROR, 0 WARN, DEBUG only. **met** |
| (d) same, fake returns `ErrNotFound` after the cancel | Same: 0 ERROR, 0 WARN, DEBUG only (not logged as a server fault). **met** |
| (e) handler sets `Cache-Control`, `ETag`, `Content-Type: text/html`, `Content-Length: 100`, then panics | 500 `application/problem+json` with `Content-Length: 91` (correct for the problem). Headers are only `Content-Type`, `Referrer-Policy: no-referrer`, `X-Content-Type-Options: nosniff`, `X-Request-Id`, `Date`, `Content-Length`. No `Cache-Control`, no `ETag`. **met** |

Round-1 behaviours, re-checked with the old probe (`probe\main.go`):

| Probe | Result |
|---|---|
| `/health` healthy | 200, `application/json`, `{"checks":[{"name":"database","status":"ok"}],"status":"ok"}` |
| `/health` failing, raw detail `dial tcp ... password=hunter2` | 503, client gets `"detail":"check failed"` |
| `/health` failing, public detail | 503, `"detail":"database ping failed"` |
| `/system/info` ok | 200, all six fields, `serverId` from the service |
| `/system/info`, error wrapping `ErrNotFound` with a secret path | 500 `problem+json` `internal_error`, no path in the body. The path is only in the ERROR log. |
| `POST /api/v1/health` | 405 `problem+json` `method_not_allowed`, `Allow: GET, HEAD` |
| `GET /api/v1/nope` | 404 `problem+json` `not_found` |
| `GET /nope` without fallback | 404 `text/plain` (not an API path, expected) |
| Request ID reuse, 200-char ID, tab, quote and forged log text, control chars, CRLF injection | Same as round 1: valid ID echoed, bad ones replaced by a UUID, control characters get 400 from `net/http`, no `X-Injected` header |
| `?token=...`, `Authorization`, `Cookie` | none of them appear in the log |
| Panic in `Health` | 500 `problem+json`, ERROR with stack, security headers present |
| Cancel the server context | `Run returned: <nil>` |

The access log now carries `aborted=<bool>`. The `hunter2` text is still in the server-side WARN log (round-1
informational note, unchanged).

## Findings

No blockers. Non-blocking observations, none of which breaks an acceptance criterion:

- `server/middleware.go:175-179` `isAbort` uses `errors.Is`, but net/http compares `== http.ErrAbortHandler`. A handler
  that panics with a wrapped abort (`fmt.Errorf("x: %w", http.ErrAbortHandler)`) is re-panicked as is (line 148), and
  net/http logs `http: panic serving ...` with a stack at WARN through `Server.ErrorLog`. There is no ERROR line and the
  connection is dropped, so it is harmless. The bare sentinel, as the net/http contract says, is silent. If wanted,
  re-panic `http.ErrAbortHandler` itself instead of `recovered`.
- The access log shows the recorded or default status for an aborted request (`status=200 aborted=true bytes=0` when
  nothing was sent, and likewise for a client that cancelled before any write). `aborted` disambiguates, so this is
  only a readability note.
- The 256-byte limit includes the `...` marker (a cut value is at most 253 bytes plus the marker). That is within the
  requested "<= 256 bytes + marker".
- Round-1 notes still open: no compile-time check that `*appsystem.Service` satisfies `restapi.SystemService` (M1-10
  wiring will enforce it), and the raw health detail in the WARN log.

## Acceptance criteria (round 2)

All ten criteria and the orchestrator notes remain **met**. The recover criterion ("a panicking handler yields a 500
problem and an error log with stack") holds when nothing was sent yet (probe (e) and the old panic probe). When part of
the response is already out, the connection is aborted instead, and the error log with stack is kept (probe (a)). The
task file now lists `requestid` and `statuswriter` in its areas.

## History (round 2)

- Round 2: PASS.