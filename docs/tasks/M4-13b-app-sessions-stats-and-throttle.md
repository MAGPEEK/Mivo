# M4-13b App playback: session list, statistics and throttle controller
Owner: mivo-backend
Depends on: M4-07b, M4-07c
Files/areas: `internal/app/playback/throttle.go`, `listing.go`, `registry.go`, `session.go` (+ tests)

## Goal
Operators see what runs, weak NAS disks and CPUs stay protected by pausing transcodes that are far ahead of the viewer (ADR 0030 §2, §7).

## Acceptance criteria
- [ ] `Service.List(ctx) []SessionInfo` (id, item, state, mode, encoder, hardware, output bit rate, speed, paused, client name, started at); no paths.
- [ ] `Access` records the highest requested segment index; a controller goroutine (tick 2 s, named constant) pauses a production whose produced
      segments exceed `highestRequested + ceil(ahead / segment duration)` and resumes below half of the lead; `ahead` from config; 0 disables.
- [ ] Pause only if `Pausable()`; the session's `paused` flag shows in `List`; `Stop`, replacement, shutdown and idle reaper resume-then-kill.
- [ ] A paused session whose client never returns is reaped by the idle timeout like any other.
- [ ] While paused the idle reaper still counts only stream requests (unchanged).
- [ ] Tests: fakes with a fake clock: lead grows → pause, requests advance → resume, unsupported producer → never paused, stop while paused, hysteresis (no flapping).
- [ ] Gates: lint + layer rules + `go test -race ./internal/app/playback/...` + coverage thresholds.

## Notes
- Segment duration is the ADR 0024 constant (6 s); the first segments (startup burst of ~3) are never throttled.
- Seeking uses a new session, so the throttle never conflicts with seeks.
