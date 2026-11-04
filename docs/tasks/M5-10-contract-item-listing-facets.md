# M5-10 Contract: item listing with sort, filters, facets, letter offsets, folders
Owner: mivo-backend
Depends on: M3-06b
Files/areas: `api/openapi.yaml`, generated `internal/adapters/http/apigen`, `web/packages/api-client`, `docs/adr/` (only if a decision changes)

## Goal
The API describes everything the Browse pivot needs (LB-20...23, 29, 36): sorting, quick and
field filters, per-value counts, start offset per letter, and the folder tree of a library.

## Acceptance criteria
- [ ] `listLibraryItems` gains `sort` (title, year, releaseDate, contentRating, duration,
      dateAdded, resolution, bitrate, fileSize, random with `seed`), `order`, `type` (library's
      item type), `quick` (all, hdr, dolbyVision, hdr10plus, atmos, uhd, unavailable,
      duplicates) and field filters (genre, year, decade, contentRating, resolution,
      videoCodec, audioCodec, container, audioLanguage, subtitleLanguage); response carries
      `totalCount`. Filters M6/M7 (people, collection, unmatched, watch state) are absent.
- [ ] `GET /api/v1/libraries/{id}/facets` returns values with counts per field for the current
      filter; `GET .../letters` returns `{letter, offset}` for the title sort and current filter.
- [ ] `GET /api/v1/libraries/{id}/folders?path=` lists library-relative children (folders and
      items), never absolute paths; traversal attempts (`..`) answer 400.
- [ ] Unknown sort/filter values answer 400 with a problem body; cursors stay opaque.
- [ ] Tests: contract lint (spec validates), `go generate ./...` and `pnpm -C web gen` leave
      `git diff --exit-code` clean.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Spec-first only: handlers return 501 until M5-11. Random order must be stable per seed so paging
works. Offsets are positions in the sorted, filtered list so the client can scroll a virtual grid.
