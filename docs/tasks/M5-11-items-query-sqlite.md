# M5-11 Items query: filters, sort, facets, letter offsets, folders
Owner: mivo-backend
Depends on: M5-10
Files/areas: `internal/domain/` (filter/sort value types), `internal/app/` (library read model), `internal/adapters/sqlite/` (queries, sqlc, indexes migration), `internal/adapters/http/` (handlers)

## Goal
The endpoints of M5-10 return real data for libraries of 50,000+ items quickly.

## Acceptance criteria
- [ ] Each sort and filter of M5-10 works and is covered by a table-driven test against real
      SQLite (temp dir) with a seeded catalog; ties break by id so cursors are stable.
- [ ] Title sort uses `sort_title`; facets honour all other active filters but not their own field.
- [ ] `letters` returns `#` and A-Z with offsets; letters without titles are marked absent.
- [ ] Folder listing resolves only inside library roots; `..`, absolute paths and symlinks
      leaving the root answer 400/404 (tests).
- [ ] Performance check: 50,000 seeded items, a filtered+sorted first page and the facets in
      < 300 ms in a Go benchmark (documented number, not a flaky assertion).
- [ ] Tests: domain table tests, use case fakes, adapter integration, handler tests.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
New migration only for indexes/columns that are needed (resolution, bitrate, size, added
time); keep functions <= 50 lines by building queries per filter in small functions.
