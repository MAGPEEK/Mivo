# M1-19 test report: gen-testmedia audio, image, subtitle, multi-track

Result: **PASS** (round 1)
Date: 2026-10-30, FFmpeg/ffprobe 9.0.2 full build (gyan.dev), Windows 11, scope `./scripts/...`

## Gates

| Gate | Command | Result |
|---|---|---|
| vet | `go vet ./scripts/...` | exit 0 |
| tests | `go test -race -count=1 -cover -coverprofile=cov.out ./scripts/...` | ok, 5.1 s, **97.3 %** of statements |
| lint | `golangci-lint run ./scripts/...` | `0 issues.` |
| format | `golangci-lint fmt --diff ./scripts/...` | exit 0, no diff |
| coverage | `go tool cover -func=cov.out` | total 97.3 % (>= 70 %). Lowest non-main funcs: `runner.removePartial` 66.7 %, `manifest.encode` 75.0 %, `writeManifest` 83.3 %. `main` 0 % (composition root, covered by `run` tests) |
| integration | `-v` run | `TestIntegrationGeneratesMKVCase`, `...MultiTrackCase`, `...NewCategories`, `...CorpusMatchesManifest` ran against real FFmpeg, no SKIP |
| domain layer / threshold | n/a | `internal/domain` is not touched by M1-19 (only `scripts/`) |

Only `./scripts/...` was run, as instructed. Docker/web/E2E are not part of this task.

## Real corpus run (clean `testdata/media`, built binary)

Run 1 (clean dir, 1 s wall time): exit 0.

```
  audio       11 generated, 0 skipped, 0 not selected, 3 gaps, 0 failed
  image        6 generated, 0 skipped, 0 not selected, 2 gaps, 0 failed
  multitrack   1 generated, 0 skipped, 0 not selected, 0 gaps, 0 failed
  subtitle     3 generated, 0 skipped, 0 not selected, 3 gaps, 0 failed
  video       22 generated, 0 skipped, 0 not selected, 1 gaps, 0 failed
  total       43 generated, 0 skipped, 0 not selected, 9 gaps, 0 failed
```

Matches the expectation (43 generated, 9 known gaps, 0 failed). 47 files on disk (43 cases plus
3 extra sidecars plus manifest.json).

- **Rerun without `-force`**: exit 0; no media/sidecar file changed (hash and mtime identical for
  all 46 files); `manifest.json` was rewritten with identical content (same SHA-256).
- **Rerun with `-force`**: exit 0; all 47 files rebuilt (new mtimes) and **hash-identical** to run 1,
  including `manifest.json` and the sidecars (`sidecar.srt/.vtt/.ass`). Output is deterministic.
- **Leftovers**: no `.partial` or temp files and no unexpected directories under `testdata/media`;
  no `gen-testmedia-inputs-*` directories remain in `%TEMP%` (also after the interrupted runs).
- **Gaps in manifest**: 9 entries with `status: skipped`, `skipKind: known-gap`:
  `audio-ape`, `audio-dff-dsd`, `audio-dsf-dsd`, `image-heic`, `image-jpeg-exif`,
  `subtitle-sup-pgs`, `subtitle-vobsub`, `subtitle-ts-dvb`, `video-rmvb-rv40-cook`
  (the last one is the M1-18 carry-over "RealVideo 4 (RMVB)"). Reasons are
  `known gap (ADR 0021): ffmpeg has no ... encoder`. All 52 manifest cases = 43 generated + 9 skipped.
- All 43 media files probe with `ffprobe` rc=0 and sensible durations.

## M1-18 video files unchanged

- `-only 'video-*'` into a fresh directory: 22 generated, 21 "not selected" (counted separately in
  the summary), exit 0. All 22 files are byte-identical to `testdata/media/video/*`.
- Additionally built the M1-18 generator from `git archive HEAD` (commit a79386d) into a scratch
  directory and compared: 22 of 22 video files byte-identical to the current output.

