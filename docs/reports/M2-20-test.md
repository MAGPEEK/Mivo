# M2-20 test report: playback session use cases (`internal/app/playback`)

Status: **PASS** (round 2)

Scope: the package `internal/app/playback` only (`ports.go`, `session.go`, `registry.go`,
`service.go`, `start.go` and the tests `fakes_test.go`, `fixtures_test.go`, `service_test.go`,
`start_test.go`, `concurrency_test.go`, `registry_internal_test.go`). Other wave-3 packages may be
mid-edit and were not judged. Probes ran through `go test -overlay` with a scratch file in the
session scratchpad; nothing was added to the repo and no production code or test was touched.

## Round-1 summary

Round 1 passed with 99.5 % coverage. Observation carried to round 2: the replaced session was
stopped before path resolution, keyframe lookup, workspace creation and producer start, so a late
failure lost the old session. Round 2 changed this (atomic replace).

## Gates (round 2)

| Gate | Command | Result |
|---|---|---|
| Vet | `go vet ./internal/app/playback/...` | clean, exit 0 |
| Tests, race | `go test -race -count=5 -coverprofile=... ./internal/app/playback/...` | `ok ... 2.212s coverage: 98.6% of statements` |
| Concurrency and replacement tests | `go test -race -count=30 -v -run 'Concurren\|AStartingSession\|Replacement\|Replaced\|ASessionBeing\|StoppingTheOld\|AFailedReplacement\|StartReplaces\|StartContinues'` | 12 tests x 30 runs, all `PASS`, no data race |
| Coverage gate | `go tool cover -func` | total 98.6 % (package gate 90 %). Every function 100 % except `Service.activate` 71.4 % (`start.go:163`, the "session vanished while starting" branch; only the registry half is tested, `registry_internal_test.go`) and `contentType` 75 % (`start.go:249`, unreachable `fallbackContentType` guard) |
| Lint | `golangci-lint run --allow-parallel-runners ./internal/app/playback/...` | `0 issues.` |
| Format | `golangci-lint fmt --diff ./internal/app/playback/...` | no diff, exit 0 |
| Layer rules | `go test -race -count=1 ./internal/archtest/...` | `ok` |
| Dependencies | `go list -deps ./internal/app/playback` | Mivo imports only `internal/domain`, `internal/domain/media`, `internal/domain/playback`; no `database/sql`, `net/http`, `os/exec`, no adapters |
| Domain purity | `go list -deps ./internal/domain/...` filtered for `database/sql\|net/http\|os/exec\|internal/adapters\|internal/app` | no match |
| TODO / panic / init | grep in package | none; one `//nolint:contextcheck` (`start.go:209`) with a reason (FFmpeg outlives the request, ADR 0024) |

Web, E2E, media corpus and Docker gates do not apply: Go app package with fakes only, no playback
media, no packaging.

## Acceptance criteria

1. **Ports (each 1-3 methods): met.** `ports.go`: `FileSource` (1), `PathResolver` (1),
   `KeyframeLocator` (1), `StreamProducer` (1) returning `Production{Ready, Stop, Done}` (3),
   `Workspace{Create, Remove}` (2), `StreamURLBuilder` (1), `IDGenerator` (1); the clock is
   `Dependencies.Clock func() time.Time`. The Production contract is documented (after `Stop`,
   `Ready` wraps `ErrProductionStopped`, which wraps `domain.ErrNotFound`; FFmpeg exit error;
   clean exit without a segment is an error). Probe `TestProbeConstructor` confirms
   `errors.Is(ErrProductionStopped, domain.ErrNotFound)`; repo test
   `TestWaitReadyAfterStopReportsAStoppedProduction`.
2. **`Start`: met.** Profile validated before the file is loaded
   (`TestStartRejectsAnInvalidProfileBeforeLoadingTheFile`). Unknown file wraps `ErrNotFound`, probe
   error wraps `ErrUnsupported`, `playback.Decide` runs with configured capabilities and policy
   (`TestStartFailures`, 13 cases). Every mode now resolves the path before a slot is reserved:
   `TestStartFailures` cases "direct-play file deleted" (`ErrNotFound`, `resolve movies/direct.mp4`) and
   "direct-play file escapes the library" (`ErrForbidden`). HLS modes create the workspace and start the
   producer (`TestStartHLSModesStartTheProducer`). Copy seeks use the keyframe, transcode and
   audio-only seeks start exactly (`TestStartSeeksCopiedVideoToAKeyframe`,
   `TestStartSeeksExactlyWithoutCopiedVideo`).
