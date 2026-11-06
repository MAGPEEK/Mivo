# M7-10 SQLite: migration 00012 watch state store and row queries
Owner: mivo-backend
Depends on: M7-06, M7-08
Files/areas: `internal/adapters/sqlite/migrations/00012_watch_state.sql`, `internal/adapters/sqlite/queries/watch.sql`, generated `sqlcgen`, new `internal/adapters/sqlite/watch_store*.go`

## Goal
Per-profile watch state is stored so that no catalog change deletes it, and the queries for
Continue watching, Up next and badges are fast.

## Acceptance criteria
- [ ] Migration `00012` creates `watch_state` per ADR 0032 §8: primary key
      (`profile_id`, `item_id`), FK to profiles `ON DELETE CASCADE`, **no FK to items**,
      `attach_key`, index for (`profile_id`, `last_played_at`).
- [ ] Store: get/put state for one item; bulk mark watched/unwatched for the episodes of a show
      or season in one transaction; hide from continue watching; states for a list of item ids;
      unwatched episode counts for a list of shows/seasons.
- [ ] Queries `ContinueWatching(profile, libraryID optional, window, limit)` (one episode per
      show, joins only existing items) and `UpNext(profile, libraryID optional, window, limit)`
      return item ids in the order of ADR 0032 §8.
- [ ] Deleting a library, rescanning, or pruning an item leaves the `watch_state` rows in place
      (backlog rows "sync must never delete user data", M2-10 review).
- [ ] Tests: integration tests (`go test ./internal/adapters/sqlite/ -run Watch`): the queries
      agree with `domain/watch.NextUp` on a fixture with gaps and specials; library delete keeps
      rows; profile delete removes them; 2 000 shows fixture answers Up next under 50 ms on the
      dev machine (benchmark, not a gate).
- [ ] Gates: lint + layer rules + coverage thresholds pass; `sqlc generate` leaves the tree clean.

## Notes
The restriction (library access, age) is not applied here: the rows use case loads the
summaries through the restricted summary reader (M7-13), which drops hidden items. Close the
BACKLOG row about sync deleting user data in the same change.
