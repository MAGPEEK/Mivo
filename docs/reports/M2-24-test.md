# M2-24 test report: FFmpeg HLS stream producer

Status: PASS (round 1)

Method: clean `git worktree` at HEAD 5e64b56 plus exactly the nine files of the M2-24 list
(`capabilities.go`, `doc.go`, `faketool_internal_test.go`, `process.go`, `process_internal_test.go`,
`fakehls_internal_test.go`, `producer.go`, `producer_corpus_test.go`, `producer_internal_test.go`,
all under `internal/adapters/ffmpeg/`) and the `testdata/media` corpus copied from the main tree.
Windows 11 with local FFmpeg 9.0.2; Linux in `golang:1.27.3` (Debian 13) with `apt` FFmpeg
7.1.5-0+deb13u1. The worktree was removed afterwards.

## Gates

| Gate | Result | Evidence |
|---|---|---|
| `go build ./...` | pass | no output |
| `go vet ./...` | pass | no output |
| `go test -coverprofile ./...` (Windows) | pass | all packages `ok`; `internal/adapters/ffmpeg` 97.6 % |
| go-test-coverage (`.testcoverage.yml`) | pass | `Total coverage threshold (70%) satisfied: PASS`, total 96.8 % (4348/4490) |
| Domain coverage >= 90 % | pass | `internal/domain/{library,media,playback,playbacktest,system}` 100.0 %, `internal/domain` has no statements |
| `golangci-lint run --allow-parallel-runners` (incl. depguard) | pass | `0 issues.` |
| `golangci-lint fmt --diff` | pass | no diff |
| `go mod tidy -diff` | pass | exit 0 |
| archtest | pass | `ok internal/archtest` |
| Layer rule (`go list -deps ./internal/domain/...`) | pass | no `database/sql`, `net/http`, `os/exec`, `internal/adapters`, `internal/app` |
| Linux `go test -race ./...` (Docker, FFmpeg 7.1.5) | pass | every package `ok`, `internal/adapters/ffmpeg` 27.3 s |
| `-race` on Windows | skipped | no cgo toolchain (`go: -race requires cgo`); covered by the Linux Docker run |
| Web gates, E2E, `docker build` | not applicable | task touches only Go adapter code |

Per-file coverage: `producer.go` functions are 100 % on Windows except `inheritedFiles` 33.3 %,
`closeAll` 50 % and `start` 88.9 %. These are the Linux-only branches (inherited descriptor) and are
exercised by the Linux run (`TestProducerHandsTheLibraryFileToFFmpeg`,
`TestProducerStartFailures/input_missing_in_the_library`). `playlistListsSegment` is 88.9 % (open-error
branches).

## Integration tests with real FFmpeg

| Platform | Run | Result |
|---|---|---|
| Windows, FFmpeg 9.0.2 | `go test -v -run 'Producer\|Corpus'` | `TestProducerRemuxesAClipToTheEnd`, `TestProducerStopsARunningTranscode`, `TestProducerReportsACorruptInput` PASS, not skipped |
| Windows | `-count=10 -run 'Producer\|ListsSegment'` | `ok` 36.3 s, no flake |
| Linux, FFmpeg 7.1.5 (race) | `-race -count=1 -v -run Producer` | all 14 Producer tests PASS, no SKIP |
| Linux (race) | `-race -count=10 -run 'Producer\|ListsSegment'` | `ok` 51.8 s, no flake |

/proc/self/fd/3 proof on Linux: a PATH wrapper around the real `ffmpeg` logged every invocation and
`ls -l /proc/self/fd/3`. The three producer runs (remux, transcode, corrupt) were all started with
`-protocol_whitelist file ... -i file:/proc/self/fd/3`, and descriptor 3 resolved to
`/src/testdata/media/video/h264_aac.mkv`, `.../mpeg2_mp2_long.ts` and
`.../corrupt.mkv` respectively, so the library file is handed over by descriptor and real FFmpeg
reads it.

Leftover processes: `Get-Process ffmpeg,ffprobe` was 0 before and after the Windows `-count=10` run;
`pgrep -a ffmpeg` in the Linux container after the `-count=10` run printed `none`. The Windows
transcode test never logged "finished before Stop", so the kill path was exercised.

## Probes

