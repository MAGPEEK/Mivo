# M2-06 Test report: testmedia seek clips, multiple filters, manifest fixes

Status: PASS (round 1)
Date: 2026-10-30
Package: `scripts/gen-testmedia` (1 new production file `cases_seek.go`, 9 changed production files, 7 changed test files)

## Gates

| Gate | Command | Result |
|---|---|---|
| vet | `go vet ./scripts/...` | clean |
| lint | `golangci-lint run ./scripts/...` | 0 issues |
| format | `golangci-lint fmt --diff ./scripts/...` | no diff |
| tests + race + coverage (Windows) | `go test -race -count=1 -coverprofile=... ./scripts/...` | ok; gen-testmedia 97.7 % (threshold 90 %), commitmsg 97.9 % |
| FFmpeg integration tests ran | `go test -v ./scripts/gen-testmedia/` | `TestIntegrationCorpusMatchesManifest`, `...GeneratesMultiTrackCase`, `...GeneratesMKVCase`, `...GeneratesSeekCases`, `...GeneratesNewCategories` all PASS (not skipped; corpus present, ffmpeg 9.0.2 on PATH) |
| signal test (Linux, Docker golang:1.27.3, repo mounted `:ro`) | `go test -race -v -run 'TestSecondInterrupt\|TestInterruptContext\|TestHelperProcess\|TestWriteManifestIsGroupReadable' ./scripts/gen-testmedia/` | `TestSecondInterruptTerminatesTheProcess` PASS (0.02 s), `TestInterruptContextFollowsItsParent` PASS, `TestWriteManifestIsGroupReadable` PASS |
| TODO grep | `grep TODO scripts/gen-testmedia/*.go` | none |

Skips (every test ran on at least one platform):
- Windows: `TestSecondInterruptTerminatesTheProcess` (Linux only, ran on Linux), `TestHelperProcess` (helper subprocess, not a test).
- Linux: none relevant.
- `TestWriteManifestIsGroupReadable` skips on Windows (POSIX permissions), passes on Linux.

Lowest function coverage in gen-testmedia: `main` 0.0 % (composition entry, covered via `run`), `removePartial` 66.7 %, `encode` 75.0 %, `removeTextInputs` 80.0 %. Package total is far above the threshold.

Note: other packages were not judged (wave 1 runs in parallel); only `./scripts/...` was in scope.

## Acceptance criteria

| # | Criterion | Verdict | Evidence |
|---|---|---|---|
| 1 | Cases `video-mkv-h264-aac-long` (H.264 High/AAC, MKV) and `video-ts-mpeg2-mp2-long` (MPEG-2/MP2, TS): 30 s, 25 fps, keyframe every 2 s, manifest `durationSeconds` 30 | met | `cases_seek.go:20-37`; manifest entries: both `"durationSeconds": 30`, mkv `profile: High`, `r_frame_rate=25/1` for both; format durations 30.021 s (mkv) and 30.010 s (ts); tests `TestSeekCasesHaveAFixedKeyframeInterval`, `TestOnlySeekCasesChangeTheClipDuration`, `TestIntegrationGeneratesSeekCases`. Keyframe quote below |
| 2 | `-only` takes a comma-separated glob list, each glob validated, usage text updated, test | met | `-only 'video-mp4-*,video-webm-vp9-opus'` built exactly 2 cases (`h264_aac_faststart.mp4`, `vp9_opus.webm`); `-only 'video-['` prints `invalid -only glob "video-[": syntax error in pattern` + usage, `exit status 2`; `-only 'a,,b'` prints `empty glob in -only "a,,b"` + usage, `exit status 2` (via `go run`, which reports its own exit 1); usage text: "comma-separated globs, e.g. 'video-mkv-*' or 'video-mp4-*,video-webm-vp9-opus'"; tests `TestParseCaseFilterAcceptsGlobLists`, `TestParseCaseFilterValidatesEveryGlob`, `TestUsageDescribesGlobLists`, `TestRunExitCodes` |
| 3 | BACKLOG "testmedia encoders": kept files carry encoders over from the previous manifest, never dropped on interrupted/`-only` runs; kept labelled `kept` in progress and summary; tests | met | After the `-only` run the manifest (SHA-256 incl. encoders) was byte-identical to the full-run manifest, so the 43 kept entries retained their encoders; progress lines `kept      <name>`; summary columns `N generated, M kept`; tests `TestReadPreviousEncoders`, `TestReadPreviousEncodersWithoutAUsableManifest`, `TestGenerateListsKeptFilesWithTheirPreviousEncoders`, `TestGenerateCorpusListsKeptFilesWithoutEncodersWhenTheManifestIsUnusable`, `TestKeptEntriesAreGeneratedInTheManifest`, `TestGenerateCorpusKeepsTheManifestOnLaterRuns`, `TestWriteSummaryCountsEveryCategoryAndSkipKind` |
| 4a | `expected()` slice aliasing fixed, test | met | `TestExpectedReturnsIndependentCopies` (`cases_test.go:615`) PASS |
| 4b | Second Ctrl+C via `context.AfterFunc(ctx, stop)` | met | `main.go:57`; `TestSecondInterruptTerminatesTheProcess` PASS on Linux, `TestInterruptContextFollowsItsParent` PASS |
| 4c | Default dispositions for video/audio in multi-track MKVs | met | ffprobe below; manifest `expected.video.track` / `audio[].track` agree; tests `TestTrackArgsDispositions`, `TestEmbeddedSubtitleMKVMarksVideoAndAudioDefault`, `TestBuildArgsForMultiTrackMapsAndTagsEveryStream`. Schema stays `schemaVersion: 2` |
| 5 | `go run ./scripts/gen-testmedia -force` gives 45 generated, 9 gaps, 0 failed; rerun without `-force` keeps files byte-identical | met | Run 1: `total 45 generated, 0 kept, 0 skipped, 0 not selected, 9 gaps, 0 failed`, exit 0. Run 2 (no `-force`): `total 0 generated, 45 kept, ... 9 gaps, 0 failed`; SHA-256 of all 49 files in `testdata/media` (incl. `manifest.json`) identical before/after |
| 6 | ffprobe keyframes of the long MKV 2 s apart | met | quote below |
| 7 | Gates vet, lint, race tests, coverage >= 90 % | met | see Gates table (97.7 %) |

