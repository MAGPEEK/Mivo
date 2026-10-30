# M1-16 Embed web UI – test report

Current status: PASS (round 2). The round 1 report follows; the round 2 section is at the end.

Round 1 – status: PASS

## Gates

| Gate | Result | Evidence |
|---|---|---|
| `go vet ./...` (untagged, `dist` moved aside) | PASS | exit 0 |
| `go test -race -count=1 -coverprofile=coverage.out ./...` | PASS | all packages ok; webui 98.6 %, cmd/mivo 85.7 % |
| Coverage: total >= 70 % | PASS | `go tool cover -func`: total 90.4 % |
| Coverage: `internal/domain/...` >= 90 % | PASS | `domain/system` 100.0 %, `domain` has no statements |
| `golangci-lint run` | PASS | `0 issues.` |
| `go build ./cmd/mivo` without `dist` | PASS | exit 0, 18.5 MB; `dist` was moved aside (`internal/adapters/http/webui/dist` absent) |
| `pnpm -C web build` | PASS | exit 0, wrote 10 files to `internal/adapters/http/webui/dist` |
| `go build -tags webui` without `dist` (sanity) | fails as intended | `files_embedded.go:20:12: pattern all:dist: no matching files found` |
| `go vet -tags webui ./...` | PASS | exit 0 |
| `go test -race -count=1 -tags webui ./cmd/... ./internal/adapters/http/...` | PASS | cmd/mivo, requestid, restapi, server, statuswriter, webui ok |
| `golangci-lint run --build-tags webui` | PASS | `0 issues.` |
| `go mod tidy -diff` | PASS | exit 0, no diff |
| `govulncheck` (tools modfile), untagged and `-tags webui` | PASS | "No vulnerabilities found." both |
| Layer rule (`go list -deps ./internal/domain/...`) | PASS | no `database/sql`, `net/http`, `os/exec`, `internal/adapters`, `internal/app` |

Not run (not relevant to this task): Playwright E2E, media corpus, Docker.

`dist` restore: the rebuilt `dist` is byte-identical (SHA-256, all files) to the one moved
aside, so the working tree is as found. The temporary copy was deleted.

## Acceptance criteria

1. `.gitignore` ignores `/internal/adapters/http/webui/dist/` – MET.
   `.gitignore:15` (`git check-ignore -v` confirms). After the web build,
   `git status --porcelain -uall` lists only the six source files in `webui/`; `dist` shows
   only under `--ignored` (`!! internal/adapters/http/webui/dist/`).
2. Untagged build on a clean checkout – MET. Build works without `dist`; untagged binary,
   `GET /` -> 200 `text/html; charset=utf-8`, `Cache-Control: no-cache`, `<title>Mivo</title>`
   (from brand), `<pre>` with `pnpm -C web install`, `pnpm -C web build`,
   `go build -tags webui ./cmd/mivo`. Log line:
   `web app not embedded, serving build instructions instead`. `/library/x` returns the same
   page, `/missing.js` 404.
3. Tagged binary (127.0.0.1, free port, isolated LOCALAPPDATA/config) – MET.
   - `GET /` -> 200, 522 bytes, `Cache-Control: no-cache`, `text/html; charset=utf-8`;
     body is the built index.html (`id="root"` also asserted in `files_embedded_test.go`).
   - `GET /assets/index-Cmd4vm6N.js` -> 200 `text/javascript; charset=utf-8`,
     `public, max-age=31536000, immutable`.
   - `GET /assets/index-CP92420n.css` -> 200 `text/css; charset=utf-8`, same immutable cache.
   - `GET /library/x` -> 200 index.html, `no-cache`.
   - `GET /missing.js`, `/assets/missing.js` -> 404.
   - `GET /api/v1/does-not-exist` (`/api/v1/nope`) -> 404 `application/problem+json`,
     `{"code":"not_found",...}`; `/api/nope.js` also problem+json, never index.html.
   - `GET /api/v1/health` -> 200 `{"checks":[{"name":"database","status":"ok"}],"status":"ok"}`.
4. Handler takes `fs.FS`; only tag files differ – MET. `NewHandler(files fs.FS)`;
   `files_embedded.go` (`//go:build webui`, `embed`) and `files_placeholder.go`
   (`//go:build !webui`) both expose `Embedded` and `Files()`.
