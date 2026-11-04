# M4-09a FFmpeg: hardware encoder detection with a test run
Owner: mivo-media
Depends on: M4-04d, M4-05a
Files/areas: `internal/adapters/ffmpeg/hwdetect.go`, `capabilities.go` (+ tests, fixtures)

## Goal
The server knows which hardware encoders actually work on this machine (VAAPI, QSV, NVENC, AMF), verified by encoding, and
reports "none" cleanly on a CPU-only NAS like the DS925+ (ADR 0030 §5; AD-25).

## Acceptance criteria
- [ ] `DetectHardware(ctx, options) ([]playback.HardwareEncoder, error)` first reads `-encoders`/`-hwaccels` to list candidates
      (`h264_nvenc`, `h264_qsv`, `h264_vaapi`, `h264_amf`), then runs a test encode per candidate: 1 s `lavfi testsrc2` 1280x720 → `-f null`,
      10 s timeout each, sequential, device from `transcode.hwaccel_device` or the first `/dev/dri/renderD*`.
- [ ] A candidate is `Usable` only when the process exits 0; failures keep a short non-path `Message` (`device_not_found`, `driver_failed`, …)
      mapped from known stderr patterns; the raw tail stays in the log (Notes M2-39).
- [ ] No candidate listed (stock static FFmpeg) = empty slice without running anything; no `/dev/dri` = VAAPI/QSV candidates skipped.
- [ ] Tests: parser/mapping unit tests on fixtures (FFmpeg 7.1 Docker, 9.0.2, Jellyfin-FFmpeg listing with all four APIs); a fake-tool test for
      success, failure, timeout; integration test on the local machine (asserts only "no error" and that the result is consistent).
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/ffmpeg/...` + coverage gate.

## Notes
- Never assume hardware from the listing alone; this is the point of the test run.
- Detection must not run concurrently with a hardware transcode start; the caller (M4-13d) serialises.
