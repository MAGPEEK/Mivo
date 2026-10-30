# M2-29 test report – direct file and HLS delivery handlers (`internal/adapters/http/stream`)

Status: **PASS** (with non-blocking test-gap findings N1, N2)
Tested together with M2-25 in a clean worktree (detached at 5e64b56 plus the 29 files of
`m2-25-29-files.txt`, corpus copied from `testdata/media`); the worktree is removed again.
M2-29 builds on the M2-25 `Access`/`Release` API, which behaved as specified in this run.

## Gates

| Gate | Result |
|---|---|
| `go build ./...`, `go vet ./...` | clean |
| `go test -coverprofile ./...` (Windows) | all ok |
| coverage `internal/adapters/http/stream` | 97.1 % |
| go-test-coverage | total 97.0 %, threshold 70 % met |
| `golangci-lint run --allow-parallel-runners` (incl. depguard) | 0 issues |
| `golangci-lint fmt --diff`, `go mod tidy -diff` | clean |
| archtest | ok |
| layer check | `stream` imports only app/playback, domain and sibling http packages (apigen, restapi, statuswriter, logtext, requestid); no other adapter in production code |
| Linux race (Docker) `go test -race ./...` | all ok |
| Flake hunt `-race -count=20 playback + stream` (Docker) | ok |
| Corpus test with real FFmpeg 9.0.2 on Windows | `TestCorpusRemuxIsServedOverHTTP` PASS (0.19 s; also `-count=3` ok). In Docker it skips (no FFmpeg), as designed. |

Note: the first corpus run skipped because my copy of `testdata/media` landed one directory too
high; after fixing the copy the test ran and passed. Not a repository problem.

## Acceptance criteria

1. Mounts for `GET .../file` and `GET .../hls/{fileName}` plus URL builder: MET.
   `Handlers.Mounts()`, `URLBuilder.StreamURL` (`routes.go`): direct URL, or `.../hls/index.m3u8`;
   patterns and builder share constants (`TestSessionStreamURLsReachTheirHandlers`).
2. Both call `Access`; unknown or stopped session 404, malformed id 400: MET. Probe over a real
   HTTP server: ids `not-a-uuid`, `12345`, `%00`, 500 characters give 400 on both routes; stopped
   and unknown sessions give 404; the HLS route on a direct session and the file route on an HLS
   session give 404; POST gives 405.
3. Direct file via `libraryfs.FileSystem.Open`, `http.ServeContent`, ADR 0023 Content-Type: MET.
   Probe on a 64 MiB file: `bytes=0-9` gives 206 with `Content-Range: bytes 0-9/67108864`;
   `bytes=-5` gives 206/5 bytes; `bytes=999999999999-` gives 416 with `bytes */67108864`;
   If-Range with matching Last-Modified gives 206, with a stale date gives 200 and the full body;
   HEAD gives 200 with Content-Length; no ETag anywhere; Content-Type `video/mp4`;
   `Accept-Ranges: bytes`. `TestDirectFileKeepsTheReaderFromPath` confirms `io.ReaderFrom`.
4. HLS names `index.m3u8|init.mp4|seg-[0-9]{5}.m4s` only, `os.Root`, explicit types and caching:
   MET. Probe (real server, counting `Sessions` wrapper): `seg-1.m4s`, `seg-000001.m4s`, Arabic-Indic
   and fullwidth digits, `%2F`, `..%2F`, `..%2Findex.m3u8`, `index.m3u8/`, `index.m3u8%2Fx`, newline
   suffixes, `seg-00000.m4sx`, `Init.mp4`, `init.mp4.`, 100,000-character names, a segment name
   with 100,000 zeros, `index.m3u8` plus 5,000 characters: all 404 and `Access` was never called.
   Types: playlist `application/vnd.apple.mpegurl` + `no-store`; init `video/mp4`; segments
   `video/iso.segment`, both `private, max-age=3600`.
