# M2-28 test report: REST API playback sessions

Verdict: **PASS** (round 1, tested together with M2-26)

Isolation: clean `git worktree` at HEAD 5e64b56 plus exactly the 20 files of
`m2-26-28-files.txt`; copies byte-identical to the main tree. The worktree is removed. The `stream`
adapter (M2-29) and the changed `app/playback` of the main tree are not part of this run; the handler
tests use a fake `PlaybackService`, and `app/playback.Service` at HEAD satisfies the port.

## Gates

Same run as M2-26, see `docs/reports/M2-26-test.md` for the table. Summary: build, vet, tests
(`restapi` 99.1 %, total 97.1 % vs 70 %), coverage gate, golangci-lint (0 issues), `fmt --diff`,
`go mod tidy -diff`, archtest, Linux race in Docker (all ok), codegen drift (clean), web e2e (10
passed). Web lint/test have 2 failing tests and lint errors in `ItemPage.test.tsx`/`router.test.tsx`
that also occur at clean HEAD (missing `createFakeServer`/`jsonResponse` in the committed
`fakeApi.ts`, an M2-22 wave problem) and are unrelated to this task.

## Acceptance criteria

1. **Port satisfied by `app/playback.Service`, stubs replaced - met.** `playback_test.go:25`
   `var _ restapi.PlaybackService = (*appplayback.Service)(nil)` compiles; nil service still answers
   501 (`TestPlaybackOperationsAnswerNotImplemented`).
2. **POST 201 with Location and session, profile validated - met.** Probe: 201 with
   `Location: /api/v1/playback/sessions/5d4c3b2a-...` (equals the session ID). Invalid requests give
   400 and the service is never started (0 calls): 17 direct-play entries (`invalid`), 33 codecs per
   list, codec shape `"H264 ; rm -rf"`, 65-character profile name, container `../x`, unknown HLS
   delivery, 33 codec limits, `maxBitDepth` 0 and 17, 17 limit profiles, negative and 1e30 start
   position, negative audio stream index, missing `fileId` and missing profile (all `invalid`);
   empty body and non-UUID `fileId` give `bad_request` (400).
3. **Error mapping, specific before generic - met.** Probe table through `POST`:
   `ErrNotFound` 404 `not_found`; `playback.ErrNoCompatibleFormat` 422 `no_compatible_format`;
   `playback.ErrTranscodeUnavailable` 422 `transcode_unavailable`;
   `appplayback.ErrSessionLimitReached` 503 `session_limit_reached` with `Retry-After: 30`
   (constant `sessionLimitRetryAfterSeconds`, playback.go:32; no other code sets the header);
   `domain.ErrUnsupported` 422 `unsupported`; `domain.ErrUnavailable` 503 `unavailable`;
   plus `ErrInvalid` 400, `ErrConflict` 409, `ErrForbidden` 403 and an unknown error 500
   `internal_error`. Order in `domainErrorMappings` (problem.go:76) puts the playback sentinels
   before the generic ones. Table tests `TestCreatePlaybackSessionMapsStartErrors` and
   `TestProblemForPlaybackErrors` green.
4. **GET 200/404, DELETE 204 always - met.** Probe: GET known 200, `ErrNotFound` 404 `not_found`;
   DELETE of a known and of an unknown session ID both 204 with empty body.
   `TestStopPlaybackSessionMapsServiceErrors` covers a failing stop.
5. **`streamUrl` passed through, no stream paths built here - met.** Fixture URL
   `/opaque/stream?token=a%2Fb&x=1` (deliberately not route-shaped) comes back unchanged; grep of the
   non-test playback files finds no stream path formatting (only the `Location` path constant).
   `TestStreamRoutesAreNotGenerated` green.
6. **Reasons, warnings, streams mapped 1:1, unknown codes pass through - met.**
   `TestPlaybackSessionPassesUnknownValuesThrough`, `assertSessionBody` against
   `wantTranscodeSession`; direct-play probe: `"reasons":[]`, `"warnings":[]`, `"streams":[...]` and
   no `null` anywhere in the body (`TestDirectPlaySessionSerialisesEmptyLists`).
7. **Handler tests with fake service for every status code and JSON shape - met.** Files
   `playback_test.go`, `playback_errors_test.go`, `playback_request_test.go`,
   `playback_response_test.go`; all green, also under `-race` on Linux.
8. **Gates - met** (see above).

Notes of the task:
- One info line per start (mode, reasons, file ID, session ID) and per stop; profile body never
  logged at info - met: `TestCreatePlaybackSessionLogsTheStart`; probe with profile name
  `secret-profile-name`: the full log output (all levels) does not contain it. The profile is
  validated before the service runs, so hostile profiles never reach the engine or the log.
- Direct-play nil `Reasons`/`Warnings` serialised as `[]` - met (above).
- `library.go` and `api.go` not edited by this task - met.
- A started session that cannot be described (non-UUID ID) answers 500 and is stopped
  (`TestCreatePlaybackSessionStopsASessionItCannotDescribe`); `GetPlaybackSession` with a malformed
  stored ID answers 500 (`TestPlaybackSessionWithMalformedIDsIsAServerFault`).
- Request logs truncate long paths (M2-08): probe with a 5000-character path logged a cut path
  (about 200 characters plus ellipsis) and a cut parser error.

## Findings

None for M2-28. Minor, non-blocking observation: `session_limit_reached` is logged by
`sessionLimitReached` at warn level separately from `writeResponseError` (intentional, because the
response carries `Retry-After`); the log line has no cut path, but contains only IDs and the error
text of the service.

## History

- Round 1: PASS.
