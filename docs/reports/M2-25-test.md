# M2-25 test report – session lifecycle (state, idle timeout, purge, shutdown)

Status: **FAIL** (round 2, tests only – no production defect found)
Tested together with M2-29 in a clean worktree (detached at 5e64b56 plus the 29 files of
`m2-25-29-files.txt`); the worktree is removed again.

Why FAIL: the mutation check demanded for this round found two guards that no test pins
(F1, F2). Every acceptance criterion and every behavioural probe is met.

## Gates

| Gate | Result |
|---|---|
| `go build ./...`, `go vet ./...` | clean |
| `go test -coverprofile` ./... (Windows, no cgo) | all packages ok |
| go-test-coverage (`.testcoverage.yml`) | total 97.0 % (4540/4682), threshold 70 % met, domain >= 90 % met |
| coverage `internal/app/playback` | 98.6 % (AC: >= 90 %) |
| `golangci-lint run --allow-parallel-runners` | 0 issues |
| `golangci-lint fmt --diff` | clean |
| `go mod tidy -diff`, `go -C tools mod tidy -diff` | clean |
| archtest (`internal/archtest`) | ok |
| layer check `go list -deps` | `internal/domain/...` imports none of database/sql, net/http, os/exec, adapters, app; `internal/app/playback` imports no adapter and no net/http |
| Linux race (Docker golang:1.27.3) `go test -race ./...` | all ok |
| Flake hunt `go test -race -count=20 ./internal/app/playback/... ./internal/adapters/http/stream/...` | ok (rc 0) |
| Probe suite under `-race -count=30` (Docker) | ok |

Note: `-race` needs cgo; the Windows machine has no C toolchain, so race runs are Docker only.

## Acceptance criteria

1. State follows the production (starting, ready on first segment, ended on clean exit, failed with
   `transcode_failed`, direct play ready at once): MET.
   `TestAnHLSSessionBecomesReadyWithItsFirstSegment`, `...EndsWithACleanExit`,
   `TestAFailedProductionFailsTheSession`, `TestADirectPlaySessionIsReadyAtOnce`,
   `TestAStoppedProductionIsNotAFailure`. Mutations `follow-ready`, `follow-ended`, `failurecode`
   each fail tests.
2. Idle reaper (goroutine, named interval, 2 min timeout constant, clock port, no sleeping): MET.
   `IdleTimeout = 2*time.Minute`, `IdleCheckInterval = 15s` (`reaper.go:13,16`); tests use
   `synctest` and the injected `After`. Probe: alive at 105 s and 115 s, gone at 120 s (HLS and
   direct).
3. `PurgeWorkspace` calls `Purge` once before the first session, logs the count: MET.
   `TestPurgeWorkspaceRemovesLeftoversOnce` (`removed=3` at info). Probe: after a first session
   (also after a failed start and after Stop) the purge returns `domain.ErrConflict`; during a
   purge new sessions get `ErrUnavailable`. Observation (not a defect): two sequential purges
   before any session both run `Purge`; the guard only forbids concurrent purges and purges after
   a session.
4. `Close`: refuses new sessions with an `ErrUnavailable` wrap, stops sessions in parallel, waits
   for productions and workspace removal bounded by ctx, stops the reaper, second `Close` is a
   no-op: MET. `ErrServiceClosed` wraps `domain.ErrUnavailable` (probe: direct, remux and
   transcode Start after Close all match both). `TestCloseWaitsForSlowStopsRunningInParallel`,
   `TestCloseGivesUpWhenItsContextEnds`, `TestASecondCloseIsANoOp`, `TestCloseWaitsForARunningPurge`.
5. Tests with fakes and fake clock (idle expiry, access keeps alive, ended/failed, close during
   start, close waits for slow stop, no goroutine left): MET (see test names in 1-4;
   `TestCloseLeavesNoGoroutineBehind` runs in a synctest bubble). Coverage 98.6 %.
