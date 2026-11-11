# M7-12 SQLite: restriction in listing, facets, letters, folders
Owner: mivo-backend
Depends on: M7-11
Files/areas: `internal/domain/library/listing.go` (one field), `internal/adapters/sqlite/listing_*.go`, `internal/adapters/sqlite/folder_items.go`, their tests

## Goal
Library browsing returns only what the viewer's profile may see: items above the content limit,
unrated items (when hidden) and libraries outside the access set are absent from pages, counts,
facets, A–Z letters and folder views.

## Acceptance criteria
- [x] `library.ListingQuery` (and the facet/letter/folder requests) carry an
      `access.Restriction`; the zero value denies everything, so a forgotten caller fails
      closed (test).
- [x] One SQL predicate builder (library ids, `effective_age_rating` ≤ max, unrated handling) is
      used by items, facets, letters, folders and total counts; facet counts and letter offsets
      match the filtered list.
- [x] Tests: integration tests (`go test ./internal/adapters/sqlite/ -run Listing`) with a
      fixture of FSK 0/12/16/18 and unrated movies plus a show with rated and unrated episodes:
      every endpoint shape returns identical item sets for a "Children" restriction; the
      listing benchmark does not regress by more than 10 % (`-bench Listing`).
- [x] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Folder view: a folder whose only items are hidden must not appear. Keep the predicate in one
file so M7-13 reuses it. Existing callers in `app/library` and `app/rows` set
`access.Unrestricted()` with `// TODO(M7-14)` so behaviour is unchanged until M7-14 (the only
app-layer edit allowed here).
