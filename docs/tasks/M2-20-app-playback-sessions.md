# M2-20 Use case: start, read and stop playback sessions
Owner: mivo-media
Depends on: M2-09
Files/areas: `internal/app/playback/**` (new package: `service.go`, `session.go`, `ports.go`,
`start.go` + tests and fakes)

## Goal
A client asks to play a file with its profile and gets a session with mode, reasons and a stream
URL; the server starts FFmpeg only when remux or transcode is needed, enforces the session limit
and stops sessions on request (ADR 0023, ADR 0024).

## Acceptance criteria
- [ ] Ports declared here (each 1–3 methods): playable-file source (`PlayableFile(ctx, fileID)`),
      path resolver (`Resolve(root, relativePath)`), keyframe locator
      (`KeyframeAtOrBefore(ctx, path, position)`), stream producer (`Start(ctx, job)` returning a
      production with `Ready(ctx)`, `Stop()`, `Done()`), workspace (`Create`, `Remove`), ID
      generator, clock.
- [ ] `Start(ctx, request)`: validates the profile, loads the file (unknown → `ErrNotFound`;
      probe error → `ErrUnsupported`), runs `playback.Decide` with the configured capabilities and
      policy, and for HLS modes creates the workspace and starts the producer. Copy-mode seeks use
      the keyframe locator for the actual start; transcode seeks start exactly.
- [ ] `replacesSessionId` stops that session first (unknown ID ignored), then the limit is checked:
      above the limit → error wrapping `domain.ErrUnavailable` with code
      `session_limit_reached`. The scope follows OD-4; until it is answered implement the
      recommendation: `MaxTranscodes` counts sessions that re-encode any stream, and a fixed
      safety cap (named constant 8) counts all sessions running FFmpeg (remux included).
- [ ] The session's stream URL is built from a URL builder passed in by the adapter layer
      (app does not know HTTP paths); direct play has no workspace and no producer.
- [ ] `Get(ctx, id)` and `Stop(ctx, id)` (idempotent); stopping kills the producer, waits, removes
      the workspace. `Access(ctx, id)` returns what the stream handler needs (direct: library root
      + relative path + content type; HLS: workspace dir + ready wait) and records the access time.
- [ ] FFmpeg runs under a service-owned context, never the request context (test: cancelling the
      request context does not stop the production).
- [ ] Tests with fakes: each mode, seek in copy vs transcode, limit reached, replace under the
      limit, stop idempotent, producer start failure cleans the workspace, unknown file, probe
      error, concurrent starts (`-race`). Coverage ≥ 90 %.
- [ ] Gates: lint + layer rules + `go test -race ./internal/app/...` + coverage gate.

## Notes
- Idle reaping, startup purge, `ended`/`failed` state tracking and `Close` on shutdown are M2-25
  (same package, next wave). Leave a clear seam (access times, a session registry type) but no
  timers here.
- `Policy.MaxTranscodeHeight` and `MaxSessions` come through the constructor options (values from
  config via `cmd`, ADR 0003).
- Keep the session registry its own type with a mutex; methods stay ≤ 50 lines.

## Orchestrator note (from M2-11)
- `KeyframeAtOrBefore` returns an error when ffprobe cannot seek (no format start time). Fall back
  to position 0 (start of file) instead of failing the session; test it.
- Keyframe positions are relative to the format start (like `-ss`), not absolute pts.
- (from M2-09 review) Call `playback.Policy.Validate()` in the service constructor so a bad
  `MaxTranscodeHeight` fails at startup instead of answering clients with 400.

## Orchestrator note (round 2)
- Supersedes "stops the old session before the limit is checked": the replacement takes over the
  slot atomically and the old session is stopped after the new production started (ADR 0024
  Notes (M2-20)). Overall cap MaxSessions = 256.