6. Gates (lint, layer rules, race, coverage gate): MET, see table.

Round 2 additions:
- In-flight tracking via `StreamAccess.Release`: MET. Probe: an open access survives 10 min of
  reaper ticks, is reaped 2m8s after an off-tick Release (window 120-135 s); double Release and
  Release of copies count once; Release after Stop/Close and on the zero value is harmless;
  a replacement still stops the old session with an open access.
- `ErrServiceClosed` on start/close races: MET (`TestAStartRacingCloseReportsTheShutdown`,
  `TestCloseDuringAStartTearsTheNewProductionDown`).
- Close waits for a running purge: MET (probe and `TestCloseWaitsForARunningPurge`).
- Failed sessions logged at info only: MET (`lifecycle.go:62`, test asserts no WARN from service).
- Orchestrator notes from M2-20: process context cancellable in Close (yes); `activate` vanished
  branch tested; union counting transcode to remux tested
  (`TestALowerClassReplacementKeepsTheOldSessionsSlotCounted`, probe with limit 2 passes);
  Access(old id) during a swap tested; ADR 0024 notes (direct-play cap) added.

## Mutation check (one guard removed at a time, `go test ./internal/app/playback`)

Killed: in-flight skip, pending skip, boundary (alive at 120 s), boundary (reaped early), reencode
slot accounting, Release once, in-flight increment, in-flight clamp, idle restart on end, purge
holder, purge `used`, failure code, reaper interval, 90 s timeout, activate-closed error, reserve
while closed, second-Close guard, Close without teardown, Close without wait, parallel teardown,
Close without cancel, workspace removal detached from ctx.

## Failures (surviving mutants)

F1. Close ordering is not pinned. `internal/app/playback/lifecycle.go:87-88`: moving
`s.cancelProcesses()` before `s.teardownAll(ctx, sessions)` leaves the whole package green
(`ok  .../app/playback`). The comment at line 86 and ADR 0024 (Notes M2-25, "Stop first, so a kill
is not reported as a crash") make the order a documented guarantee: with the real producer a
context kill reports an exit error instead of `ErrProductionStopped`. Fix: a test where the fake
production's `Stop` records `job.ctx.Err()` (the context is already in `startedJob.ctx`) and
asserts it is nil at Stop time during `Close`.

F2. FFmpeg-cap slot accounting is not pinned. `internal/app/playback/registry.go:98`: changing
`existing.runsFFmpeg && (claimant == nil || !claimant.runsFFmpeg)` to
`existing.runsFFmpeg && claimant == nil` leaves all tests green (the sibling re-encoding branch at
line 94 is pinned). Observable effect: an HLS session replaced by a direct-play session stops
counting against `MaxFFmpegSessions` while its FFmpeg still runs. Fix: a test with the FFmpeg cap
reached (8 remux sessions), one replaced by a direct-play start held at the gate, and a further
remux start that must get `ErrSessionLimitReached`. Alternatively, if this case is declared out of
scope, simplify the code and the ADR note.

Not findings (equivalent or unreachable): `track()` refusal after close for the watcher (needs
Close between `activate` and `follow`), returning pending entries from `close()` (their teardown
is a no-op).

## History

- Round 1: earlier report superseded.
- Round 2: this run, FAIL for F1 and F2 (tests only).

REPORT
status: FAIL
task: M2-25
files: docs/reports/M2-25-test.md
tests: go vet/build clean; go test -coverprofile ./... ok, total 97.0 %, playback 98.6 %; golangci-lint 0 issues; fmt/tidy clean; archtest ok; Docker race ./... ok; race -count=20 playback+stream ok; probes ok; mutation check 2 survivors
findings:
- F1 lifecycle.go:87-88 Close order (Stop before cancel) not covered by any test
- F2 registry.go:98 FFmpeg-cap claimant accounting not covered by any test
next: mivo-media adds two tests (no production change needed), then re-test
