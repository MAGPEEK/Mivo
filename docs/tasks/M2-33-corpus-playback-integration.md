# M2-33 Integration test: every corpus video plays with the expected mode
Owner: mivo-media
Depends on: M2-06, M2-09, M2-10, M2-11, M2-12, M2-15, M2-16, M2-19, M2-20, M2-24, M2-25
Files/areas: `internal/adapters/corpustest/**` (new package: `doc.go`, `expectations.go`,
`*_test.go`)

## Goal
One test proves the milestone promise: the whole generated corpus is scanned with the real
adapters, and every video file gets the expected mode and produces playable output (ADR 0025).

## Acceptance criteria
- [ ] `expectations.go` (non-test, importable by `scripts/`): expected mode and reason codes per
      manifest case name for `chromium-desktop` and `firefox-desktop`, covering every generated
      case with a video stream (categories `video`, `subtitle`, `multitrack`, incl. the long clips).
- [ ] The test reads `testdata/media/manifest.json` (schema 2), fails when a generated video case
      has no expectation, and skips cleanly (with the reason) when FFmpeg or the manifest is
      missing or under `-short`.
- [ ] Flow per run: temp SQLite + migrate → library at `testdata/media` → `app/scan` with the real
      walker and prober → for each case and profile `app/playback.Start` with the real producer,
      `FullCapabilities` → mode and reasons match → for HLS modes wait for `ended`, join `init.mp4`
      and segments, ffprobe the result: video codec `h264` when transcoded or the source codec
      when copied, audio `aac` or the source codec, duration within 0.5 s of the manifest; for
      direct play the stream source resolves to the original file.
- [ ] Seek: the long MKV started at 21 s reports an actual start at the keyframe 20 s; the long TS
      started at 21 s (transcode) reports 21 s; each output lasts 30 s minus its actual start
      (±0.5 s).
- [ ] `NoEncoders` capabilities: exactly the transcode cases fail with `transcode_unavailable`,
      the others keep their mode.
- [ ] Sessions run with bounded parallelism (named constant) and the transcode workspace is empty
      at the end.
- [ ] Runtime on the dev machine logged in the REPORT; `go test ./...` without `-short` stays
      under 3 minutes in total.
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/corpustest/...` + coverage
      gate (the package's non-test code is covered by its own test).

## Notes
- Expected modes must agree with the M2-09 matrix rows; a disagreement is a bug in one of them,
  not a reason to loosen either.
- ffprobe of joined fMP4: concatenating `init.mp4` + `seg-*.m4s` gives a valid fragmented MP4;
  probing the `.m3u8` directly may need `-allowed_extensions` options that differ between FFmpeg
  7.1 and 9.0 – avoid it.
- No `cmd` wiring is used; the test builds the graph itself, like `cmd` would.
- (from M2-06) MPEG-TS corpus files start at pts ≈ 1.44 s: measure keyframe spacing relative to the
  first keyframe, not to 0. The manifest has a new optional field `expected.video.track`
  (dispositions); `track.language` is omitted when empty. Seek clips:
  `video-mkv-h264-aac-long` (remux path) and `video-ts-mpeg2-mp2-long` (transcode path), 30 s,
  keyframe every 2 s.
- (from M2-11) FFmpeg 7.1 (Docker image) reports the format start_time of
  `h264_aac_long.mkv` as −0.021 s (FFmpeg 9.0.2 on the dev machine: 0). Keyframe positions are
  relative to the format start like `-ss`, so 7.1 yields 20.021 s instead of 20 s. Compare relative
  to the first keyframe or allow a tolerance (≥ 0.05 s); never assert exact seconds across FFmpeg
  versions.