3. **`replacesSessionId` and limits (OD-4 A): met.**
   - Limits, probe `TestProbeOD4` (5x `-race`): two re-encoding sessions start; the 3rd transcode
     returns an error wrapping `ErrSessionLimitReached` and `domain.ErrUnavailable` carrying
     `session_limit_reached`; six remuxes then start (8 FFmpeg in total); the 9th remux and an
     audio-only start are rejected; 50 direct-play sessions still start; exactly 8 producer jobs and
     8 workspaces were created (rejected starts left nothing behind).
   - Overall cap `MaxSessions = 256` including direct play, probe `TestProbeOverallCap`: 256 direct-play
     sessions start; the 257th direct play and a remux both fail with `session_limit_reached` (no
     producer job, no workspace); an unknown `ReplacesSessionID` at the cap is rejected; a seek of an
     existing session at the cap succeeds (old gone, new visible, still exactly 256: the next start is
     rejected); `Stop` of one session frees exactly one slot. Probe `TestProbeOverallCapHLSSeekAndDoubleClaim`:
     255 direct + 1 remux, the remux seek succeeds at the cap, and a second start claiming the same session
     while the first replacement is in flight gets `session_limit_reached` (one claim per session). Repo test
     `TestStartRejectsSessionsAboveTheOverallCap` does the same for direct play.
   - Seek at the transcode limit with competition, probe `TestProbeSeekAtTranscodeLimitThirdClientGated`
     (limit 2, A and B transcoding, seek of A held inside `Producer.Start`): 10 concurrent third clients
     all get `session_limit_reached`; the seek then succeeds, A is gone, the new session is visible, a
     further third client is still rejected, and after `Stop(B)` a third client starts (4 producer jobs
     in total). Probe `TestProbeSeekRaceThirdClient` (300 iterations, seek and third client released
     together): the seek never lost, the third client always got 503-class `session_limit_reached`.
   - Atomic replace ordering, probe `TestProbeReplaceOrder`: while the new `Producer.Start` runs, the old
     production has 0 stops and `Get` and `Access` of the old session still work; after `Start` returns the
     old production has exactly 1 stop, `Get(old)` is `ErrNotFound`, the new session is visible and only the
     old workspace was removed.
   - Failed replacement keeps the old session, probe `TestProbeReplacementProducerFailureKeepsOldPlayable`:
     with A and B at the limit, a replacement of A whose producer start fails returns the producer error;
     `Access(A)` and `WaitReady` succeed, A's production has 0 stops and its `Done` is open; the
     failed session's own workspace is cleaned (3 created, 1 removed, not A's or B's); a third client is still
     rejected (A's slot is held); a retry of the same replacement then succeeds and replaces A, B stays.
     Probe `TestProbeFailuresKeepOld` (remux session, replacement attempts failing at unknown file, probe
     failed file, invalid profile, no compatible format, start beyond the end, resolver error,
     workspace creation, producer start, ID generator): the old session is found, has 0 stops and its
     workspace is never removed in each case. Repo tests: `TestAFailedReplacementKeepsTheOldSession`,
     `TestStartKeepsTheReplacedSessionWhenTheDecisionFails`,
     `TestStartContinuesWhenTheReplacedWorkspaceCannotBeRemoved`, `TestASessionBeingReplacedCountsOnlyOnce`,
     `TestStoppingTheOldSessionDuringItsReplacement`, `TestAReplacementHoldsTheSlotAtomicallyWhileItStarts`.
4. **URL builder, direct play without workspace/producer: met.** `StreamURL` from the injected
   `StreamURLBuilder`; `TestStartDirectPlayNeedsNoFFmpeg` asserts 0 producer jobs, 0 workspaces,
   0 keyframe lookups.
5. **`Get`, `Stop` (idempotent), `Access`: met.** `TestStopEndsTheSession`, `TestStopIsIdempotent`, probe
   `TestProbeStopConcurrent` (50 concurrent `Stop`: exactly 1 production stop, 1 removal),
   `TestStopRemovesTheWorkspaceEvenWhenTheRequestIsCancelled`, `TestAccessRecordsTheAccessTime`
   (`Get` is not an access). Stop with a failing workspace removal, probe `TestProbeStopWorkspaceFailure`:
   `Stop` returns nil, one `level=WARN` line naming "workspace is left behind", the session ID and
   `directory locked`; the production was stopped once, `Get` is `ErrNotFound`, a second `Stop` returns
   nil without a second removal, the slot is free again, and `Stop` with an already cancelled context
   returns nil. Repo test `TestStopLogsALeftoverWorkspaceAndSucceeds`.
6. **FFmpeg under a service-owned context: met.** `TestStartRunsTheProductionUnderTheServiceContext`
   (now asserts no stop and an open `Done` after cancelling the request) and probe
   `TestProbeCancelledRequest`: job context `Err() == nil`, 0 stops, no workspace removal, `Get` finds the
   session.
7. **Tests with fakes: met.** Each mode, copy vs transcode seek, limit reached (transcode, FFmpeg, overall),
   replace under the limit, idempotent stop, producer failure cleans the workspace
   (`TestStartCleansTheWorkspaceWhenTheProducerFails`), unknown file, probe error, duplicate ID is an
   internal error and wraps none of the domain sentinels
   (`TestStartReportsADuplicateSessionIDAsAnInternalError`), concurrent starts under `-race`
   (`TestConcurrentStartsNeverExceedTheTranscodeLimit`, `TestConcurrentRemuxStartsStopAtTheFFmpegCap`,
   `TestConcurrentStartAccessGetAndStop`). Coverage 98.6 % (>= 90 %).
8. **Gates: met.** See the table above.

## Orchestrator notes

- **Keyframe lookup error falls back to 0 with a warning: met.** Probe `TestProbeKeyframe`: 30 s copy-video
  seek starts at the keyframe 20 s; after a lookup error the start is 0 and a `level=WARN` line is logged;
  position 0 does no lookup. Repo tests `TestStartFallsBackToTheBeginningWhenTheKeyframeLookupFails`,
  `TestStartGivesUpWhenTheRequestEndsDuringTheKeyframeLookup` (request cancel aborts the start and frees the
  slot), `TestStartAtTheBeginningLooksUpNoKeyframe`.
- **Keyframe positions relative to the format start: met** (port doc; the service passes the value through).
- **`Policy.Validate()` in the constructor and limit 1-8: met.** Probe `TestProbeConstructor`: limits -1, 0,
  9, 100 and heights -1, 1, 241, 1081 return `ErrInvalid`; limits 1, 2, 8 are accepted.
- **Starting sessions invisible until FFmpeg runs: met.** `TestAStartingSessionIsInvisibleButHoldsItsSlot`;
  probe `TestProbePendingReplace`: a start naming a still-pending session as replaced does not stop it, a
  `Stop` of the pending ID is ignored, and the session becomes visible with 0 stops once its producer
  returns.
- **No timers, seam for M2-25: met.** `registry` is its own type with a mutex and never calls ports under
  the lock; access times are recorded; no idle reaper.
- Function size, parameters and complexity: lint is clean with the project configuration.

## Observations (no action required for PASS)

- `Service.activate` (`start.go:163-171`) branch for a pending session that vanished during start has no
  test through the service; `registry.activate` returning `found=false` is tested internally
  (`TestRegistryActivateReportsAVanishedSession`). Reachable only after M2-25 adds `Close`; add a service
  level test there.
- `Service.processContext` is `context.Background()` until `Close` lands in M2-25 (documented).
- `contentType` fallback branch (`start.go:249`) uncovered by design.
- Probe defect during this run (not a product defect): my first `TestProbeFailuresKeepOld` assertion
  expected no workspace removal at all; in the producer-failure case the new session's own workspace is
  correctly removed. The assertion was corrected to check that the old session's workspace is untouched.

## History

- Round 1: PASS. All gates green, 99.5 % coverage, scratch probes (OD-4 limits, replace ordering,
  cancelled request, concurrent stop, constructor bounds, pending sessions, keyframe fallback) passed.
- Round 2: PASS. Atomic replace, Production contract, overall 256 cap, early path resolution, Stop with
  leftover workspace, duplicate ID as internal error. All gates green at 98.6 % coverage; round-1 probes and
  the new probes (seek at the limit against competing clients, failed replacement, 256 plus one, Stop with
  a failing removal) passed 5x under `-race`; replacement and concurrency tests passed 30x.

REPORT
status: PASS
task: M2-20
files: docs/reports/M2-20-test.md
tests: go vet clean; go test -race -count=5 ./internal/app/playback/... ok, 98.6 % coverage; concurrency/replacement tests (12) -count=30 pass; golangci-lint run 0 issues, fmt --diff clean; archtest ok; go list -deps shows only internal/domain/** imports, domain has no forbidden imports; overlay probes (17 tests incl. 10 failure cases) passed 5x under -race, seek race probe 300 iterations
findings:
- none blocking
- observation: Service.activate vanished-session branch (start.go:163) is 71.4 % covered, no service-level test until M2-25 Close exists
- observation: processContext is context.Background() until Close lands in M2-25
- observation: contentType fallback branch (start.go:249) uncovered by design
next: hand over to mivo-reviewer
