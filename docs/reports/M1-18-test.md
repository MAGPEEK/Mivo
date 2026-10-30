# M1-18 test report: gen-testmedia runner, manifest and video cases

Status: PASS
Round: 1
Environment: FFmpeg 9.0.2-full_build-www.gyan.dev (gyan.dev "full"), Go 1.27.3, golangci-lint 2.15.0, Windows 11.
Scope: `./scripts/...` only (other agents work on M1-04b, M1-06b, M1-07 in parallel; repo-wide `go test ./...` and `golangci-lint run` were not run).

## Gates

| Gate | Result | Evidence |
|---|---|---|
| `go vet ./scripts/...` | PASS | exit 0, no output |
| `go test -race -count=1 -cover ./scripts/...` | PASS | `ok .../scripts/gen-testmedia 3.325s coverage: 95.9% of statements` |
| `golangci-lint run ./scripts/...` | PASS | `0 issues.` |
| `golangci-lint fmt --diff ./scripts/...` | PASS | exit 0, no diff |
| Coverage profile (`-race -coverprofile`, scratch dir) | PASS | total 95.9 % (>= 70 %). Weakest: `main` 0 % (os.Exit wrapper), `removePartial` 66.7 %, `writeManifest` 73.3 %, `encode` 75 % |
| Domain >= 90 % / layer rule | N/A | task touches no `internal/` code; no new imports of `internal/**` from scripts |
| Web / Docker | N/A | not touched |

## Manual runs (testdata/media removed first; it held 22 files + manifest)

- Clean run `go run ./scripts/gen-testmedia`: exit 0, `ffmpeg 9.0.2-full_build-www.gyan.dev: 22 generated, 0 skipped, 0 failed`, 0.84 s wall clock (incl. `go run` compile).
- Second run without `-force`: exit 0, all 22 listed as generated, mtimes of all files unchanged (files kept, not re-encoded).
- `-force`: all 22 re-encoded (0.74 s). SHA-256 of all 23 files (22 media + `manifest.json`) compared before/after: 0 differing, so regeneration is byte-identical. No `*.partial` leftovers.
- `-only 'video-webm-vp9-*'`: exit 0; manifest still has 22 entries, 22 generated (unselected cases kept); manifest hash identical; mkv mtime unchanged. With `-force -only ...`: only vp9_opus.webm mtime changed, mkv untouched.
- `-only 'nomatch*'`: exit 0, manifest still 22 generated. `-only '['`: `invalid -only glob "[": syntax error in pattern`, usage printed, exit 2. `-jobs 0`: clamped to 1, works. `-ffmpeg nonexistent`: clear error, exit 1.
- `git status --short` shows no path under `testdata/media`; `git check-ignore -v` -> `.gitignore:22:/testdata/media/`. Manifest case names are sorted.

## Independent ffprobe checks

- `hevc_main10_hdr10_ac3.mkv`: hevc, Main 10, yuv420p10le, `color_primaries=bt2020`, `color_transfer=smpte2084`, `color_space=bt2020nc`, ac3 48000 Hz stereo; first frame side data: "Mastering display metadata" (max_luminance 10000000/10000, min 1/10000) and "Content light level metadata" (max_content 1000, max_average 400). Manifest entry carries the same expectation incl. `hdr: hdr10`.
- `mpeg4_xvid_mp3.avi`: mpeg4 Simple Profile, `codec_tag_string=XVID`, mp3, format avi. `mpeg4_divx_mp3.avi`: `DIVX`.
- `vp9_opus.webm`: vp9 Profile 0 320x240, opus 48000/2, format `matroska,webm`.
- `mpeg2_mp2.ts`: mpeg2video Main, mp2, format mpegts.
- Extra: `h264_ac3.m2ts` is 192-byte packet stream (size mod 192 = 0, sync byte 0x47 at offsets 4 and 196); `h263_aac.3gp` 352x288 h263 + aac; `rv20_ra144.rmvb` rv20 + ra_144, format rm.
- Integration test `TestIntegrationCorpusMatchesManifest` ran all 22 sub-tests against the repo corpus with ffprobe (PASS, not skipped); `TestIntegrationGeneratesMKVCase` PASS (also verifies no rewrite on second run).

## No-shell check

