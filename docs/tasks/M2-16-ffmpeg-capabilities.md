# M2-16 FFmpeg adapter: detect server capabilities
Owner: mivo-media
Depends on: M2-09, M2-11
Files/areas: `internal/adapters/ffmpeg/capabilities.go` (+ tests),
`internal/adapters/ffmpeg/testdata/ffmpeg-*-{version,encoders,muxers}.txt`

## Goal
At startup the server knows what its FFmpeg can do – version, HLS/MP4 muxing, H.264 and AAC
encoders – instead of assuming it, so playback never promises a transcode it cannot deliver
(ADR 0015, ADR 0023).

## Acceptance criteria
- [ ] `DetectCapabilities(ctx, ffmpegPath) (playback.ServerCapabilities, error)` runs
      `ffmpeg -hide_banner -version`, `-encoders` and `-muxers` through the M2-11 runner with a
      timeout, parses them and fills version, `hls` + `mp4` muxers, `libx264` and `aac` encoders.
- [ ] A missing executable returns zero capabilities plus an error wrapping
      `domain.ErrUnavailable`; the caller (M2-32) logs it and continues with direct play only.
- [ ] Parser unit tests on fixtures from FFmpeg 9.0.2 (copy `scripts/gen-testmedia/testdata/*`)
      **and** from the Docker image's FFmpeg 7.1.x (capture with
      `docker run --rm --entrypoint ffmpeg mivo:dev -hide_banner -encoders`, note the command in
      the fixture header).
- [ ] Integration test against the local FFmpeg (skip if missing): H.264, AAC and HLS reported.
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/ffmpeg/...` + coverage gate.

## Notes
- Hardware encoders and the "test run" detection of the plan are milestone 4; do not list them.
- `-encoders` output lines look like ` V....D libx264 ...`; match the name column, not substrings.
