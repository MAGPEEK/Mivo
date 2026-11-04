# M4-13d App: transcoder status and hardware test
Owner: mivo-backend
Depends on: M4-09a, M4-04d, M4-05a
Files/areas: `internal/app/transcoder/**` (new package, + tests)

## Goal
The server runs the hardware detection in the background at startup, offers "Test hardware acceleration" and tells the playback service which
encoders are usable (ADR 0030 §5; AD-25).

## Acceptance criteria
- [ ] `Service.Status()` returns setting, detected encoders (usable/degraded/message), `throttleSupported`, `lastTestedAt`; `Test(ctx)` re-runs
      detection (409-style `ErrBusy` while running), publishes the result atomically.
- [ ] Startup detection is asynchronous: sessions started before it completes use software; a CPU-only machine finishes with an empty list and one info log.
- [ ] `MarkDegraded(api)` excludes an encoder until the next successful `Test`.
- [ ] `Capabilities()` merges the FFmpeg base capabilities with the hardware list for `playback.Decide`.
- [ ] `hwaccel=off` never runs detection.
- [ ] Tests: fakes for the detector: success, failure, busy, degraded, off, slow detection while a session starts.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
- Detection and the first hardware transcode must not collide: take the same mutex a hardware start takes (reader/writer).
