# M1-18 gen-testmedia: runner, manifest and video container cases
Owner: mivo-media
Depends on: M1-02, M1-03
Files/areas: `scripts/gen-testmedia/**` (`main.go`, `runner.go`, `capabilities.go`,
`manifest.go`, `cases.go`, `cases_video.go`, tests)

## Goal
`go run ./scripts/gen-testmedia` generates short, rights-free video clips in every container
and codec FFmpeg can produce on the local machine. A manifest lists what was generated and
what was skipped, and why (ADR 0021).

## Acceptance criteria
- [ ] Flags: `-out` (default `testdata/media`), `-ffmpeg` (default `ffmpeg` on PATH),
      `-force` (regenerate existing), `-only <glob>` (case filter), `-jobs N` (default
      `runtime.NumCPU()`, minimum 1).
- [ ] Capability detection parses `ffmpeg -hide_banner -encoders` and `-muxers`. A case whose
      required encoders or muxers are missing is **skipped** with a reason, not failed.
- [ ] Every case is declared as data: name, relative output path, args, required encoders and
      muxers, expected container, video codec, audio codec. Clips are 2 s long, 320×240 (or
      the codec's required size), with `testsrc2` + `sine` sources.
- [ ] Video cases (at least): MKV H.264/AAC, MKV HEVC Main10/AC-3, MKV HEVC 10-bit with HDR10
      metadata, MP4 H.264/AAC (`+faststart`), M4V, MOV H.264/PCM, AVI MPEG-4 Part 2 (Xvid
      tag)/MP3, AVI with DivX tag, MPEG-TS MPEG-2/MP2, M2TS H.264/AC-3, MTS, WebM VP9/Opus,
      WebM AV1/Opus (if an AV1 encoder exists), FLV H.264/AAC, WMV WMV2/WMA2, OGV
      Theora/Vorbis, 3GP H.263 (352×288)/AAC, MPEG-PS (`.mpg`) MPEG-2/MP2, VOB MPEG-2/AC-3,
      MXF MPEG-2/PCM, RMVB/RM RV20 (best effort).
- [ ] Writes `<out>/manifest.json` with one entry per case: status
      (`generated|skipped|failed`), reason and expected properties. The output is sorted and
      stable.
- [ ] Idempotent: existing files are kept without `-force` and still listed as generated.
      Exit code is non-zero only if a non-skipped case failed.
- [ ] FFmpeg runs through `exec.CommandContext` with an argument slice (no shell), a per-case
      timeout constant, `-y`/`-n` handled explicitly, and stderr captured into the failure
      reason (truncated).
- [ ] Tests (no FFmpeg needed): parsing of real `-encoders`/`-muxers` output fixtures (store
      them under `scripts/gen-testmedia/testdata/`), case filtering by capability and glob,
      manifest ordering and JSON shape, and argument building for two representative cases.
      An integration test generates one MKV case into `t.TempDir()` and checks it with
      `ffprobe`. It **skips** with a clear message when `ffmpeg`/`ffprobe` are not on PATH.
- [ ] `go run ./scripts/gen-testmedia` on the dev machine generates the corpus. The REPORT
      lists generated/skipped counts and the FFmpeg version.
- [ ] Gates: `golangci-lint run`, `go test ./...` and the coverage check pass.

## Notes
- `/testdata/media/` is already git-ignored. Never commit media.
- Windows FFmpeg builds differ (e.g. gyan.dev "essentials" vs "full"). That is exactly why
  capability detection and skipping exist.
- `cases.go` holds the case type and an `allCases()` registry that concatenates per-category
  functions. M1-19 adds its categories with one line there.
- Use `-map_metadata -1 -fflags +bitexact` where possible for reproducible output.
- RealVideo: FFmpeg only encodes RV10/RV20. RV40 (typical RMVB) is a known gap (ADR 0021).