5. Tests – MET. `handler_test.go` covers content types, cache headers, SPA fallback, dot
   directories, directories not listed, 404 without file content, HEAD, 405, Range, seekless
   files, error paths (500), `NewHandler` validation. Routing test
   `TestRunRoutesAPIAndWebUI` (cmd/mivo) passes in both variants and checks that API paths
   (including `/api/library/x`) get problem+json and never the UI.
6. Gates – MET (see table).

### Orchestrator note
`.gitignore` entry present; `pnpm -C web build` writes to the `dist` directory. Without
`dist` the tagged build fails at compile time, the untagged build is unaffected (ADR 0020).
MET.

### Review carry-over from M1-10
- Bound port is not `config.DefaultPort`: MET. `assertNotDefaultPort` in
  `cmd/mivo/serve_test.go`, called from `runAndQueryServer`.
- Close error on cancelled startup is reported while "stopped" is decided from the startup
  error only: MET. `withCloseError` (`serve.go:112`, 100 % covered);
  `TestWithCloseErrorKeepsCloseFailures` covers six cases, including "stopped while starting
  does not hide a close failure".
- "start database %s" prefix consistent: MET (`serve.go:97` and migration path).
- `application.run` renamed to `start`: MET (`serve.go:68`).
- Exact "http server listening" count: MET (`listeningMessage` constant).
- Data directory removable after the second run: MET (`os.RemoveAll` assertion at the end of
  `TestRunServesSystemEndpointsAndKeepsServerIDAcrossRestarts`; passes on Windows).

## Smoke run details (tagged binary)

Request/response behaviour observed (all via `curl.exe`, `--path-as-is` where noted):

- `HEAD /assets/index-Cmd4vm6N.js` -> 200, correct `Content-Length: 394242`, content type
  and cache headers, no body.
- `Range: bytes=0-99` on the same asset -> 206, `Content-Range: bytes 0-99/394242`,
  `Content-Length: 100`. `Accept-Ranges: bytes` is advertised.
- Conditional requests (documented behaviour): responses carry neither `ETag` nor
  `Last-Modified`, because `embed.FS` reports a zero modification time and `ServeContent`
  omits `Last-Modified` for it. Consequently `If-Modified-Since` never yields 304 and
  `If-None-Match: "bogus"` yields 200. Only `If-None-Match: *` produces 304. So
  `index.html` (`no-cache`) is re-sent in full on each revalidation; hashed assets avoid the
  request entirely via `immutable`.
- `/assets/` and `/assets` -> 200 `index.html` (no directory listing, SPA fallback because the
  path has no extension). `/index.html` -> 200.
- `POST /` -> 405 with `Allow: GET, HEAD`.
- Traversal, all of them returned 404 with `404 page not found` and no file content:
  `/go.mod`, `/..%2f..%2fgo.mod`, `/assets/..%2f..%2f..%2fgo.mod`,
  `/%2e%2e/%2e%2e/go.mod`, `/..\..\go.mod`, `/..%5c..%5cgo.mod`, `/assets\..\..\go.mod`,
  `/assets%5c..%5c..%5cgo.mod`, `/C:/Windows/win.ini`, `/C%3A%5CWindows%5Cwin.ini`,
  `/index.html%00`.
  `/assets/../../go.mod` and `/../../go.mod` (sent raw) -> 307 to `/go.mod` (ServeMux path
  cleaning), which is 404. No file content at any step.
- `GET /api` -> 307 to `/api/` (mux behaviour; `/api/` goes to the API handler).
- Untagged binary gave the identical answers for the same request set, except the page content.

Shutdown / process hygiene: the servers were stopped with `Stop-Process` (TerminateProcess,
exit code -1), because Windows offers no portable SIGINT for a detached console process in
this environment. Graceful shutdown (clean exit, database closed) is covered by the Go tests
(`assertCleanExit`, data-dir removal). After the runs, `Get-Process mivo*` found 0 processes.
Scratch data lives only under the session scratchpad; the user's real LOCALAPPDATA was not used.

## Findings (non-blocking)

1. `internal/adapters/http/webui/handler.go` / no validators: documented above. If
   revalidation of `index.html` should be cheap, an ETag (for example a content hash computed
   when the handler starts) would be needed. Not required by the criteria.
2. `cmd/mivo/serve.go:~104` (doc comment of `withCloseError`): still says "run decides on a
   clean stop", but `run` is now `start`/`stoppedWhileStarting`. Comment only.
