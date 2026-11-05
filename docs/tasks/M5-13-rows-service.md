# M5-13 Rows service and endpoints
Owner: mivo-backend
Depends on: M5-12 (uses M5-11 queries where available)
Files/areas: `internal/app/` (rows use case, small ports), `internal/adapters/sqlite/` (row queries), `internal/adapters/http/`, `cmd/mivo` (wiring)

## Goal
Home and Overview rows are generated on the server per M5-12.

## Acceptance criteria
- [ ] Each row kind has a use-case function and a fake-based table test (ordering, limits,
      empty data, library with missing metadata).
- [ ] Episode grouping logic (show card vs season card, `newEpisodeCount`) is pure domain
      code with >= 90 % coverage.
- [ ] "More <genre>" rotation is deterministic for a given date (test with a fixed clock).
- [ ] Integration test against SQLite with the test corpus; unavailable items are excluded
      from rows.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Only libraries the (future) profile may see: take a `visibleLibraries` port now (all libraries)
so M7 plugs access control in without changing handlers. The clock is a port.

Implementation note: the "recently added" window of the episode grouping (rows.RecentEpisodeWindow,
7 days before the show's newest episode) is applied in SQL by the row reader; the domain
function GroupEpisodes turns the windowed per-season counts into show or season cards.
