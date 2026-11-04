# M4-04d Domain: hardware encoder capabilities and encoder choice
Owner: mivo-backend
Depends on: none
Files/areas: `internal/domain/playback/capabilities.go`, `encoder.go` (+ tests)

## Goal
The engine can name the video encoder of a transcode: a usable hardware encoder when configured/detected,
software otherwise, never a hardware encoder that failed its test (ADR 0030 §5; AD-25, PL-41).

## Acceptance criteria
- [ ] `ServerCapabilities.HardwareEncoders []HardwareEncoder{API, Name, Codec, Device, Usable, Degraded}`.
- [ ] `ChooseVideoEncoder(capabilities, setting) Encoder`: `off` or nothing usable = libx264; `auto` order
      NVENC, QSV, VAAPI, AMF; a named API picks it only when usable, else software with reason `hwaccel_unavailable` (info).
- [ ] Degraded encoders are skipped; HEVC output is not chosen in M4 (H.264 only, ADR 0023 target).
- [ ] The plan's video `Encoder` carries the FFmpeg encoder name (`h264_vaapi`, …) and a `Hardware` flag.
- [ ] Tests: table-driven; coverage ≥ 90 %.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
- DS925+ case (no hardware entries) must be a named test: result is libx264, no warning.
- Decode acceleration is an adapter detail (M4-09b), not a domain concept.