## Independent ffprobe samples

| File | Observed |
|---|---|
| `audio/flac_24bit_96k.flac` | flac, `sample_fmt=s32`, `bits_per_raw_sample=24`, 96000 Hz, 2 ch, 5.0 s |
| `audio/flac_16bit_44k.flac` | flac, s16, 16 bit, 44100 Hz, 2 ch, 5.0 s |
| `audio/aac_chapters.m4b` | aac 48 kHz stereo (+ a `bin_data` chapter-text stream); tags title=Generated Test Book, artist=Generated Narrator, album=Generated Test Corpus; chapters `0-2.5 "Chapter 1"`, `2.5-5.0 "Chapter 2"` |
| `audio/alac.m4a` | alac, 48 kHz, 2 ch, 5.0 s |
| `image/av1.avif` | codec av1, 320x240, yuv420p, mov/mp4 container |
| `subtitle/h264_aac_srt_ass.mkv` | h264, aac, subtitle `subrip` (eng, default) and subtitle `ass` (eng) = 2 subtitle streams |
| `subtitle/h264_aac_mov_text.mp4` | h264, aac, `mov_text` (eng) |
| `multitrack/h264_aac2_srt2.mkv` | audio#1 eng default=1, audio#2 deu default=0; subtitle#3 eng forced=0, subtitle#4 deu forced=1 |
| `subtitle/sidecar.{srt,vtt,ass}` | next to `sidecar.mkv`; SRT `00:00:00,000 --> 00:00:00,900`, VTT `WEBVTT` + dot timings, ASS with valid `[Script Info]/[V4+ Styles]/[Events]` |

Other cases seen: opus (5.0065 s), vorbis, wavpack, mp3, aiff, wav, gif (animated, 1.0 s), jpeg, png,
tiff, webp all probe cleanly.

## Interruption handling

Unit tests: `TestGenerateAllAfterCancellationDoesNotFailUntouchedCases`,
`TestGenerateTreatsAFailureDuringInterruptionAsInterrupted`, `TestGenerateCorpusReportsInterruption`,
`TestExecRunnerKillsTheProcessOnCancel`, `TestWriteManifestIsGroupReadable` (0o640), `TestTailBufferCutsAtARuneBoundary`,
`TestExecRunnerWithoutStderrHasNoTrailingSeparator` all pass.

Real Ctrl+C equivalent (`GenerateConsoleCtrlEvent(CTRL_C_EVENT)` to a hidden console, run with
`-force -jobs 1`):

- **Empty output dir**, interrupted after 8 files: exit **1**, message
  `gen-testmedia: generation interrupted, run again to finish the corpus: context canceled`,
  manifest written: `generated=8; skipped/interrupted=35; skipped/known-gap=9`, 0 failed, summary
  shows `35 interrupted`, no `.partial`/temp, every "generated" entry has its file. A following
  plain run produced a corpus byte-identical to the full corpus (resume works).
- **Populated dir**, interrupted mid-run (20 and 29 of 46 files rebuilt in two attempts): exit 1,
  0 failed, untouched files kept (old mtime and content), no partials, manifest lists all 43 as generated.
- Interrupt during capability detection: exit 1, same message, existing manifest left untouched.

## Acceptance criteria

