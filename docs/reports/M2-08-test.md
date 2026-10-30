# M2-08 test report – HTTP adapter: bounded log text, streaming-friendly writers, quiet stream logs

Status: **PASS** (round 2)
Date: 2026-10-30
Environment: Windows 11, Go 1.27.3, golangci-lint (repo config), real binary built with `-tags webui`
after `pnpm -C web build`. Scope: `./internal/adapters/http/... ./cmd/... ./internal/archtest/...`
(M2-12 `internal/adapters/fs/libraryfs` is mid-edit and not judged; the scoped packages built and
tested without trouble).

## Round-2 gates

| Gate | Command | Result |
| --- | --- | --- |
| vet | `go vet` and `go vet -tags webui` on the scope | exit 0 / exit 0 |
| web build | `pnpm -C web build` | exit 0 |
| tests, race | `go test -race -count=1 -coverprofile=<scratchpad>/plain.out <scope>` | all `ok`: logtext 100.0 %, statuswriter 100.0 %, healthprobe 100.0 %, requestid 100.0 %, restapi 98.2 %, server 97.2 %, webui 98.8 %, cmd/mivo 89.0 %, archtest 95.4 % |
| tests, race, `-tags webui` | same with `-tags webui` | all `ok`: restapi 98.2 %, server 97.2 %, webui 97.6 %, cmd/mivo 88.4 % |
| coverage gate | `go tool '-modfile=tools/go.mod' go-test-coverage --config=.testcoverage.yml --profile=<profile>` | plain: total 96.0 % (623/649) PASS; webui: 95.7 % (621/649) PASS (threshold 70 %). Scope only; no domain code touched in this task, repo-wide total and `internal/domain/...` >= 90 % not re-measured |
| lint | `golangci-lint run --allow-parallel-runners <scope>`, also `--build-tags webui` | `0 issues.` both |
| format | `golangci-lint fmt --diff` (repo-wide and scoped) | no diff, exit 0 |
| layer rules | `internal/archtest` (in the test run above) + `go list -deps ./internal/domain/...` filtered for `database/sql\|net/http\|os/exec\|internal/adapters\|internal/app` | `ok`; no match |
| flake, server + logtext | `go test -race -count=15 -run 'BodyReadDeadline\|BodyDeadline\|Quiet\|Recover\|AccessLog\|LogRecords\|Abort\|Truncat' ./internal/adapters/http/server/ ./internal/adapters/http/logtext/` | `ok 13.760s`, `ok 1.834s` |
| flake, restapi | `-count=15 -run 'Malformed\|RequestLog\|RemainingBody\|Truncat\|Abandon'` | `ok 2.421s` |
| flake, rest | `-count=15 ./internal/adapters/http/{statuswriter,webui}/`; `-count=3` whole `server` package | all `ok` |

Functions below 100 % in the changed files: `server/bodydeadline.go:39 setBodyReadDeadline` 66.7 %
(the "deadline not supported" debug log branch is not exercised), `restapi/bodylimit.go:53
remainingBodyTooLarge` 75.0 %, `webui/handler.go:158 openRegularFile` 90.9 %. Not blocking.

## Acceptance criteria (round 2)

1. **logtext move, 10 KiB tests: met.** `logtext` exports `Truncate`, `TruncateMiddle`, `TruncateMiddleTo`,
   `RequestAttributes`, `Error`, `MaxBytes` (256), `TruncationMarker`; the old `server/logtext.go` is gone.
   `RequestAttributes` (method and path cut, request_id) is used by `server/accesslog.go:54`,
   `server/middleware.go:127`, `server/bodydeadline.go:45`, `restapi/api.go:117,130,141,146,151`,
   `webui/handler.go:128`. `logtext.Error` (start and end kept) is used for the 4xx, abandoned and
   webui-500 error text. Panic value: `TruncateMiddleTo(..., maxLoggedPanicBytes = 4 KiB)`.
   Per-package 10 KiB tests pass: `TestLogRecordsTruncateClientText` (method, path, multi-byte User-Agent,
   panic value with its end kept), restapi `requestlog_*_test.go`, webui `TestServerErrorLogTruncatesClientText`.
2. **`Recorder.ReadFrom`: met** (unchanged from round 1, tests pass).
3. **Quiet routes: met, in the changed form.** The task text says `QuietPathPrefixes`; the code now has
   `server.Options.QuietPatterns` (ServeMux patterns, private mux), which the task's own 2026-10-30 note
   (lines 54-55) records as replacing the criterion. `TestAccessLogLevelForQuietPatterns` (16 rows): 200/206/304/no write
   and HEAD on `GET .../file` and `GET .../hls/{fileName}` -> DEBUG; 400/404/503/panic/abort, POST on the
   same path, `hls/a/b`, session GET/DELETE, `/api/v1/health`, web path -> INFO.
   `TestNewQuietRoutesRejectsBadPatterns` (empty, unclosed wildcard, duplicate, conflicting wildcards ->
   error, no panic), `TestQuietRoutesWithoutPatternsMatchNothing`. Malformed/conflicting is wrapped in
   `domain.ErrInvalid` through `validate` (`server.go:116-123`).
4. **BACKLOG "http nits": met** (abort status 0 / `aborted=true`, plain `http.ErrAbortHandler` re-panic,
   101 final, "Close" comments; tests pass, see round 1 for the 101 evaluation; orchestrator note matches).