| Probe | Result | Evidence |
|---|---|---|
| Ready not before the first segment | met | `TestProducerReadyWaitsForTheFirstSegment` (fake HLS writes the segment after a delay; asserts the segment file exists when Ready returns; `TestProducerReadyEndsWithItsContext/hls_header_only` stays not ready); real FFmpeg in the corpus tests |
| Early FFmpeg exit with exit code | met | `TestProducerReportsAnExitBeforeTheFirstSegment` (`exited with code N`, `ErrorOutput` stderr tail); `TestProducerReportsACorruptInput` with real FFmpeg on both OSes (`ffmpeg exited with code`) |
| Stop idempotent, concurrent with Ready | met | `TestProducerStopIsIdempotentAndSafeWithReady` (4 Ready + 4 Stop goroutines, all Ready get `ErrProductionStopped`, repeated Stop < 1 s); passes under `-race` count=10 on Linux |
| Stop within the 10 s bound, directory removable on Windows | met | `stopTimeout = processWaitDelay(5 s) + 5 s = 10 s` (`producer.go:27`); `TestProducerStopsARunningTranscode` runs `os.RemoveAll(job.OutputDir)` immediately after `Stop` on Windows, succeeds in ~0.5 s total; `TestProducerStopGivesUpAfterTheStopTimeout` covers the timeout branch with one warning |
| Clean end without segment is an error | met | `TestProducerReportsACleanExitWithoutSegments` (`errNoSegment`, one warn with `exit_code 0`, `segments_written false`) |
| Service context cancelled without Stop | met | `TestProducerStopsWithItsContext`: Ready returns `ErrProductionStopped`, `assertNoWarnings` |
| Exactly one warn per failure with stderr tail | met | `assertOneFailureLog` in the early-exit, after-readiness (`TestProducerLogsAFailureAfterReadinessOnce`, two Ready calls plus Stop add no second record) and clean-exit tests; the record carries `session`, `exit_code`, `segments_written`, `error`, `stderr` |
| No FFmpeg left after the tests | met | see "Leftover processes" |
| `-protocol_whitelist file` and `file:` inputs | met | unit assertion `-protocol_whitelist file -i file:<input>` in `TestProducerHandsTheLibraryFileToFFmpeg` (both OS variants), `hlsargs_test.go:221-257` for the argument builder, and the real Linux invocations above |

## Acceptance criteria

1. `Producer.Start(ctx, job)` builds the arguments with `HLSArguments`, starts through the runner
   (`tool.start`), returns a `Production` with `Ready`, `Stop`, `Done` plus the exit result through
   `Ready` after `Done`: **met** (`producer.go:105-157`, compile-time check
   `var _ appplayback.StreamProducer = (*Producer)(nil)`, `TestProducerHandsTheLibraryFileToFFmpeg`).
2. `Ready` returns once `index.m3u8` lists a segment (named `readyPollInterval` = 150 ms), the
   process error if FFmpeg exits first, `ctx.Err()` when the caller gives up, concurrent callers fine:
   **met** (tests above; the playlist is read through `os.Root` so FFmpeg's rename works on Windows).
3. `Stop` kills, waits bounded by the named `stopTimeout`, idempotent, safe with `Ready`: **met**.
4. A failure after readiness is logged once at warn with session, exit code and bounded stderr tail;
   a kill by `Stop` or by the service context is not logged: **met**
   (`TestProducerLogsAFailureAfterReadinessOnce`, `TestProducerReadyWaitsForTheFirstSegment`,
   `TestProducerStopIsIdempotentAndSafeWithReady`, `TestProducerStopsWithItsContext`).
5. Integration tests (skip without FFmpeg/corpus and under `-short`): remux of the MKV H.264 clip until
   `Done` with `#EXT-X-ENDLIST` (`hlsAssertPlaylist`), transcode of the long TS clip with `Stop` after
   `Ready` (Done closed, directory removable at once), corrupt file gives an error with the exit code:
   **met** on Windows and Linux.
6. Gates (lint, layer rules, race test of `internal/adapters/ffmpeg`, coverage gate): **met**.

Notes from the task file:
- `ErrProductionStopped` after Stop (wraps `domain.ErrNotFound`) and an error for a clean exit
  without segment: met (see probes).
- `StartPosition` is passed unchanged (the corpus test derives it from `KeyframeAtOrBefore`): met.
- Linux TOCTOU closure through `libraryfs`/`os.Root` and `ExtraFiles` plus `/proc/self/fd/3`: met
  and verified with real FFmpeg; Windows keeps the resolved path.

## Open items for the orchestrator (not test failures)

- The "Orchestrator note (after implementation)" asks for ADR notes in ADR 0022 (Linux descriptor
  input, content-only format detection) and ADR 0024 (warn log with stderr tail that may contain an
  absolute path, playlist read through `os.Root`) before the M2-24 commit. They are not in the main
  tree yet (`grep 'proc/self/fd' docs/adr/*` finds nothing); the tasks says to add them after M2-25
  landed its ADR 0024 notes. They were outside the file list and are a Definition-of-Done item.
- `TestProducerStopGivesUpAfterTheStopTimeout` proves the give-up path with a 1 ns timeout; the
  10 s bound itself is a constant and is not measured against a process that ignores the kill.

## History

- Round 1: PASS.

REPORT
status: PASS
task: M2-24
files: docs/reports/M2-24-test.md
tests: build, vet, go test + go-test-coverage (96.8 % total, domain 100 %), golangci-lint 0 issues, fmt clean, archtest ok, Linux -race ./... ok (FFmpeg 7.1.5), producer tests -count=10 on Windows (FFmpeg 9.0.2) and Linux without flakes, no FFmpeg process left
findings:
- all acceptance criteria met; /proc/self/fd/3 verified with real FFmpeg on Linux
- ADR 0022/0024 notes from the orchestrator note are still to be written before the commit
next: orchestrator adds the ADR notes (after M2-25) and proceeds to mivo-reviewer
