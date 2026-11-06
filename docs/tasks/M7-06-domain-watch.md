# M7-06 Domain: watch state rules
Owner: mivo-backend
Depends on: none
Files/areas: new `internal/domain/watch/`

## Goal
The rules of progress, watched, Continue watching and Up next are pure functions with tests, so
the store (M7-10) and the use cases (M7-18, M7-19) only move data.

## Acceptance criteria
- [ ] `State` (position seconds, watched, play count, last played, hidden) and
      `ApplyProgress(state, position, duration, now) State` per ADR 0032 §8: below
      max(30 s, 1 %) keeps "not started"; ≥ 90 % marks watched, resets position, increments the
      play count; unknown duration stores the position only; a new position un-hides the item.
- [ ] `MarkWatched` / `MarkUnwatched` (unwatched keeps the play count, clears the position).
- [ ] `SupportsWatchState(kind)` for movie, episode, audiobook (show/season only as mark targets).
- [ ] `NextUp(episodes []EpisodeRef, states)` returns the first unwatched episode after the last
      watched one in (season, episode) order, skipping season 0; none when all are watched.
- [ ] `ContinueWatchingWindow` = 12 weeks, `ContinueWatchingLimit` = 40, `UpNextLimit` = 20,
      `WatchedThreshold` = 0.9 as named constants.
- [ ] Tests: table-driven `go test -cover ./internal/domain/watch/` ≥ 90 %, incl. boundaries
      (29 s / 30 s, 89.9 % / 90 %, duration 0, 20 s file), multi-episode files, specials only,
      gaps in numbering.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Time is passed in, no clock. Episode refs carry season and episode numbers as the catalog has
them (absolute numbering has season 1). Keep `NextUp` independent of SQL so M7-10 may compute
it in SQL or in Go; the domain function is the reference the store test compares against.