5. **BACKLOG "http/webui nits (r2)": met** (API path before method in `webui/handler.go:81-95`, tests pass).
6. **Gates: met** (table above).
7. **Carry-over: read deadline: met in the new place.** `server/bodydeadline.go`: `BodyReadTimeout = 30 s`;
   `limitBodyReadTime` sets `http.NewResponseController(w).SetReadDeadline` whenever
   `request.Body != nil && != http.NoBody`, as the second-outermost middleware (`middleware.go:60-66`,
   after request ID, before security headers, access log, recover and every handler). No deadline for
   bodiless requests. `restapi` no longer sets one (`bodylimit.go` only documents the dependency).
   Tests over a real connection with a 200 ms timeout:
   `TestBodyReadDeadlineEndsSilentBodies` (8 rows, each must be answered not before the deadline and
   not hang: api early 413 -> 413, api chunked -> 400, trickled chunk -> 400, unknown api route -> 404,
   webui POST chunked -> 405, webui announced POST -> 405, GET with body -> 200, nil fallback -> 404),
   `TestBodyReadDeadlineSparesBodilessStreams` (stream for 3x the deadline, context alive, >= 12 chunks),
   `TestBodyReadDeadlineEndsWithTheBody` (announced and chunked; handler works 3x the deadline, context
   alive). All pass 15x under `-race`.
8. **Carry-over: bounded drain: met** (`bodylimit.go:58`, `io.LimitReader(request.Body, MaxRequestBodyBytes+1)`).
9. **Carry-over: log after classification: met** (`api.go:110-120`, status attribute).

## Black-box with the real binary (isolated `MIVO_*` config in the scratchpad, 127.0.0.1:18089, `-tags webui`, log level debug)

Raw TCP client; request head announces `Content-Length: 102400`, then nothing is sent. Three clients ran
in parallel.

| Check | Result |
| --- | --- |
| `POST /api/v1/playback/sessions`, silent | `HTTP/1.1 413 Request Entity Too Large` after 30 s (client time) |
| `POST /library` (webui path), silent | `HTTP/1.1 405 Method Not Allowed` after 30 s |
| `GET /` with `Content-Length: 102400`, silent | `HTTP/1.1 200 OK` after 30 s; server record `duration=30.0000781s bytes=522` |
| 10 KiB path `GET /aaaa...` | `200`; access record `path=/aaa...` ending in `...` (record 705 bytes in round 1; largest line of this run 438 bytes) |
| 10 KiB method `MMMM... /x` | `405`; record `method=MMM...` ending in `...`, `path=/x` |
| 10 KiB API path `/api/v1/bbbb...` | `404`; record path ends in `...` |
| 10 KiB file name `/fff....js` | real binary answers `404` (embedded FS reports `ErrNotExist`); a webui 500 is not reachable there, so see the probe below |

Probe for the webui 500 (throw-away `go build -overlay` main, no file in the repo, `cmd/` unchanged):
real `server.New` + real `webui.NewHandler` over an `fs.FS` whose `Open` fails with
`disk read failure: CAUSE-AT-END` for every file but index.html, on 127.0.0.1:18091.
`GET /` + 10 KiB of `f` + `.js` -> `500`; the error record is 660 bytes: `path=/fff...` cut with `...`
and `error="open ffff...` cut in the middle, ending in `...ffff.js: disk read failure: CAUSE-AT-END"`
(cause kept). `GET /short.js` -> `error="open short.js: open short.js: disk read failure: CAUSE-AT-END"` (uncut).

The early 413 and 405 records show `duration=0s` although the client waited 30 s: net/http discards the unread
body after the handler returned and the access log measures the handler only. See observations.

## Findings

None blocking. Observations (for the reviewer):
- Access-log `duration` for requests answered before the body arrived (early 413, 405, 404) covers the
  handler only, not the up to 30 s net/http spends discarding the body before the answer leaves; the
  log shows 0 s for a request the client experienced as 30 s. Harmless, may confuse an operator.
- `setBodyReadDeadline`'s error branch (no deadline support) has no test (66.7 %).
- `cmd/mivo` does not yet pass `QuietPatterns` (M2-32), so quiet behaviour is only exercised at `server` level.
- Repo-wide coverage and domain >= 90 % were not re-measured (scope as instructed).
- The criterion wording `QuietPathPrefixes` in the task body is superseded only by the note at the file end.

## History

- Round 1 (2026-10-30): PASS. Logtext move, `Recorder.ReadFrom`, prefix-based quiet logs, BACKLOG nits and the
  carry-over (read deadline then in `restapi/bodylimit.go`, bounded drain, log after classification) met;
  black-box incl. a trickled body cut off at 30 s through the real binary; the 101-as-final decision accepted.
  Open points then: the deadline only covered requests that reached restapi (early 413, webui 405 and nil-fallback
  404 waited unbounded); error text cut at the start only.
- Round 2 (2026-10-30): PASS. Deadline moved into `server` middleware, `QuietPatterns`, shared
  `logtext.RequestAttributes`/`TruncateMiddle`/`Error`, panic value 4 KiB; all gates green, black-box confirms
  answers after 30 s for API, webui and GET-with-body cases.

Stray files: none in the repo (coverage profiles, binary, client, probe and logs live in the scratchpad
`m208r2` directory; the overlay build created no file under `cmd/`). Both server processes were stopped.