`grep` over `scripts/gen-testmedia/*.go` for `cmd.exe|sh|bash|powershell|-c|exec.Command`: only hit is `runner.go:49: exec.CommandContext(ctx, r.executable, args...)` (argument slice, no shell). Per-case timeout `caseTimeout = 2 * time.Minute` (`runner.go:21`), `-y` set explicitly in `buildArgs` (`cases.go:158`) and justified by the write-to-`.partial`-then-rename flow, stderr captured into a tail buffer capped at 2048 bytes with a `[truncated]` marker (`runner.go:27,53,63`).

## Acceptance criteria

1. Flags `-out` (default `testdata/media`), `-ffmpeg`, `-force`, `-only`, `-jobs` (default `runtime.NumCPU()`, min 1): MET. `main.go:80-84,98`; `-jobs` default shown as 20 in usage; `-jobs 0` runs fine.
2. Capability detection parses `-encoders`/`-muxers`; missing -> skipped with reason: MET. `capabilities.go`, `plan.go:55-63`; covered by `capabilities_test.go`/`plan_test.go` with real fixtures. (Skip path is not exercisable on this "full" build, 0 skipped; unit-tested only.)
3. Cases declared as data (name, output, args, encoders, muxers, expected container/codecs), 2 s, 320x240 or required size, testsrc2 + sine: MET. `cases.go:31-47,99-109,168-192`; RV needs 8 kHz mono, H.263 uses 352x288, all others 320x240.
4. Required video cases: MET. All listed items present and generated: MKV H.264/AAC, MKV HEVC Main10/AC-3, MKV HEVC10 HDR10, MP4 faststart, M4V, MOV H.264/PCM, AVI Xvid/MP3, AVI DivX, TS MPEG-2/MP2, M2TS, MTS, WebM VP9/Opus, WebM AV1/Opus (libsvtav1 -> libaom -> librav1e fallbacks), FLV, WMV2/WMA2, OGV, 3GP H.263 352x288, MPG, VOB, MXF MPEG-2/PCM, RM and RMVB (RV20, best effort).
5. `<out>/manifest.json` with status/reason/expected, sorted and stable: MET. Fields `schemaVersion`, `ffmpegVersion`, `cases[]` (name, category, file, status, reason, expected); sorted by name; byte-identical across runs.
6. Idempotent, exit code non-zero only on non-skipped failure: MET. See manual runs; `main.go:132` returns error only when `failed > 0`.
7. `exec.CommandContext` with arg slice, per-case timeout, `-y`/`-n` explicit, stderr in truncated failure reason: MET. See no-shell check.
8. Tests without FFmpeg (fixtures under `testdata/`, filtering by capability and glob, manifest ordering/JSON shape, arg building for two cases) plus integration test with ffprobe that skips with a clear message: MET. Fixtures `ffmpeg-9.0.2-encoders.txt` (251 lines), `ffmpeg-9.0.2-muxers.txt` (190 lines); skip message in `integration_test.go:35`; 43 test functions.
9. `go run ./scripts/gen-testmedia` generates corpus, REPORT lists counts and FFmpeg version: MET. 22 generated, 0 skipped, 0 failed, FFmpeg 9.0.2-full_build-www.gyan.dev.
10. Gates `golangci-lint run`, `go test ./...`, coverage: MET for scope `./scripts/...` (see Gates). Repo-wide runs were out of scope because of parallel work by other agents.

## Findings (non-blocking)

- `-only` with a glob that matches nothing exits 0 silently (summary still printed). Consider a warning; not required by the task.
- Progress output prints cases excluded by `-only` as `generated` (they are kept files); consistent with the manifest semantics, only slightly noisy.
- Coverage gaps are small error paths (`removePartial`, `writeManifest`, `encode`); total is far above the 70 % gate.

## History

- Round 1: PASS, no failures.

REPORT
status: PASS
task: M1-18
files: docs/reports/M1-18-test.md
tests: go vet ./scripts/... ok; go test -race -count=1 -cover ./scripts/... ok (95.9 %); golangci-lint run ./scripts/... 0 issues, fmt --diff clean; manual generator runs (clean, rerun, -force byte-identical, -only) ok; ffprobe spot checks ok. Corpus 22 generated / 0 skipped / 0 failed, FFmpeg 9.0.2-full_build-www.gyan.dev
findings: none blocking; minor notes in report (-only no-match exits 0 silently)
next: send to mivo-reviewer
