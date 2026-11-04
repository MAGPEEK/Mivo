# M3-05c SQLite: catalog reader v2 and artwork lookup
Owner: mivo-backend
Depends on: M3-05, M3-06, M3-09
Files/areas: `internal/adapters/sqlite/catalog_reader.go`, `catalog_reader_v2.go` (new),
`catalog_artwork_reader.go` (new), `catalog_cursor.go`, `queries/catalog_read.sql` (new),
generated `sqlcgen/catalog_read.sql.go`, their tests, `docs/tasks/BACKLOG.md` (only the two
`sqlite` reader rows)

## Goal
The read side of the catalog serves the v2 read model of `app/library` (M3-06) and the
artwork lookup of `app/images` (M3-09): top-level items with posters, children of shows and
seasons in order, item details with metadata, ancestors, siblings, extras and the play target,
and the stored location of an artwork for the image endpoint. UX: DT-20 … DT-22, DT-24,
LB-40, LB-44, DT-06, DT-08.

## Acceptance criteria
- [ ] Implements `applibrary.ItemReader` v2 and `applibrary.ChildReader` (M3-06) and the
      artwork source port of `app/images` (M3-09) with their error contracts (compile-time
      assertions); `PlayableFile` for playback keeps
      working (primary file = first by `(part_number, relative_path)`).
- [ ] Top-level listing: movies and shows only, ordered by `(sort_title, id)` with the existing
      cursor format; summary includes year, selected poster (season → show fallback is not
      needed at this level), child and leaf counts, primary media summary for movies.
- [ ] Children: seasons by number with 0 last; episodes by number, unnumbered last by sort
      title; `desc` reverses; cursor encodes the order key; extras never returned.
- [ ] Detail: record (tags in order, credits in order, provider IDs), ancestors root first,
      previous/next sibling in ascending order, `playItemId` per ADR/contract rules, extras
      (≤ 100, by type then title), artwork refs per kind with the season-poster fallback to
      the show, files with version label and part.
- [ ] Artwork lookup by ID returns `artwork.Stored` with the library root and the relative
      path (or media file path and stream index for embedded); a list of all `(id, tag)`
      pairs for the cache purge; unknown ID → `domain.ErrNotFound`.
- [ ] BACKLOG rows done and deleted: cursor `\r`/`\n` rejected explicitly; the M2-10 sqlite
      nits not handled by M3-05 (`COLLATE NOCASE` wording, lightweight roots query).
- [ ] Integration tests against a temp SQLite with a fixture written through the M3-05
      schema (insert helpers in this task's test files, not through M3-05b): ordering incl.
      specials and DT-24 (131 episodes, one season, pages of 50), fallback, siblings at both
      ends, play target of an empty season, query plan uses the new indexes (`EXPLAIN QUERY
      PLAN` test like M2's catalog plan test).
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./internal/adapters/sqlite/...`, coverage gate.

## Notes
- Runs in wave 3, before the writers exist: build fixtures with plain SQL inserts so this task
  does not depend on M3-05b/M3-05d.
- Counts (`childCount`, `leafCount`) may come from correlated subqueries; check the plan for
  a 50,000-item library stays index-only.