3. `GET /assets/` returns the SPA page rather than a 404. Acceptable per the task (no listing)
   and consistent with the extension-based fallback rule.

## History

- Round 1: PASS. All gates green in both variants, all criteria and carry-overs met.
- Round 2: PASS. See below.

---

# Round 2 – status: PASS

Scope: `webui/{handler.go,handler_test.go}` (API names answered with 404 problem+json after
decoding and cleaning; `NewHandler(files, logger)` logs Open/Stat/Read failures),
`server/{server.go (apiPathGuard),server_test.go,middleware_internal_test.go}`,
`cmd/mivo/{serve.go,serve_test.go}`.

## Gates (round 2)

| Gate | Result | Evidence |
|---|---|---|
| `go vet ./...` (untagged, `dist` moved aside) | PASS | exit 0 |
| `go test -race -count=1 "-coverprofile=coverage.out" ./...` | PASS | all packages ok, exit 0; server 97.2 %, webui 98.8 %, cmd/mivo 85.7 % |
| Coverage: total >= 70 % | PASS | `go tool cover -func`: total 90.5 % |
| Coverage: `internal/domain/...` >= 90 % | PASS | `domain/system` 100.0 % (every function), `domain` has no statements |
| `golangci-lint run` (untagged) | PASS | `0 issues.` |
| `go build ./cmd/mivo` without `dist` | PASS | exit 0 |
| `pnpm -C web build` | PASS | exit 0; rebuilt `dist` byte-identical (SHA-256 per file) to the one moved aside |
| `go vet -tags webui ./...` | PASS | exit 0 |
| `go test -race -count=1 -tags webui ./internal/adapters/http/... ./cmd/...` | PASS | requestid, restapi, server, statuswriter, webui, cmd/mivo ok |
| `golangci-lint run --build-tags webui` | PASS | `0 issues.` |
| Layer rule | PASS | unchanged from round 1; the diff touches only adapters and `cmd`, no `internal/domain` file |

Tooling note (no repo impact): PowerShell splits an unquoted `-coverprofile=coverage.out` at
the dot, so my first run failed with `no required module provides package .out`. It also wrote
a stray `coverage` file, which I deleted. The rerun with quoted arguments is the one reported.
`coverage.out` is gitignored (`.gitignore:6`). `git status` shows only the pre-existing
modified files plus `webui/` and this report; nothing was staged by me.

Not run (not relevant): Playwright E2E, media corpus, Docker, govulncheck and go mod tidy (no
dependency change in this round).

## Round-2 changes verified

- `apiPathGuard` (`server.go`, `newHandler`): `Fallback` never sees a request whose decoded
  and cleaned path is `/api` or below it; such requests go to `APIHandler`.
  `TestRunNeverHandsAPIPathsToTheFallback` uses a counting fallback: `/api/v1/health` 200,
  `/api/v1/x` 404, `/api/` 404, `/api` 307, `/api%2Fv1%2Fx` 404, `/%2e%2e/api/v1/x` 404,
  fallback calls 0, and a control request `/library/x` reaches it exactly once. Passes with
  `-race`.
- `webui.Handler` defence in depth: `isAPIName` after `path.Clean` returns the 404 problem
  (`TestHandlerAnswersAPIPathsWithNotFoundProblem`).
- `NewHandler(files, logger)`: nil logger is rejected (`TestNewHandlerRequiresLogger`); the
  500 path logs exactly one error record `web ui request failed` with method, path,
  request id and the cause, while the body contains no internal text
  (`TestHandlerReportsUnreadableFilesAsServerError`, four cases: open, stat, permission
  denied, read).
- `cmd/mivo`: `newHTTPHandlers` / `newWebUIHandler` pass the logger. Log line
  `web app not embedded, serving build instructions instead` appears only without the tag.

## Smoke run (isolated, 127.0.0.1, free port, scratch dirs under the session scratchpad)

Tagged binary (`go build -tags webui`), via `curl.exe --path-as-is`:

