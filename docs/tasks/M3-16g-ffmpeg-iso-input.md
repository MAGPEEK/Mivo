# M3-16g FFmpeg: ISO input through a loopback range source
Owner: mivo-media
Depends on: M3-16e, M3-16f
Files/areas: `internal/adapters/ffmpeg` (new file `iso_input.go`), `internal/adapters/disc/iso` (read-only use)

## Goal
Let FFmpeg read clips inside an ISO without mounting and without new FFmpeg protocols.

## Acceptance criteria
- [ ] A per-session loopback HTTP range server (127.0.0.1, random port, random path token, closed on session end) serves each clip as a seekable resource from the ISO reader; FFmpeg input is `http://127.0.0.1:<port>/<token>/<clip>` joined via the concat demuxer.
- [ ] The `http` protocol is whitelisted only for that loopback form (host + token checked before the process starts); no other URL can reach FFmpeg.
- [ ] Range requests, EOF and cancellation handled; server leaks no goroutines (test with `goleak`-style check or counted goroutines).
- [ ] Tests: unit tests for the range server; integration test with a generated ISO (skip under `-short`).
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Fallback if loopback proves flaky on the NAS: FFmpeg `pipe:0` fed sequentially (no seeking; transcode-from-start only), documented in ADR 0029 Notes.
