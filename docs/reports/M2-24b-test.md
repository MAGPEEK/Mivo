# M2-24b test report: FFmpeg hardening

Status: **PASS** (round 3; the round 1 report is first, the round 2 section starts at "# Round 2", the round 3 section at "# Round 3")

Tester: mivo-tester. Isolation: clean worktree at `548a12d` plus the 21 files of
`scratchpad/m2-24b-files.txt` (byte-identical to the main tree, re-verified after the usage
pause) plus a copy of `testdata/media`. The worktree is removed after this report.
Environments: Windows 11 (Go 1.27.3, FFmpeg 9.0.2 gyan full build) and Linux
(golang:1.27.3 container, FFmpeg 7.1.5-0+deb13u1 installed with apt).

## Blocking finding

### F1 Windows: `adopt` fails a start for a process that is just ending (flaky, reproduced twice)

`internal/adapters/ffmpeg/lifetime_windows.go:63`

```
--- FAIL: TestProbeCorpusMatchesManifest/video-flv-h264-aac (0.15s)
    probe_corpus_test.go:35: Probe(video/h264_aac.flv): probe h264_aac.flv: start ffprobe:
    unavailable: assign process 56940 to its job object: Access is denied.
```

Seen once in the first `go test -coverprofile ... ./...` run (all packages in parallel).
Reproduced with a throwaway stress test (since deleted): 16 goroutines x 150 `tool.output(ffprobe -version)`
= 2400 starts gave 1 failure `assign process 37648 to its job object: Access is denied.`; two
repeats of 2400 gave 0. So roughly 1 in 5000 starts of a quickly exiting process, more likely
under load. The ordinary suite passed 40+ times without it (`-count=10`, `-count=20`,
6 x `./...`), so it is rare but real.

Cause (likely): `AssignProcessToJobObject` returns ACCESS_DENIED for a process that has called
ExitProcess but whose handle is not signalled yet. The guard `!hasEnded(handle)` checks the
signal once, with timeout 0, and so rejects that window. The result is a spurious
`domain.ErrUnavailable` for a probe (scan marks the file failed) or a producer start (user sees
a playback error) although ffprobe/ffmpeg ran fine. `TestAdoptAcceptsAProcessThatAlreadyEnded`
only covers a fully ended process, not the terminating one.

Suggested fix for the implementer: when the assignment fails, give the process a short bounded
grace to finish (for example `WaitForSingleObject(handle, ~100 ms)` or poll
`GetExitCodeProcess != STILL_ACTIVE`) before treating the failure as real, and add a stress test
(many short-lived starts) so the flake is caught.
ADR 0024 Notes ("a tool that already ended by then needs no job") should then mention the
terminating state.

