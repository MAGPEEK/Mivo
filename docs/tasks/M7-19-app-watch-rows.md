# M7-19 App: Continue watching and Up next rows
Owner: mivo-backend
Depends on: M7-14, M7-18
Files/areas: `internal/app/rows/`, `internal/domain/rows/` (row kinds and order only)

## Goal
Home shows "Continue watching" and "Up next" first, and a library's Overview shows its own
"Continue watching" (HO-03, HO-04, LB-01).

## Acceptance criteria
- [ ] Row kinds `continueWatching` and `upNext` in the domain row model; Home order:
      Continue watching, Up next, then the existing rows; library Overview: Continue watching
      (this library) first. Empty rows are omitted as today.
- [ ] A `WatchRows` port (2 methods, implemented by the M7-10 store) yields ids; summaries come
      from the restricted `SummaryReader`, so hidden items vanish; episode cards keep show title
      and season/episode numbers.
- [ ] Rows without a profile in the context are not produced (no error for the rest of Home).
- [ ] Hero (HO-01) prefers a Continue watching item with a backdrop, as the contract text says.
- [ ] Tests: fakes (`go test ./internal/app/rows/ ./internal/domain/rows/`): order, omission when
      empty, restricted summary drops an id, library scoping.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Viewer state decoration for row items comes from `app/watch.States` through a port, so cards in
the rows show progress bars.