5. Readiness wait <= 20 s, 503 `stream_not_ready` with `Retry-After: 2`, failed production 500
   logged with the session: MET. `ReadyTimeout` pinned to 20 s by test; 503 + `Retry-After: 2`
   by `TestPlaylistNotReadyInTimeAnswersRetryLater`; failed production gives 500 for playlist and
   segment (`TestFailedProductionAnswersInternalError`); stopped production gives 404, never 500.
   Detail: the handler's own record for a failed production is at debug (with `session=`); the
   cause is logged once by the producer at warn and the service logs the failure at info with the
   session ID (design from the M2-08/M2-24 notes, avoids one error line per player retry).
6. Corpus integration test, playlist, init, first segment all 200 with correct types: MET (see gates).
7. Gates: MET.

## Probes of the brief

- Release exactly once per successful Access, also on client disconnect mid-copy: MET.
  `TestEveryGrantIsReleasedOnce` (all handler exits) plus an end-to-end probe: a raw TCP client
  starts a Range request on a 64 MiB file, reads 1 KiB and stalls; 40 reaper ticks (10 min) do not
  reap the session; after the client closes the connection the session becomes reapable (Release
  ran). Removing `defer granted.release()` in either handler fails `TestEveryGrantIsReleasedOnce`.
- No file handle leaks: MET. On Windows, after full reads of playlist and init, a segment request
  aborted mid-copy and a HEAD, `Stop` removes the session directory immediately.
- Never a problem after the body started: guard exists (`writeProblem`); see N2.

## Mutation check (extra, one guard at a time, `go test ./internal/adapters/http/stream`)

Killed: segment digit count, name case, both release defers, retry-after header and value,
20 s constant, stopped-production 404, name check before `Access`, playlist caching, segment
content type, regular-file check, Last-Modified, direct Content-Type, canonical session id,
short-body warning, URL builder path and escaping, quiet patterns, the 404/400 mappings, the
file route delivery guard, readiness wait removed (tests hang), ErrNotFound mapping of output files.

## Findings (non-blocking test gaps, production behaviour verified correct)

N1. `hls.go:105` ASCII-digit class check is not pinned. Replacing
`strings.IndexFunc(number, isNotASCIIDigit) < 0` with `true` keeps all tests green, because
names like `seg-0000a.m4s` have no decoy file and are 404 either way. The real code is correct
(probe: 404 and no `Access` call); only the "without session lookup" part is untested for
five-byte non-digit names. Suggest a decoy file plus a counting `Sessions` for `seg-abcde.m4s`.
Same for the HLS-route delivery guard (`hls.go:72`): a direct session on the HLS route is 404 via
the missing directory anyway (equivalent outcome).

N2. "Cancelled with the request" is not pinned by timing. `hls.go:119`: using
`context.WithTimeout(context.Background(), ...)` instead of the request context keeps all tests
green; `TestPlaylistWaitEndsWithTheRequest` then simply waits the full 20 s and still sees 503
(whole package took 21.4 s). Suggest asserting that the response arrives well before the timeout
(or set a long `SetReadyTimeout` in that test). The `HeadersSent` guard in `respond.go:97` is
defensive and has no reachable trigger in the current handlers.

## History

- Round 1: this report (new package).

REPORT
status: PASS
task: M2-29
files: docs/reports/M2-29-test.md
tests: go vet/build clean; go test -coverprofile ./... ok, stream 97.1 %, total 97.0 %; golangci-lint 0 issues; fmt/tidy clean; archtest ok; Docker race ./... and -count=20 ok; corpus test with real FFmpeg PASS on Windows; HTTP probes ok
findings:
- N1 hls.go:105 digit-class check and N2 hls.go:119 request-cancellation of the wait are not pinned by tests (non-blocking)
next: optionally add the two tests together with the M2-25 fix round; otherwise hand to mivo-reviewer
