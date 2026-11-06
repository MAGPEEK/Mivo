# M7-13 SQLite: restriction in rows, search, children, summaries, detail
Owner: mivo-backend
Depends on: M7-12
Files/areas: `internal/app/rows/ports.go`, `internal/app/search/ports.go`, `internal/app/library/ports.go` (restriction parameters only), `internal/adapters/sqlite/rows_*.go`, search reader, `catalog_reader*.go`, their tests

## Goal
Every other catalog read honours the same restriction, so a locked title appears in no row, no
search result, no children list and not by direct link (US-12).

## Acceptance criteria
- [ ] Port requests gain an `access.Restriction`: rows `ItemRequest`/`EpisodeRequest`,
      `SummaryReader.ItemSummaries`, search query, children listing, item detail; the sqlite
      readers apply the M7-12 predicate.
- [ ] A hidden item read by id returns `domain.ErrNotFound`; a show's children omit hidden
      episodes; search suggestions and result tabs omit hidden items and their people-only hits
      stay (people carry no rating).
- [ ] Tests: integration tests (`go test ./internal/adapters/sqlite/ -run 'Rows|Search|Children|Detail'`)
      on the M7-12 fixture: the "Children" restriction yields no FSK 16 item in any read;
      zero restriction yields nothing.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
App services keep compiling by passing `access.Unrestricted()` until M7-14 threads the viewer
through; mark those call sites with `// TODO(M7-14)`. The FTS5 query must filter inside the
query (not after `LIMIT`), or pages come back short.
