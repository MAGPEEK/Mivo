# M5-14 SQLite FTS5 index and query
Owner: mivo-backend
Depends on: M3-05c
Files/areas: `internal/adapters/sqlite/` (migration, index maintenance, query), `internal/app/` (search port declaration only)

## Goal
Fast, accent- and case-insensitive prefix search over titles, original titles, sort titles,
people and genres (SR-03).

## Acceptance criteria
- [ ] Migration creates an FTS5 table (`unicode61 remove_diacritics`) kept in sync by triggers
      or writer hooks on insert, update and delete; an existing catalog is back-filled.
- [ ] `Search(ctx, query, filter, limit)` port (declared in `app`) returns ranked hits with
      type and library; prefix matching ("matr" finds "The Matrix"); "cafe" finds "Café".
- [ ] Hostile input (quotes, `*`, `NEAR`, empty, 10 kB) is escaped and never causes an SQL error.
- [ ] Integration tests on temp SQLite: rank order, delete/rename updates, 50,000 items query
      < 100 ms (benchmark number documented).
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
People/genre columns exist only where M3 NFO delivered them; index what is there.