Extra check, `-force -only 'video-mp4-*,video-webm-vp9-opus'` on the full corpus: summary `video 2 generated, 22 kept ... total 2 generated, 43 kept, 9 gaps, 0 failed`; only `h264_aac_faststart.mp4`, `vp9_opus.webm` and `manifest.json` got new mtimes; no file hash changed (output deterministic); manifest hash equals the full-run manifest.

### Keyframes (quote)

`ffprobe -v error -skip_frame nokey -select_streams v:0 -show_entries frame=pts_time -of csv testdata/media/video/h264_aac_long.mkv`

```
frame,0.000000,
frame,2.000000
frame,4.000000
... (every 2.000000 s) ...
frame,26.000000
frame,28.000000
```
15 keyframes, 0 to 28 s, spacing exactly 2 s.

`mpeg2_mp2_long.ts`: 15 keyframes at 1.44, 3.44, 5.44, ... 27.44, 29.44 s (spacing exactly 2 s, stream starts at pts 1.44 s as claimed; format duration 30.010022 s).

### Dispositions (ffprobe)

```
h264_aac2_srt2.mkv  video default=1 | audio eng default=1 | audio deu default=0 | sub eng default=0 | sub deu forced=1
h264_aac_srt_ass.mkv video default=1 | audio default=1 | sub srt default=1 | sub ass default=0
```
Matches the manifest `track` fields.

## Findings (non-blocking)

- `docs/development.md:263` still describes `-only 'video-mkv-*'` as "selects cases by glob"; it does not mention comma-separated lists or the `kept` label. Minor docs follow-up (DoD item 4); `M2-36` (docs) is the natural place.
- `testdata/media/sidecar.mkv` (`subtitle/sidecar.mkv`) has default=0 on video and audio. This is outside the criterion (only multi-track and embedded-subtitle MKVs were in scope) and stays consistent with its manifest entry.
- Environment note: a freshly built `gen-testmedia.exe` in `%TEMP%` was blocked by Windows Defender on its second launch ("contains a virus or potentially unwanted software", typical heuristic false positive for unsigned Go binaries). Real runs used `go run`. Not a code issue.

## Housekeeping

- `testdata/media/` is git-ignored (`.gitignore:22`); no generated files show up in `git status`. The only new untracked file in `scripts/` is `cases_seek.go` (expected).
- During this run the QA tooling created a stray `coverage-scratch` file in the repo root through a PowerShell argument-splitting slip. It was removed again; `git status` shows no `coverage*` or `testdata` entries.

## History

- Round 1: PASS. All gates green, all criteria met.

REPORT
status: PASS
task: M2-06
files: docs/reports/M2-06-test.md
tests: go vet ./scripts/... clean; golangci-lint run ./scripts/... 0 issues, fmt no diff; go test -race ./scripts/... ok (gen-testmedia 97.7 %, FFmpeg integration tests ran, not skipped); Linux Docker run of the signal test PASS; real runs 45 generated / 9 gaps / 0 failed, rerun 45 kept byte-identical, -only subset rebuilt only 2 files with unchanged manifest, invalid globs exit 2
findings:
- non-blocking: docs/development.md:263 does not yet document comma-separated -only lists or the "kept" label (fits M2-36)
- non-blocking: subtitle/sidecar.mkv has no default dispositions (outside the criterion)
- both seek clips verified with ffprobe: keyframes exactly 2 s apart (mkv from 0 s, ts from 1.44 s), durations 30.02 s / 30.01 s
next: forward to mivo-reviewer
