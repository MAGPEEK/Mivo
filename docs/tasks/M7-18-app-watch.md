# M7-18 App: watch state use cases
Owner: mivo-backend
Depends on: M7-06, M7-15
Files/areas: new `internal/app/watch/`

## Goal
The signed-in profile's progress, watched marks and hidden flags are recorded with the rules of
`domain/watch`, and item lists can be decorated with the profile's state.

## Acceptance criteria
- [x] `ReportProgress(ctx, itemID, position)`: viewer from context; item looked up through a
      viewer-scoped item port (hidden → `ErrNotFound`); kind checked with
      `SupportsWatchState`; duration taken from the catalog; `attach_key` from the item's
      provider reference; `ApplyProgress` then store.
- [x] `MarkWatched` / `MarkUnwatched` for movie, episode, audiobook, and for show/season over
      their episodes; `HideFromContinueWatching`.
- [x] `States(ctx, items)` returns the viewer state per summary (incl. unwatched counts for
      shows/seasons) in at most two store calls per page.
- [x] Tests: fakes (`go test ./internal/app/watch/`): no viewer → `ErrUnauthenticated`,
      no profile → profile error, photo/track refused, 90 % boundary through the use case,
      show mark cascades, hidden item.
- [x] Gates: lint + layer rules + coverage thresholds pass.

## Notes
The item port is satisfied by the scoped `app/library` service after M7-14; use a fake here,
so this task does not wait for M7-14. Progress calls arrive every 10 s per player – no extra
reads beyond one item lookup and one upsert.