No other gate failed. Everything below passed; F1 alone makes the round FAIL ("repeat -count=10
to catch flakes (Job Object assignment race)").

## Gates

| Gate | Result | Evidence |
|---|---|---|
| `go build ./...` | PASS | rc=0 |
| `go vet ./...` | PASS | rc=0 |
| `go vet` GOOS=linux, GOOS=darwin `./internal/adapters/ffmpeg/...` | PASS | rc=0 both |
| `golangci-lint run --allow-parallel-runners` | PASS | `0 issues.` |
| same with GOOS=linux | PASS | `0 issues.` |
| `golangci-lint fmt --diff` | PASS | empty |
| `go mod tidy -diff`, `go -C tools mod tidy -diff` | PASS | empty; go.mod/go.sum untouched |
| `go test -coverprofile ./...` (Windows) | PASS on the second run, FAIL on the first (F1) | all packages ok |
| go-test-coverage (`.testcoverage.yml`) | PASS | total 96.8 % (4800/4958), threshold 70 %; `internal/domain/...` 100 % in every package; ffmpeg package 95.5 % |
| archtest (layer matrix, depguard) | PASS | `TestRepositoryFollowsLayerMatrix`, `TestDepguardDeniesEveryStandardPackageOfTheMatrix` |
| Linux `go test -race -count=1 ./...` (Docker, FFmpeg 7.1.5) | PASS | every package ok, ffmpeg 28.9 s |
| Linux ffmpeg package `-race -count=1 -v` | PASS | 323 PASS, 0 FAIL; only SKIPs are two Windows-device-path tests (Windows only by nature) |
| Linux ffmpeg package `-count=10` | PASS | ok 98.6 s |
| Linux ffmpeg package `-race -count=5` | PASS | ok 138.6 s |
| Windows ffmpeg package `-count=3`, `-count=10` | PASS | ok 32.6 s, ok 102.5 s |
| Windows ffmpeg package `-v` | PASS | 322 PASS, 0 FAIL; the only SKIP is `TestProducerStartFailures/input_missing_in_the_library` (`linuxOnly` unit case with a fake tool, not an integration test); no corpus or FFmpeg integration test skipped on either system |
| FFmpeg left over after the runs | PASS | `Get-Process ffmpeg,ffprobe` empty; `ps` in the container shows none |
| Web gates, Docker build, E2E | not applicable | task touches only `internal/adapters/ffmpeg` and docs |

## Acceptance criteria

1. **Demuxer restriction on every invocation** - MET. `-format_whitelist` is built in
   `inputArguments` (probe.go:151-165), which probe (probe.go:119), keyframe lookup
   (keyframe.go:49) and the producer (`ffmpegInputArguments`, hlsargs.go:148) all use; there is
   no other `-i`/input site. `-protocol_whitelist file` stays. image2 only for paths without
   `%`. The choice is justified in ADR 0022 Notes M2-24b (including why not `-f`).
   Corpus: `TestProbeCorpusMatchesManifest` 45/45 generated cases pass on Windows (9.0.2) and
   Linux (7.1.5). Independent before/after comparison with plain ffprobe: 66 files (48
   corpus files plus 18 extra containers I generated: mpg, vob, wmv, ogv, 3gp, 3g2, mts, m2ts, mk3d,
   divx, m4v, mov, webm, asf, flv, rm, rmvb, mxf) give byte-identical JSON with and without the whitelist
   (the mxf sample may be empty, so mxf is only weakly covered; it is on the list). Every scanner
   extension (`IsVideoFileName`) maps to a whitelisted demuxer.
2. **Hostile inputs refused** - MET. `TestHostileInputsNeverOpenOtherFiles` (HLS playlist as
   `evil.mkv`, HLS as `evil.m3u8`, ffconcat list as `evil.mkv`) and
   `TestProbeRefusesImageSequencePatterns` (`frame%d.jpg`; an unrestricted ffprobe reads 3
   frames, the protected one is refused, `frame1.jpg` still probes) pass on both systems.
   The tests are not vacuous: the control run shows the unrestricted ffprobe follows the
   reference (`true` for ffconcat and `.m3u8`, `false` for an HLS playlist named `.mkv`, which
   FFmpeg refuses on its own), and the protected probe, keyframe lookup and producer fail with
   FFmpeg's "not on whitelist" error, i.e. before the canary is opened; the refused production
   writes no output.
3. **Children die with the server** - MET for the mechanism, but see F1 for Windows start
   reliability. `TestChildDiesWithItsParent` (the parent is killed without cleanup, then the
   child must be gone): Linux in Docker 5/5 PASS, Windows 10/10 PASS. Mutation check in the
   scratch worktree: removing `Pdeathsig` makes the Linux test fail ("the child 183 still runs
   30s after its parent was killed"), so the test has teeth. No processes left over
   afterwards. Linux locking: the producer starts and waits for FFmpeg on its supervising
   goroutine (producer.go `run`); `-race` is clean. Job object uses
   `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` with `golang.org/x/sys` only (no new module).
4. **`ErrorOutput` doc** - MET (process.go): the tail may be logged, the producer logs it once
   at warn level per ADR 0024, it is never returned to clients, and it can contain absolute
   media paths and bytes from the file.
5. **Windows rename-over under a 1 ms poll** - MET.
   `TestProducerRenamesThePlaylistWhileReadyPolls` (20 remuxes of the 30 s clip per test, Ready
   at 1 ms plus a reader loop, about 45 playlist opens per run) run with `-count=20`, i.e. 400
   remuxes: 20/20 PASS, every final playlist ends with `#EXT-X-ENDLIST` and has at least 5
   segments, no `index.m3u8.tmp` remains, no warn log. Mutation check (the `completePlaylist`
   call disabled in the scratch copy): the test fails at run 3 ("the temporary playlist is
   left", playlist incomplete), so the fix is needed and the test detects its absence. Also passes
   on Linux; `TestProducerCompletesALostFinalPlaylistRename` and `TestCompletePlaylist`
   (including 1 s of failing retries) pass.
6. **Nits** - MET. `exit_code` is logged only for real exits (`exitCode()` returns 0 only for
   a clean exit without segments); `stopKillGrace` names the 5 s; the header-only Ready test
   waits for `index.m3u8` before its not-ready deadline (producer_internal_test.go).
7. **Gates** - NOT MET because of F1 (lint, layer rules, Linux race, Linux and Windows
   corpus integration, coverage are all green otherwise).

## Notes (non-blocking)

- Documented known gap, not a failure: the tool runs briefly before it joins its job on
  Windows (BACKLOG entry added by the implementer). Related to F1 but a separate issue.
- Docker Desktop stopped during the pause and was restarted by the tester; the image
  `mivo-qa-ffmpeg` (golang:1.27.3 + apt ffmpeg 7.1.5 + procps) was built for the Linux runs.

## History

- Round 1: FAIL (F1, Windows job assignment race). All other gates and criteria green.
- Round 2: FAIL (F2 job object configuration fails ~1 in 75000 starts, F3 flaky TestProducerStopGivesUpAfterTheStopTimeout). F1 fix reviewed and plausible, not provably exercised. All other gates green.

# Round 2 (re-test after the F1 fix)

Status: **FAIL**. F1 is addressed, but two new blocking findings: F2 (job object
configuration fails sporadically on Windows, found by the new 5000-start stress) and F3 (a flaky
unit test, pre-existing in M2-24, fails 4 to 9 % of runs in isolation and failed 1 of 3 full
`go test ./...` runs).

Isolation: clean worktree `wt24c` at `f91ffbc` plus the 25 files of `scratchpad/m2-24b-files.txt`
(SHA-256 identical to the main tree after copying) plus a copy of `testdata/media` (49 files).
Mutation and throwaway tests ran only in that worktree; production code and tests in the main tree
are untouched. The worktree is removed. Note: the main tree now also holds unrelated changes from
another session (docs/adr 0002/0014/0026, docs/plan.md, docs/tasks/SPK-*); they are not part of
M2-24b and were not tested. Environments: Windows 11 (Go 1.27.3, FFmpeg 9.0.2) and Linux (image
`mivo-qa-ffmpeg`: golang:1.27.3, FFmpeg 7.1.5-0+deb13u1).

## Blocking findings

### F2 Windows: `bindChildLifetime` fails a start with "configure job object: The parameter is incorrect." (~1 in 75000 starts)

`internal/adapters/ffmpeg/lifetime_windows.go:58-59`

```
process_stress_internal_test.go:48: first failed start: start fakeprobe: unavailable:
    configure job object: The parameter is incorrect.
process_stress_internal_test.go:57: 1 of 5000 starts failed
--- FAIL: TestManyShortLivedStartsSucceed (7.07s)
```

Reproduction on the unmodified code with the new test: `MIVO_FFMPEG_STRESS_STARTS=5000`, test
binary built with `go test -c`, run 15 times: 14 PASS, 1 FAIL (run 3 of the second series; the control runs are described below).
That is 1 failure in 75000 starts on the unmodified code, plus 1 in 20000 starts in the 4 control runs
that only changed the F1 guard (2 in 95000 overall, about 1 in 50000). The requested 3x5000 passed by
chance (15000 starts); 15 runs were needed to see it. At the default 1000
starts of the suite this is a roughly 1 to 2 % flake per `go test` run of the package, and every
production start (probe, keyframe, producer) can hit it: a spurious `domain.ErrUnavailable`.

Cause (demonstrated, not guessed): `limits` is a stack variable whose address is handed over as
`uintptr(unsafe.Pointer(&limits))` to `windows.SetInformationJobObject`, a wrapper that is not
marked `//go:uintptrescapes`. When the stack grows inside that call the runtime copies the stack
and the uintptr points at the old, freed stack memory, which then holds garbage (or a stale copy:
a silent success with `LimitFlags` lost would mean no `KILL_ON_JOB_CLOSE`, the very guarantee of the
task). Evidence from a throwaway test in the scratch worktree that calls only `bindChildLifetime`
in 16 goroutines at varying stack depths:

| Variant | Failures |
|---|---|
| unchanged code, no GC pressure | 0 of 3 200 000 (stack frames are rarely reused) |
| unchanged code, a goroutine looping `runtime.GC()` and 4 allocating goroutines | 2 of 3 200 000, then 2 and 1 in two more runs; always at stack depth 13 (the growth boundary) |
| `limits` moved to the heap (assigned to a package variable) | 0 of 12 800 000 in four runs |

Suggested fix: pass the structure through a function annotated `//go:uintptrescapes` (or build
it on the heap so it escapes, keep it alive with `runtime.KeepAlive`, and assert in the test that
the flags were set, for example with `QueryInformationJobObject`). Add a test that queries the
job's limit flags after `bindChildLifetime`, so a lost `KILL_ON_JOB_CLOSE` is caught, and keep the
GC-pressure stress as a regression test. (`lifetime_windows.go:59` also carries a
`//nolint:gosec // see above` for this conversion, so the lint exception is the place where this
risk was accepted.)

### F3 Flaky `TestProducerStopGivesUpAfterTheStopTimeout`

`internal/adapters/ffmpeg/producer_internal_test.go:666-679` (fails at line 677)

```
--- FAIL: TestProducerStopGivesUpAfterTheStopTimeout (0.01s)
    producer_internal_test.go:677: warnings = [], want one about the slow exit
```

Seen in 1 of 3 full `go test -count=1 ./...` runs on Windows (ffmpeg package 11.9 s). Isolated:
`-test.count=500` on the compiled test binary gives 46 failures (9 %) with the M2-24b tree and 20
failures (4 %, line 478 there) at the clean HEAD `f91ffbc`, so the flake exists in the committed
M2-24 code and the M2-24b changes did not cure it. Round 1 missed it (about 50 runs without it).
Cause: the test sets `stopTimeout = time.Nanosecond` and assumes the process is not done when
`Stop` selects (producer.go:269-277: `select` over `p.done` and a 1 ns timer chooses randomly when
both are ready, and `p.done` is already closed when the kill and reaping are quick). Not a
Windows-only problem in principle; on Linux the five runs of `-race -count=3` did not hit it.
Suggested fix: make the test deterministic, for example a fake mode that ignores the kill until the
test releases it (so `done` cannot be ready), or give `production` a hook to observe the timer path.

## Gates (round 2)

| Gate | Result | Evidence |
|---|---|---|
| `go build ./...`, `go vet ./...` | PASS | rc=0 |
| `go vet` and `go build` GOOS=linux, GOOS=darwin `./internal/adapters/ffmpeg/...` | PASS | rc=0 all four |
| `golangci-lint run --allow-parallel-runners` | PASS | `0 issues.` |
| same with GOOS=linux | PASS | `0 issues.` |
| `golangci-lint fmt --diff` | PASS | empty |
| `go mod tidy -diff`, `go -C tools mod tidy -diff` | PASS | empty |
| `go test -race -coverprofile ./...` (Windows, cgo works) | PASS | all packages ok, ffmpeg 97.2 s, 95.6 % |
| go-test-coverage | PASS | total 96.9 % (5088/5252), threshold 70 %; `internal/domain/...` 100 % in each package |
| archtest | PASS | `TestRepositoryFollowsLayerMatrix`, `TestDepguardDeniesEveryStandardPackageOfTheMatrix` |
| Linux `go test -race -count=1 ./...` (Docker, FFmpeg 7.1.5) | PASS | every package ok, ffmpeg 35.6 s |
| Linux ffmpeg package `-race -count=3 -v` | PASS | 243 PASS lines at top level, 0 FAIL, ffmpeg 104.4 s; SKIP only the two Windows-device-path tests; `TestManyShortLivedStartsSucceed` (100 starts, 7.2 s) and `TestAbandonKillsAndReapsTheProcess` pass 3x; no `ffmpeg`/`ffprobe`/`.test` left in the container |
| Linux `TestChildDiesWithItsParent`, `TestHostile*`, `TestProbeRefusesImageSequencePatterns`, `TestProbeCorpusMatchesManifest`, rename test `-race -count=5` | PASS | all PASS |
| Windows `MIVO_FFMPEG_STRESS_STARTS=5000` x3 (as requested) | PASS | 3 of 3, 6.6 to 6.7 s each, together with `TestAdoptAcceptsAProcessThatAlreadyEnded` and `TestAbandon...` |
| Windows `MIVO_FFMPEG_STRESS_STARTS=5000`, 12 further runs | FAIL | 11 PASS, 1 FAIL (F2) |
| Windows ffmpeg package `-count=10` | PASS | ok 102.1 s |
| Windows full `go test -count=1 ./...` x3 | FAIL | run 1 PASS, run 2 FAIL (F3), run 3 PASS |
| Windows ffmpeg package `-v` (324 PASS) | PASS | only SKIP `TestProducerStartFailures/input_missing_in_the_library` (`linuxOnly` fake-tool unit case); no corpus or FFmpeg integration test skipped |
| Left-over processes | PASS | no ffmpeg, ffprobe, `*.test`, scratch exe; the only container is a pre-existing `buildx_buildkit_mivo-release0` |
| Worktrees | PASS | `wt24c` and `wt24base` removed with `git worktree remove --force` (`Remove-Item \\?\...` is blocked in this environment), `git worktree prune`, `git worktree list` shows only the main tree |
| Web, Docker build, E2E | not applicable | the task touches only `internal/adapters/ffmpeg` and docs |

## Round 1 results re-checked

- Hostile inputs refused: `TestHostileInputsNeverOpenOtherFiles` (HLS playlist, ffconcat list, `evil.m3u8`; three subtests) and `TestProbeRefusesImageSequencePatterns` pass 10x on Windows and 5x on Linux (-race).
- Orphan child: `TestChildDiesWithItsParent` 10/10 on Windows, 5/5 on Linux (-race); in the real runs the child is gone, nothing left.
- Rename stress: `TestProducerRenamesThePlaylistWhileReadyPolls` and `TestProducerCompletesALostFinalPlaylistRename` `-count=20` PASS on Windows, 5x on Linux.
- Whole corpus probes identical: a throwaway test compared plain `ffprobe -protocol_whitelist file` JSON with the package's `inputArguments` for all 49 files under `testdata/media`: 48 media files byte-identical, 0 different; the 49th is `manifest.json` (not media, refused as expected). `TestProbeCorpusMatchesManifest` passes in every run above.

## Acceptance criteria (round 2)

1. Demuxer restriction on every invocation - MET (unchanged since round 1, see below; corpus identical).
2. Hostile inputs refused - MET.
3. Children die with the server - NOT MET for Windows reliability: the Job Object configuration fails in about 1 of 75000 starts (F2) and, if the stack-copy cause is right, can silently lose `KILL_ON_JOB_CLOSE`. Mechanism tests pass (Linux and Windows).
4. `ErrorOutput` doc - MET.
5. Windows rename-over under 1 ms poll - MET.
6. Nits - MET.
7. Gates - NOT MET: F2 and F3 (flaky full suite on Windows).

F1 itself: the new code accepts ACCESS_DENIED only for a process whose exit code is set or whose
handle is signalled within 100 ms. In this round 19 200 real `ffprobe -version` starts (16
goroutines, scratch test) with the fix gave 0 failures, but the control with the old behaviour
(grace 0, no exit-code check) also gave 0 of 19 200, so F1 could not be re-triggered here and its
fix is verified by reading and by the unit test for an already ended process only. The default
`TestManyShortLivedStartsSucceed` uses a Go helper that exits instantly, not ffprobe, and in the
control it did not trigger F1 either (1 failure in 20000 starts was F2, not F1).

## What to fix for round 3

1. F2: heap-allocated or `//go:uintptrescapes`-passed `JOBOBJECT_EXTENDED_LIMIT_INFORMATION`; test that the limit flags are set; keep a GC-pressure stress.
2. F3: make `TestProducerStopGivesUpAfterTheStopTimeout` deterministic (this is committed M2-24 code; the fix is small).
3. Then rerun: Windows `MIVO_FFMPEG_STRESS_STARTS=5000` at least 15 times, `go test -count=1 ./...` at least 5 times, ffmpeg package `-count=50` for the F3 test.

# Round 3 (re-test after the F2 and F3 fixes)

Status: **PASS**. F2 and F3 are fixed and proven by mutation; all gates are green; no flake in
any of the repeated runs.

Isolation: clean worktree `wt24d` at `d329b5d` plus the 25 files of `scratchpad/m2-24b-files.txt`
(SHA-256 identical to the main tree before and after the tests) plus a copy of `testdata/media`
(49 files). Mutation and throwaway tests ran only there; production code and tests in the main
tree are untouched. The worktree is removed (`git worktree remove --force`, `git worktree prune`;
`git worktree list` shows only the main tree). The main tree also holds unrelated in-progress work
(`packaging/synology/**`, `docs/tasks/BACKLOG.md`); it is not part of this test. Environments:
Windows 11 (Go 1.27.3, FFmpeg 9.0.2) and Linux (image `mivo-qa-ffmpeg`: golang:1.27.3, FFmpeg
7.1.5-0+deb13u1).

## Gates (round 3)

| Gate | Result | Evidence |
|---|---|---|
| `go build ./...`, `go vet ./...` | PASS | rc=0 |
| `go vet` and `go build` GOOS=linux, GOOS=darwin `./internal/adapters/ffmpeg/...` | PASS | rc=0 all four |
| `golangci-lint run --allow-parallel-runners` | PASS | `0 issues.` |
| same with GOOS=linux | PASS | `0 issues.` |
| `golangci-lint fmt --diff` | PASS | empty |
| `go mod tidy -diff`, `go -C tools mod tidy -diff` | PASS | empty, rc=0 |
| `go test -race -coverprofile ./...` (Windows) | PASS | all packages ok; ffmpeg 102.2 s, 95.4 % |
| go-test-coverage (`.testcoverage.yml`) | PASS | total 96.9 % (5090/5255), threshold 70 %; `internal/domain/...` 100 % in each package |
| archtest | PASS | `internal/archtest` ok (layer matrix, depguard) in the Windows and the Linux run |
| Windows `MIVO_FFMPEG_JOB_STRESS_BINDS=5000000`, 4 runs | PASS | 4 of 4 (9.5, 6.6, 7.6, 11.8 s), `TestBindChildLifetimeSetsKillOnJobClose` also PASS |
| Windows `MIVO_FFMPEG_STRESS_STARTS=5000`, 30 runs (15 together with `TestAdoptAcceptsAProcessThatAlreadyEnded` and `TestAbandonKillsAndReapsTheProcess`, 15 more alone) | PASS | 30 of 30, i.e. 150 000 starts, 0 failures (round 2 saw 1 failure in about 75 000) |
| Windows `TestProducerStopGivesUpAfterTheStopTimeout -count=200` | PASS | 200/200 normal, 200/200 with `-race` (round 2: 4 to 9 % failed) |
| Windows full `go test -count=1 ./...` x5 | PASS | 5 of 5, 34 packages ok each, no FAIL |
| Windows ffmpeg package `-count=10 -v` | PASS | ok 128.0 s, 860 top-level PASS, 0 FAIL; the only SKIP is `TestProducerStartFailures/input_missing_in_the_library` (`linuxOnly` fake-tool unit case), no corpus or FFmpeg integration test skipped |
| Linux `go test -race -count=1 ./...` (Docker, FFmpeg 7.1.5) | PASS | every package ok, ffmpeg 36.0 s |
| Linux ffmpeg package `-race -count=3 -v` | PASS | ok 107.2 s, 243 PASS, 0 FAIL; SKIP only `TestHLSArgumentsRejectsWindowsDeviceOutputDirectories` and `TestProbeRejectsWindowsDevicePaths` (Windows device paths, Windows only by nature); no integration skip |
| Linux F3 test `-race -count=200` | PASS | 200/200 |
| Left-over processes | PASS | no `ffmpeg`, `ffprobe`, `*.test` on Windows; `ps` in the container shows none (after the package run and after the F3 run); the only container is the pre-existing `buildx_buildkit_mivo-release0` |
| Web, Docker build, E2E | not applicable | the task touches only `internal/adapters/ffmpeg` and docs |

## Verification of the F2 and F3 fixes

- F2, code: `bindChildLifetime` now calls `setJobInformation` (`lifetime_windows.go:57-81`), annotated
  `//go:uintptrescapes`, with the `uintptr(unsafe.Pointer(&limits))` conversion inside the call
  expression, so the compiler moves `limits` to the heap.
- F2, the new tests have teeth. Mutation in the scratch worktree (only the `//go:uintptrescapes`
  line removed): `TestBindChildLifetimeUnderGCPressure` with 5 000 000 binds fails in 3 of 3
  runs with `first failed bind: job limit flags = 0x0, want JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`,
  with 58, 37 and 63 of 5 000 000 binds lost. That confirms the round-2 suspicion: without the fix a
  job silently loses `KILL_ON_JOB_CLOSE` (success, flags zero). With the default 500 000 binds the
  mutant is detected in 6 of 6 runs, so the default test is a reliable regression guard. With the
  fix restored (file identical to the main tree) there are 0 failures in 4 x 5 000 000 binds.
- F3: the test uses the new fake mode `hang-holding-stderr`; a grandchild holds the stderr pipe, so
  `Done` cannot close before the stop timeout (50 ms). It asserts `Done` is open after `Stop`, exactly
  one warning, then kills the holder and expects `Done` and a stopped state. 200/200 on Windows,
  200/200 with `-race` on Windows and Linux.

## Round 1 and 2 results re-checked

- Hostile inputs refused: `TestHostileInputsNeverOpenOtherFiles` and `TestProbeRefusesImageSequencePatterns`, together with `TestChildDiesWithItsParent` and `TestProbeCorpusMatchesManifest`, `-count=10` on Windows: 40 PASS, 0 FAIL, 0 SKIP; Linux `-race -count=5` together with the rename tests: 30 PASS, 0 FAIL, 0 SKIP.
- Orphan child: `TestChildDiesWithItsParent` is part of the runs above on both systems; nothing left over.
- Rename stress: `TestProducerRenamesThePlaylistWhileReadyPolls` and `TestProducerCompletesALostFinalPlaylistRename` `-count=20` on Windows: 40 PASS, 0 FAIL; Linux `-race -count=5` PASS.
- Corpus probe identity: a throwaway test (deleted) compared plain `ffprobe` JSON with the package's `inputArguments` (only `-format_whitelist` dropped) for all 49 files under `testdata/media`: 48 media files byte-identical, 0 different; the 49th is `manifest.json` (not media, rejected by both).

## Acceptance criteria (round 3)

1. Demuxer restriction on every invocation - MET (corpus identical, 45/45 `TestProbeCorpusMatchesManifest` in every run).
2. Hostile inputs refused - MET.
3. Children die with the server - MET: mechanism tests pass on both systems, Job Object configuration reliable (0 failures in 150 000 starts and 20 000 000 binds, flags verified by read-back), mutation shows the guard works.
4. `ErrorOutput` doc - MET (unchanged since round 1).
5. Windows rename-over under 1 ms poll - MET.
6. Nits - MET.
7. Gates - MET: lint (Windows and Linux), layer rules, Linux race, Windows and Linux corpus integration, coverage 96.9 %, repeated runs without any flake.

## Notes (non-blocking)

- Known and documented gap: the tool runs briefly before it joins its job on Windows (ADR 0024 Notes, BACKLOG).
- F1 (ACCESS_DENIED for a terminating process) is still verified by reading and by `TestAdoptAcceptsAProcessThatAlreadyEnded`; it could not be re-triggered in any run of rounds 2 or 3 (150 000 stress starts without a failure), so its fix cannot be proven by observation, only that no regression showed up.

## History (round 3)

- Round 3: PASS. F2 and F3 fixed and proven; no flakes in 30 x 5000 starts, 4 x 5 000 000 binds, 200 x F3 (3 variants), 5 x full `go test ./...`, `-count=10` package run on Windows, `-race` runs on Linux.