| Request | Status | Notes |
|---|---|---|
| `/api%2Fv1%2Fx` | 404 | `application/problem+json`, `code: not_found` (in round 1 this was index.html) |
| `/api%2F` | 404 | problem+json |
| `/%2e%2e/api/v1/x` | 404 | problem+json |
| `/api%2fv1%2fx`, `/%2e%2e%2fapi%2fv1%2fx`, `/foo/..%2fapi%2fv1%2fx` | 404 | problem+json (hex case, encoded dot-dot with encoded slash, nested) |
| `/API/v1/x` | 200 | SPA `index.html`, `no-cache`. Documented behaviour: matching is case-sensitive, so `/API/...` is a client route, not the API (same as the mux pattern `/api/`). No file content or API data is exposed. |
| `/api/v1/health` | 200 | `{"checks":[{"name":"database","status":"ok"}],"status":"ok"}` |
| `/apix/v1` | 200 | SPA `index.html` (`id="root"`), so the guard does not over-match on the prefix |
| `/library/x`, `/`, `/index.html`, `/assets/` | 200 | `index.html`, `text/html; charset=utf-8`, `no-cache` |
| `/api` | 307 | `Location: /api/` (mux); `/api/` -> 404 problem+json |
| `/api/v1/nope` | 404 | problem+json |
| `/assets/index-Cmd4vm6N.js` | 200 | `text/javascript; charset=utf-8`, `public, max-age=31536000, immutable` |
| `/assets/index-CP92420n.css` | 200 | `text/css; charset=utf-8`, same immutable cache |
| `/missing.js`, `/assets/missing.js` | 404 | plain `404 page not found` |
| `/assets/../api/v1/x` | 307 | `Location: /api/v1/x` (mux cleaning), then API |

Round-1 traversal list, all 404 `404 page not found` with no file content:
`/go.mod`, `/..%2f..%2fgo.mod`, `/assets/..%2f..%2f..%2fgo.mod`, `/%2e%2e/%2e%2e/go.mod`,
`/..\..\go.mod`, `/..%5c..%5cgo.mod`, `/C:/Windows/win.ini`, `/C%3A%5CWindows%5Cwin.ini`,
`/index.html%00`. `/assets/../../go.mod` -> 307 `/go.mod` (404 afterwards).

Untagged binary: identical statuses and headers for the same request set; the page is the
placeholder (build instructions) instead of the app, and the encoded API paths
(`/api%2Fv1%2Fx`, `/%2e%2e/api/v1/x`, ...) give problem+json as well.

Process hygiene: servers were stopped with `Stop-Process -Force` (TerminateProcess; graceful
shutdown is covered by the Go tests). Once, a truncated pipe in my own script left two of my
test servers running; I killed them. Final check: 0 `mivo*` / test-binary processes. The
built test executables live only in the session scratchpad. The repo's `dist` directory is
present again (rebuilt, identical to before; gitignored).

## Acceptance criteria (round 2)

All six criteria from round 1 remain MET (re-checked by the smoke table above: `.gitignore`
entry, untagged build and placeholder page, tagged assets/fallback/404/API behaviour,
`fs.FS` handler with two tag files, tests, gates). The routing requirement "API paths never
reach the UI handler" now also holds for encoded forms, at two layers (server guard and
handler).

## Findings (non-blocking)

1. `cmd/mivo/serve.go:109` (doc comment of `withCloseError`): still says "run treats every
   error that wraps the shutdown request as a clean stop", but `run` was renamed to
   `start` and the decision lives in `stoppedWhileStarting`. Carried over from round 1,
   comment only.
2. `/API/v1/x` (any case variant) gets the SPA page, by design (see smoke table). A
   case-insensitive API prefix would be a separate decision.
3. Carried over, unchanged: no ETag/Last-Modified on embedded files; `/assets/` falls back
   to `index.html`.

REPORT
status: PASS
task: M1-16
files: docs/reports/M1-16-test.md
tests: round 2 untagged: go vet, go test -race -count=1 ./... (all ok), coverage total 90.5 %, domain 100 %, golangci-lint 0 issues, go build without dist ok; tagged (after pnpm -C web build): go vet -tags webui, go test -race -tags webui ./internal/adapters/http/... ./cmd/... ok, golangci-lint --build-tags webui 0 issues; smoke of tagged and untagged binaries: encoded API paths give 404 problem+json, /apix/v1 and /library/x give the SPA, traversal list all 404, no processes left
findings:
- cmd/mivo/serve.go:109 stale comment in withCloseError ("run treats ...") after rename to start (carried over, comment only)
- /API/v1/x (case variant) is served the SPA index.html; case-sensitive by design, documented
- No ETag/Last-Modified on embedded files; /assets/ falls back to index.html (carried over)
next: orchestrator may hand over to mivo-reviewer; nothing blocking
