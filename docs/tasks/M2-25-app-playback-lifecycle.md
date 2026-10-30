# M2-25 Use case: session lifecycle – state, idle timeout, purge and shutdown
Owner: mivo-media
Depends on: M2-20
Files/areas: `internal/app/playback/{lifecycle.go,reaper.go}` (+ tests), small changes to the
session registry from M2-20

## Goal
Sessions clean up after themselves: finished or failed productions update the session state,
forgotten sessions are stopped after the idle timeout, leftovers of a crash are purged at
startup, and shutdown stops every FFmpeg before the server exits (ADR 0024).

## Acceptance criteria
- [ ] Session state follows the production: `starting` → `ready` (first segment) → `ended`
      (clean exit) or `failed` (exit error; the session's `failureCode` is `transcode_failed`,
      field defined in M2-01); direct play sessions are `ready` immediately.
- [ ] Idle reaper: a goroutine owned by the service checks every named interval and stops sessions
      whose last access is older than the 2-minute idle timeout (named constant); uses the clock
      port so tests run without sleeping.
- [ ] `PurgeWorkspace(ctx)` calls the workspace port's `Purge` once before the first session
      (wired at startup in M2-32) and logs the number of removed directories.
- [ ] `Close(ctx)` stops accepting new sessions (`ErrUnavailable`), stops all sessions in
      parallel, waits for every production and workspace removal (bounded by ctx), stops the
      reaper. A second `Close` is a no-op.
- [ ] Tests with fakes and a fake clock: idle expiry, access keeps alive, ended/failed transitions,
      close during start, close waits for slow stop, no goroutine left after `Close` (`-race`).
      Coverage of the package ≥ 90 %.
- [ ] Gates: lint + layer rules + `go test -race ./internal/app/...` + coverage gate.

## Notes
- The reaper must not hold the registry lock while stopping a session (stopping waits for
  FFmpeg).
- Keep timers injectable (ticker factory or clock with `After`) instead of `time.Sleep`.

## Orchestrator note (from M2-20)
- The service's process context is `context.Background()` for now (an uncalled cancel trips
  gosec G118); make it cancellable in `Close` and remove the justified `//nolint:contextcheck`
  rationale if it no longer applies. Registry, states and access times are prepared; no timers yet.
- Production contract: after `Done`, `Ready` returns the exit result → map to ended/failed.
- Open question recorded: there is no overall cap for direct-play sessions (ADR 0024 does not
  require one); the idle reaper removes them – note this in ADR 0024 Notes.
- (from M2-20 r2) `Service.activate`'s branch for a pending session that vanished during start
  (tears down the new production, returns ErrUnavailable) becomes reachable with `Close` – add a
  test for it. MaxSessions = 256 is the overall cap incl. direct play.
- (from M2-20 review r2) Union counting during a replacement: a replacement in a lower class
  (e.g. transcode → remux) frees the old session's higher-class slot while the old one still
  runs; count a claimed old entry in every category its claimant does not occupy (countExcept),
  test: transcode replaced by remux whose producer fails with a third transcode in between.
  Any path that removes a pending entry must clear its claim on the replaced session (reuse
  release). Fix the stale ReplacesSessionID doc in session.go; add a test that Access(old id)
  still returns the old workspace during a swap.
