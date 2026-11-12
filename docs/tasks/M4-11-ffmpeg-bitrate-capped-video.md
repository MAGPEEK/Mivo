# M4-11 FFmpeg: bit-rate capped video (quality steps)
Owner: mivo-media
Depends on: M4-04c, M4-07a
Files/areas: `internal/adapters/ffmpeg/hlsargs.go` (new `bitrateargs.go`) (+ tests)

## Goal
A quality step (PL-10) produces a stream that stays under the chosen bit rate without breaking segment alignment (ADR 0030 §6).

## Acceptance criteria
- [x] With a cap: `-maxrate <video budget> -bufsize 2×maxrate` and `-crf` kept (constrained CRF) for libx264. Hardware-encoder rate control (`-b:v`, `-maxrate` per API) moved to M4-09b (amended by the orchestrator 2026-11-12: no hardware argument path exists before M4-09b; `videoArguments` returns ErrUnsupported for non-libx264 encoders, so nothing is silently uncapped).
- [ ] Video budget = cap − audio bit rate (never < 200 kbit/s); documented constant.
- [ ] Output height from the plan; width even; no upscale.
- [ ] Tests: golden args per step; corpus test: a 4 Mbit/s cap on a 20 s 1080p clip produces segments whose total size ≤ 1.3 × the cap.
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/ffmpeg/...` + coverage gate.

## Notes
- Without a cap the M2 constant-quality behaviour is unchanged.