| Criterion | Status | Evidence |
|---|---|---|
| Audio cases (FLAC 16/44.1, FLAC 24/96, ALAC, MP3, AAC, Opus, Vorbis, WAV, AIFF, WavPack, M4B with 2 chapters and tags) | met | 11 generated; ffprobe samples above; `TestIntegrationCorpusMatchesManifest` |
| Image cases (JPEG, PNG, WebP lossy, animated GIF, TIFF, AVIF) | met | 6 generated, all probe |
| Subtitle cases (sidecar srt/vtt/ass, MKV srt+ass, MP4 mov_text) | met | 3 generated, 3 sidecar files, ffprobe streams |
| Multi-track MKV (eng/deu audio, one default; eng/deu subs, one forced) | met | ffprobe output above |
| Gap entries `skipped` with reason (APE, DSF, DFF, HEIC, PGS, VobSub, DVB, EXIF JPEG) | met | 9 known-gap entries incl. RV40; `TestRegistryDeclaresKnownGaps`, `TestGenerateCorpusListsKnownGapsInEveryRun` |
| Tests without FFmpeg: M4B and multi-track args, sidecar writer, gaps; integration test for multi-track | met | `TestBuildArgsForAudiobookAddsChaptersAndTags`, `TestBuildArgsForMultiTrackMapsAndTagsEveryStream` (`-map`, `-metadata:s:a:0 language=eng`, `-disposition`), `TestFormatSRT`/`TestGeneratedSRTFilesAreValid`, `TestIntegrationGeneratesMultiTrackCase` (ran, not skipped) |
| Full program run succeeds, REPORT shows counts per category | met | tables above |
| Gates lint/test/coverage | met | see Gates |

### Review carry-over from M1-18

| Item | Status | Evidence |
|---|---|---|
| RealVideo 4 gap entry | met | `video-rmvb-rv40-cook` known-gap; `.rmvb` still RV20 |
| Temp manifest chmod to named 0o640 | met | `outputFilePermissions = 0o640` in `manifest.go:22`, `Chmod` at `:155`, `TestWriteManifestIsGroupReadable` |
| Interrupted runs: wrapped error, untouched not failed, SIGTERM handled | met | `signal.NotifyContext(..., os.Interrupt, syscall.SIGTERM)` in `main.go:44`; real run above |
| No trailing ": " on empty stderr | met | `TestExecRunnerWithoutStderrHasNoTrailingSeparator` |
| Stderr tail on UTF-8 rune boundary | met | `process.go:81` `utf8.RuneStart`, `TestTailBufferCutsAtARuneBoundary` |
| HDR10 checks colour matrix `bt2020nc` | met | `cases_video.go:74` |
| Summary counts "not selected" separately | met | observed in `-only` run (`21 not selected`) |
| Record encoder used (schema bump) | met | manifest `schemaVersion: 2`, `encoders` array per case |
| Remove `gen.jobs = 0` in runner_test.go | met | grep finds no match |

## Findings (non-blocking, no FAIL)

1. After an interrupted `-force` run on a populated directory, the manifest entries for cases that were
   kept but not rebuilt are written without `encoders` (14 of 43, e.g. `audio-aiff-pcm` loses
   `"pcm_s16be"`), so the manifest differs from a complete run until the next run. Cause: `keptOrSkipped`
   in `runner.go` (~line 91) returns `newEntry(...)` without the encoder list. The same applies to files
   kept by `-only` ("not selected"). Suggestion: carry the planned encoders into kept entries.
2. The progress line and summary label cases whose existing file was kept (no `-force`) as `generated`,
   so a rerun without `-force` prints 43 generated although nothing was built. Cosmetic.
3. The M4B has an extra `bin_data` stream (FFmpeg's chapter-text track). Normal; consumers should
   ignore non-audio streams.

## History

- Round 1: PASS (all gates green, 43 generated / 9 gaps / 0 failed, determinism and interruption verified).

REPORT
status: PASS
task: M1-19
files: docs/reports/M1-19-test.md
tests: go vet ./scripts/... ok; go test -race -count=1 -cover ./scripts/... ok 97.3 %; golangci-lint run + fmt --diff clean; real corpus run 43 generated / 9 known-gap / 0 failed, rerun and -force hash-identical, M1-18 video files byte-identical, Ctrl+C run exit 1 with manifest and no partials
findings:
- non-blocking: interrupted/-only runs write kept-but-not-rebuilt manifest entries without `encoders` (runner.go keptOrSkipped)
- non-blocking: kept existing files are labelled "generated" in progress/summary on a non-force rerun
next: orchestrator may proceed to mivo-reviewer
