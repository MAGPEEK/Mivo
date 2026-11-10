# M7-35 player-core progress reporter and player integration
Owner: mivo-frontend
Depends on: M7-03
Files/areas: new `web/packages/player-core/src/progress*.ts`, `web/apps/web/src/features/player/` (hook wiring, start position)

## Goal
While a movie, episode or audiobook plays, the server learns the position, so resume and
Continue watching work across devices.

## Acceptance criteria
- [x] Framework-free `createProgressReporter({ send, clock })` in `player-core`: reports every
      10 s while playing, immediately on pause, seek end, stop, ended and page hide; skips
      unchanged positions; never sends more than once per 2 s; `send` failures are retried on
      the next tick only (no queue growth).
- [x] The player wires it with `fetch(..., { keepalive: true })` to
      `PUT /items/{id}/progress`, using media position + `startPositionSeconds` (session
      offset) as the absolute position; music and photos do not report.
- [x] The player route accepts a start position (existing param or `?t=` seconds) and passes it
      as `startPositionSeconds`; the detail page link is M7-36.
- [x] Tests: Vitest with fake timers (`pnpm -C web test`): interval, pause/seek/hide triggers,
      throttle, offset math after a remux seek restart, no report for music.
- [x] Gates: `pnpm -C web lint` (incl. player-core legacy-transpile rules, OD-UX-1) and coverage pass.

## Notes
`player-core` must stay usable on webOS 4.x (no newer syntax than the package allows). The
audiobook player (M5-73) uses the same reporter.

## Implementation notes (M7-35)
- Reporter: `web/packages/player-core/src/progress/progressReporter.ts`. Its clock is
  `{ now, startTimer }`; a trigger inside the 2 s throttle window is sent at the end of the
  window (one timer, latest position), so the throttle never drops the position of a pause.
  The reporter tests drive the injected clock, because player-core has no timer globals.
- The position always comes from the engine (`PlaybackEngine.currentPosition`), which adds the
  session offset and keeps the position while the element is empty during a remux seek restart.
  A session that starts playing after such a restart counts as "seek end".
- Audiobooks of several parts report the position in the book (`bookPartStart`), as the API
  `ProgressReport` requires; `progressTarget` decides per item kind.
- The sender resolves for 204, 401, 403 and 501 and rejects otherwise (retry with the next
  report); nothing is ever shown in the player.
- The route already accepted `?t=` (`validatePlayerSearch`, `resolvePlaybackStart`); no change.
